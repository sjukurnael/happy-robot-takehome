package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	EventTaskCreated             = "task.created"
	EventTaskUpdated             = "task.updated"
	EventTaskDeleted             = "task.deleted"
	EventTaskDependenciesChanged = "task.dependencies_changed"
	EventCommentAdded            = "comment.added"
	EventCommentDeleted          = "comment.deleted"
	EventProjectUpdated          = "project.updated"
)

// StoredEvent mirrors a row in the events table. It's the shape returned
// by the catch-up endpoint and embedded in live WS broadcasts — one type
// for "an event you can apply," regardless of how it arrived.
type StoredEvent struct {
	ProjectID string          `json:"projectId"`
	Seq       int64           `json:"seq"`
	EventType string          `json:"eventType"`
	Payload   json.RawMessage `json:"payload"`
	Actor     string          `json:"actor"`
	CreatedAt time.Time       `json:"createdAt"`
}

type TaskCreatedPayload struct {
	Task *Task `json:"task"`
}

// TaskUpdatedPayload.Changes only ever holds scalar fields (title, status,
// assignedTo, configuration) — dependency changes are always their own
// task.dependencies_changed event, never folded in here.
type TaskUpdatedPayload struct {
	TaskID  string         `json:"taskId"`
	Changes map[string]any `json:"changes"`
}

type TaskDeletedPayload struct {
	TaskID                    string   `json:"taskId"`
	RemovedFromDependenciesOf []string `json:"removedFromDependenciesOf"`
}

type TaskDependenciesChangedPayload struct {
	TaskID    string   `json:"taskId"`
	DependsOn []string `json:"dependsOn"`
}

type CommentAddedPayload struct {
	Comment *Comment `json:"comment"`
}

type CommentDeletedPayload struct {
	CommentID string `json:"commentId"`
	TaskID    string `json:"taskId"`
}

type ProjectUpdatedPayload struct {
	Changes map[string]any `json:"changes"`
}

// recordEvent atomically claims the next seq for projectID and appends the
// event row, all within tx. It must be called before tx.Commit, and the
// caller must broadcast the returned event only after that commit
// succeeds — never before, and never if the transaction rolls back. If
// this function returns an error, the caller's deferred tx.Rollback()
// undoes the claim along with everything else in the transaction, so a
// failed mutation never burns a seq number or leaves an orphaned event
// row.
func recordEvent(ctx context.Context, tx querier, projectID, eventType string, payload any, actor string) (StoredEvent, error) {
	var seq int64
	if err := tx.QueryRow(ctx, `UPDATE projects SET last_seq = last_seq + 1 WHERE id = $1::uuid RETURNING last_seq`, projectID).Scan(&seq); err != nil {
		return StoredEvent{}, fmt.Errorf("claim seq: %w", err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return StoredEvent{}, fmt.Errorf("encode event payload: %w", err)
	}
	var createdAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO events (project_id, seq, type, payload, actor)
		VALUES ($1::uuid, $2, $3, $4::jsonb, $5)
		RETURNING created_at`,
		projectID, seq, eventType, body, actor,
	).Scan(&createdAt); err != nil {
		return StoredEvent{}, fmt.Errorf("insert event: %w", err)
	}
	return StoredEvent{
		ProjectID: projectID,
		Seq:       seq,
		EventType: eventType,
		Payload:   body,
		Actor:     actor,
		CreatedAt: createdAt,
	}, nil
}

// ListEventsSince returns events for projectID with seq > after, ordered
// ascending, capped at limit. Passing after=0 returns the project's full
// event history from the beginning — that's a valid query but not what
// this is for; a fresh client should seed its lastSeq from a snapshot
// endpoint instead of walking the whole log.
func (s *Store) ListEventsSince(ctx context.Context, projectID string, after int64, limit int) ([]StoredEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT project_id::text, seq, type, payload, actor, created_at
		FROM events
		WHERE project_id = $1::uuid AND seq > $2
		ORDER BY seq ASC
		LIMIT $3`,
		projectID, after, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []StoredEvent{}
	for rows.Next() {
		var e StoredEvent
		if err := rows.Scan(&e.ProjectID, &e.Seq, &e.EventType, &e.Payload, &e.Actor, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
