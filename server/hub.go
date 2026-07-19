package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	// writeTimeout bounds any single write to a connection — a peer that
	// stops reading can't wedge the writer goroutine indefinitely.
	writeTimeout = 10 * time.Second
	// pongWait is how long a connection may go silent before the read loop
	// gives up on it; pingInterval must be comfortably shorter so a healthy
	// client always gets a ping (and answers with a pong, resetting the
	// deadline) in time. Browsers answer pings automatically.
	pongWait     = 60 * time.Second
	pingInterval = 30 * time.Second
	// sendQueueSize is the per-connection buffered send queue. A client
	// that falls this many messages behind is disconnected (backpressure)
	// rather than allowed to slow everyone else's broadcast; it resyncs
	// through the normal reconnect path — re-sending "viewing" with its
	// lastSeq — so nothing is lost by dropping it.
	sendQueueSize = 256
)

// newID generates an ephemeral, prefixed ID for things that don't live in
// Postgres (currently just WebSocket client IDs) — domain records get their
// IDs from the database instead (gen_random_uuid()).
func newID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, uuid.NewString())
}

// PresenceEntry and Event (the outbound WS envelope) live in models.go
// with the rest of the wire contract.

type wsClient struct {
	conn      *websocket.Conn
	clientID  string
	name      string
	projectID string
	taskID    string
	// send is drained by the client's writer goroutine — the sole writer
	// to conn, which is what satisfies gorilla/websocket's one-writer rule
	// (pings included, since the writer goroutine sends those too).
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

// close is safe to call from any goroutine, any number of times.
func (c *wsClient) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		c.conn.Close()
	})
}

// enqueue hands data to the writer goroutine without ever blocking the
// caller (broadcasts run on HTTP handler goroutines). A full queue means
// the client can't keep up — disconnect it instead of letting it apply
// backpressure to everyone else; it will reconnect and catch up via the
// event log.
func (c *wsClient) enqueue(data []byte) {
	select {
	case c.send <- data:
	case <-c.done:
	default:
		log.Printf("ws: disconnecting slow client %s (send queue full)", c.clientID)
		c.close()
	}
}

// writeLoop is the only goroutine that writes to conn. It drains the send
// queue and keeps the connection alive with periodic pings.
func (c *wsClient) writeLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case data := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				c.close()
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.close()
				return
			}
		case <-c.done:
			return
		}
	}
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]*wsClient
	store   *Store
}

func NewHub(store *Store) *Hub {
	return &Hub{clients: map[*websocket.Conn]*wsClient{}, store: store}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // naive: allow all origins for local dev
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ws upgrade error:", err)
		return
	}

	clientID := r.URL.Query().Get("clientId")
	if clientID == "" {
		clientID = newID("client")
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Guest"
	}

	client := &wsClient{
		conn:     conn,
		clientID: clientID,
		name:     name,
		send:     make(chan []byte, sendQueueSize),
		done:     make(chan struct{}),
	}
	h.mu.Lock()
	h.clients[conn] = client
	h.mu.Unlock()

	go client.writeLoop()

	// Dead-connection detection: the read loop bails if the peer goes
	// silent past pongWait; the writer's pings keep a healthy peer's pongs
	// (sent automatically by browsers) flowing to reset the deadline.
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	go func() {
		defer func() {
			h.mu.Lock()
			c := h.clients[conn]
			delete(h.clients, conn)
			h.mu.Unlock()
			client.close()
			if c != nil && c.projectID != "" {
				h.broadcastPresence(c.projectID)
			}
		}()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				Type      string `json:"type"`
				ProjectID string `json:"projectId"`
				TaskID    string `json:"taskId"`
				Name      string `json:"name"`
				LastSeq   int64  `json:"lastSeq"`
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "viewing":
				h.setViewing(conn, msg.ProjectID, msg.TaskID, msg.LastSeq)
			case "rename":
				h.rename(conn, msg.Name)
			}
		}
	}()
}

