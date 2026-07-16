package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// PresenceEntry describes one connected client's current location for
// "who's viewing what" indicators.
type PresenceEntry struct {
	ClientID string `json:"clientId"`
	Name     string `json:"name"`
	TaskID   string `json:"taskId,omitempty"`
}

// Event is the naive real-time payload: for resource events it just
// tells clients *what* changed, not the full new state — clients
// refetch the affected resource via REST. Presence events instead
// carry the full roster for the project, since that's cheap and
// simplifies the client.
type Event struct {
	Type       string          `json:"type"` // e.g. "task.created", "task.updated", "task.deleted", "comment.created", "project.updated", "presence.updated"
	ProjectID  string          `json:"projectId,omitempty"`
	ResourceID string          `json:"resourceId,omitempty"`
	Presence   []PresenceEntry `json:"presence,omitempty"`
}

type wsClient struct {
	conn      *websocket.Conn
	clientID  string
	name      string
	projectID string
	taskID    string
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]*wsClient
}

func NewHub() *Hub {
	return &Hub{clients: map[*websocket.Conn]*wsClient{}}
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
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "viewing":
				h.setViewing(conn, msg.ProjectID, msg.TaskID)
			case "rename":
				h.rename(conn, msg.Name)
			}
		}
	}()
}

// setViewing records where a client is currently looking and notifies
// anyone watching the old and new project of the roster change.
func (h *Hub) setViewing(conn *websocket.Conn, projectID, taskID string) {
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

	if oldProjectID != "" && oldProjectID != projectID {
		h.broadcastPresence(oldProjectID)
	}
	if projectID != "" {
		h.broadcastPresence(projectID)
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

func (h *Hub) Broadcast(evt Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		log.Println("event marshal error:", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Println("ws write error:", err)
		}
	}
}
