package main

import "time"

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
