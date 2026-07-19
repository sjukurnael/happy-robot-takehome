package main

import (
	"encoding/json"
	"time"
)

type Project struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata"`
	// LastSeq is the project's current event sequence number — a fresh
	// client seeds its lastSeq from this when it first loads the project,
	// then subscribes over WS with it so the server replays only what's
	// missing.
	LastSeq   int64     `json:"lastSeq"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type TaskStatus string

const (
	StatusTodo       TaskStatus = "todo"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
)

type TaskConfiguration struct {
	Priority     string         `json:"priority"`
	Description  string         `json:"description"`
	Tags         []string       `json:"tags"`
	CustomFields map[string]any `json:"customFields"`
}

type Task struct {
	ID            string            `json:"id"`
	ProjectID     string            `json:"projectId"`
	Title         string            `json:"title"`
	Status        TaskStatus        `json:"status"`
	AssignedTo    []string          `json:"assignedTo"`
	Configuration TaskConfiguration `json:"configuration"`
	Dependencies  []string          `json:"dependencies"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

type Comment struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"taskId"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	Timestamp time.Time `json:"timestamp"`
}

// ProjectPatch and TaskPatch are the PATCH request bodies. Nil means "field
// not present, leave it alone" — which is what makes field-granularity
// updates possible (see README: last-write-wins per field, not per record).
type ProjectPatch struct {
	Name        *string        `json:"name"`
	Description *string        `json:"description"`
	Metadata    map[string]any `json:"metadata"`
}

type TaskPatch struct {
	Title         *string            `json:"title"`
	Status        *TaskStatus        `json:"status"`
	AssignedTo    []string           `json:"assignedTo"`
	Configuration *TaskConfiguration `json:"configuration"`
	Dependencies  []string           `json:"dependencies"`
}

// BreakdownSuggestion is one AI-proposed subtask. DependsOn holds indices
// into the same suggestions array — the subtasks don't have IDs yet; the
// server maps indices to real UUIDs when the batch is created.
type BreakdownSuggestion struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Tags        []string `json:"tags"`
	DependsOn   []int    `json:"dependsOn"`
}

// BreakdownResponse is the suggest-phase response — pure suggestions,
// nothing persisted. The user reviews (and may deselect) these before the
// apply phase creates anything.
type BreakdownResponse struct {
	Suggestions []BreakdownSuggestion `json:"suggestions"`
}

// BreakdownApplyRequest is the apply-phase body: the suggestions the user
// kept, re-indexed by the client so DependsOn refers to positions in
// Subtasks.
type BreakdownApplyRequest struct {
	Subtasks []BreakdownSuggestion `json:"subtasks"`
}

// BreakdownApplyResponse returns the created subtasks; the board itself
// updates via the task.created / task.dependencies_changed events.
type BreakdownApplyResponse struct {
	Created []*Task `json:"created"`
}

// ProjectStats is the dashboard's per-project aggregate — computed in one
// SQL pass instead of shipping every task of every project to the client.
// "Blocked" mirrors client/src/taskUtils.ts exactly: a task counts as
// blocked when any of its dependencies is not done.
type ProjectStats struct {
	ProjectID  string    `json:"projectId"`
	Total      int       `json:"total"`
	Done       int       `json:"done"`
	Blocked    int       `json:"blocked"`
	Assignees  []string  `json:"assignees"`
	LastEdited time.Time `json:"lastEdited"`
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
// events table row. presence.updated, project.created, and
// project.deleted (all intentionally outside the per-project event log —
// see events.go) keep their own simpler shapes, using Type directly and
// Presence/ProjectID.
type Event struct {
	Type       string          `json:"type"` // "event" | "presence.updated" | "project.created" | "project.deleted"
	ProjectID  string          `json:"projectId,omitempty"`
	ResourceID string          `json:"resourceId,omitempty"` // only used by the thin project.created/deleted notifications
	Seq        int64           `json:"seq,omitempty"`
	EventType  string          `json:"eventType,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Actor      string          `json:"actor,omitempty"`
	Presence   []PresenceEntry `json:"presence,omitempty"`
}
