package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// newID generates an ephemeral, prefixed ID for things that don't live in
// Postgres (currently just WebSocket client IDs) — domain records get their
// IDs from the database instead (gen_random_uuid()).
func newID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, uuid.NewString())
}

// PresenceEntry describes one connected client's current location for
// "who's viewing what" indicators.
type PresenceEntry struct {
	ClientID string `json:"clientId"`
	Name     string `json:"name"`
	TaskID   string `json:"taskId,omitempty"`
}

// Event is the outbound WS envelope. Durable, sequenced domain events
// (task.*, comment.*, project.updated) use Type "event", with the actual
// domain type in EventType and Seq/Payload/Actor populated from the
// events table row. presence.updated and project.deleted (both
// intentionally outside the per-project event log — see events.go) keep
// their own simpler shapes, using Type directly and Presence/ProjectID.
type Event struct {
	Type       string          `json:"type"` // "event" | "presence.updated" | "project.deleted"
	ProjectID  string          `json:"projectId,omitempty"`
	ResourceID string          `json:"resourceId,omitempty"` // only used by the legacy thin project.deleted notification
	Seq        int64           `json:"seq,omitempty"`
	EventType  string          `json:"eventType,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Actor      string          `json:"actor,omitempty"`
	Presence   []PresenceEntry `json:"presence,omitempty"`
}

type wsClient struct {
	conn      *websocket.Conn
	clientID  string
	name      string
	projectID string
	taskID    string
	// writeMu serializes writes to conn: gorilla/websocket forbids
	// concurrent writers on one connection, and this connection can now be
	// written to from two different goroutines — its own read-loop
	// (subscribe-time replay) and whichever HTTP handler goroutine is
	// broadcasting a live event.
	writeMu sync.Mutex
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

	client := &wsClient{conn: conn, clientID: clientID, name: name}
	h.mu.Lock()
	h.clients[conn] = client
	h.mu.Unlock()

	go func() {
		defer func() {
			h.mu.Lock()
			c := h.clients[conn]
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
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
		h.replayTo(conn, client, projectID, lastSeq)
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
func (h *Hub) replayTo(conn *websocket.Conn, client *wsClient, projectID string, after int64) {
	events, err := h.store.ListEventsSince(context.Background(), projectID, after, 500)
	if err != nil {
		log.Println("replay query error:", err)
		return
	}
	for _, ev := range events {
		h.writeEvent(conn, client, ev)
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

// writeEvent sends a single durable event to one connection, under that
// connection's write lock.
func (h *Hub) writeEvent(conn *websocket.Conn, client *wsClient, ev StoredEvent) {
	data, err := json.Marshal(Event{
		Type:      "event",
		ProjectID: ev.ProjectID,
		Seq:       ev.Seq,
		EventType: ev.EventType,
		Payload:   ev.Payload,
		Actor:     ev.Actor,
	})
	if err != nil {
		log.Println("event marshal error:", err)
		return
	}
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Println("ws write error:", err)
	}
}

// BroadcastEvent sends a durable event to every connected client. Callers
// must only call this after the transaction that produced ev has
// committed successfully — never before, and never if it rolled back.
func (h *Hub) BroadcastEvent(ev StoredEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn, client := range h.clients {
		h.writeEvent(conn, client, ev)
	}
}

// Broadcast sends a pre-built envelope (presence updates, and the legacy
// thin project.deleted notification) to every connected client.
func (h *Hub) Broadcast(evt Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		log.Println("event marshal error:", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn, client := range h.clients {
		client.writeMu.Lock()
		err := conn.WriteMessage(websocket.TextMessage, data)
		client.writeMu.Unlock()
		if err != nil {
			log.Println("ws write error:", err)
		}
	}
}