// setViewing records where a client is currently looking, replays any
// events it missed if this is a subscribe to a (new) project, and
// notifies anyone watching the old and new project of the roster change.
func (h *Hub) setViewing(conn *websocket.Conn, projectID, taskID string, lastSeq int64) {
	h.mu.Lock()
	client, ok := h.clients[conn]
	if !ok {
		h.mu.Unlock()
		return
	}
	oldProjectID := client.projectID
	client.projectID = projectID
	client.taskID = taskID
	h.mu.Unlock()

	// Subscribing to a (new) project: replay everything missed since
	// lastSeq directly to this connection before it's otherwise eligible
	// for live broadcasts of new events on that project. Only fires on an
	// actual project switch/initial subscribe, not on every task-focus
	// change within the same project.
	if projectID != "" && projectID != oldProjectID {
		h.replayTo(client, projectID, lastSeq)
	}

	if oldProjectID != "" && oldProjectID != projectID {
		h.broadcastPresence(oldProjectID)
	}
	if projectID != "" {
		h.broadcastPresence(projectID)
	}
}

// replayTo sends every event the connection missed for projectID directly
// to it (not broadcast to everyone), in seq order.
//
// TODO: if a client is extremely far behind (very old lastSeq on a
// long-lived, busy project), replaying the full backlog here could mean a
// large query and a burst of messages. A "too far behind — tell the
// client to refetch a fresh snapshot instead" fallback would be the fix;
// not implemented in this phase since ordinary reconnect gaps are small,
// and ListEventsSince already caps a single query at 500 rows.
func (h *Hub) replayTo(client *wsClient, projectID string, after int64) {
	events, err := h.store.ListEventsSince(context.Background(), projectID, after, 500)
	if err != nil {
		log.Println("replay query error:", err)
		return
	}
	for _, ev := range events {
		data, err := marshalStoredEvent(ev)
		if err != nil {
			log.Println("event marshal error:", err)
			continue
		}
		client.enqueue(data)
	}
}

// rename updates a client's display name and re-broadcasts presence
// for whatever project they're currently viewing, if any.
func (h *Hub) rename(conn *websocket.Conn, name string) {
	if name == "" {
		return
	}
	h.mu.Lock()
	client, ok := h.clients[conn]
	if !ok {
		h.mu.Unlock()
		return
	}
	client.name = name
	projectID := client.projectID
	h.mu.Unlock()

	if projectID != "" {
		h.broadcastPresence(projectID)
	}
}

func (h *Hub) broadcastPresence(projectID string) {
	h.mu.RLock()
	entries := make([]PresenceEntry, 0)
	for _, c := range h.clients {
		if c.projectID == projectID {
			entries = append(entries, PresenceEntry{ClientID: c.clientID, Name: c.name, TaskID: c.taskID})
		}
	}
	h.mu.RUnlock()
	h.Broadcast(Event{Type: "presence.updated", ProjectID: projectID, Presence: entries})
}

func marshalStoredEvent(ev StoredEvent) ([]byte, error) {
	return json.Marshal(Event{
		Type:      "event",
		ProjectID: ev.ProjectID,
		Seq:       ev.Seq,
		EventType: ev.EventType,
		Payload:   ev.Payload,
		Actor:     ev.Actor,
	})
}

// BroadcastEvent sends a durable event to every connected client. Callers
// must only call this after the transaction that produced ev has
// committed successfully — never before, and never if it rolled back.
func (h *Hub) BroadcastEvent(ev StoredEvent) {
	data, err := marshalStoredEvent(ev)
	if err != nil {
		log.Println("event marshal error:", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.clients {
		client.enqueue(data)
	}
}

// Broadcast sends a pre-built envelope (presence updates, and the thin
// project.created/deleted notifications) to every connected client.
func (h *Hub) Broadcast(evt Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		log.Println("event marshal error:", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.clients {
		client.enqueue(data)
	}
}
