package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrSelfDependency         = errors.New("a task cannot depend on itself")
	ErrCyclicDependency       = errors.New("dependency would create a cycle")
	ErrDependencyNotFound     = errors.New("dependency task not found")
	ErrCrossProjectDependency = errors.New("dependency must belong to the same project")
	ErrBlockedByDependency    = errors.New("blocked by an incomplete dependency")
	ErrInvalidBatch           = errors.New("invalid subtask batch")
)

// validateSuggestions checks a subtask batch before any of it touches the
// database: bounded size, non-empty titles, normalized priorities, and a
// well-formed dependency graph over array indices (in range, no self-refs,
// no duplicates, acyclic). It mutates subs in place to normalize priority
// and nil slices. Used for both AI output (untrusted model) and the apply
// endpoint's body (untrusted client) — the index graph is validated here,
// in memory, because these tasks have no IDs yet for validateDependencies
// to check against the DB.
func validateSuggestions(subs []BreakdownSuggestion) error {
	if len(subs) == 0 {
		return fmt.Errorf("%w: no subtasks", ErrInvalidBatch)
	}
	if len(subs) > 10 {
		return fmt.Errorf("%w: more than 10 subtasks", ErrInvalidBatch)
	}
	for i := range subs {
		subs[i].Title = strings.TrimSpace(subs[i].Title)
		if len(subs[i].Title) == 0 || len([]rune(subs[i].Title)) > 200 {
			return fmt.Errorf("%w: subtask %d has an empty or overlong title", ErrInvalidBatch, i)
		}
		switch subs[i].Priority {
		case "low", "medium", "high":
		default:
			subs[i].Priority = "medium"
		}
		if subs[i].Tags == nil {
			subs[i].Tags = []string{}
		}
		seen := map[int]bool{}
		deduped := subs[i].DependsOn[:0]
		for _, d := range subs[i].DependsOn {
			if d < 0 || d >= len(subs) {
				return fmt.Errorf("%w: subtask %d dependsOn index %d out of range", ErrInvalidBatch, i, d)
			}
			if d == i {
				return fmt.Errorf("%w: subtask %d depends on itself", ErrInvalidBatch, i)
			}
			if !seen[d] {
				seen[d] = true
				deduped = append(deduped, d)
			}
		}
		subs[i].DependsOn = deduped
	}

	// Cycle check over the index graph, mirroring validateDependencies'
	// walk but on positions instead of UUIDs.
	const (
		unvisited = 0
		inStack   = 1
		done      = 2
	)
	state := make([]int, len(subs))
	var walk func(i int) bool
	walk = func(i int) bool {
		if state[i] == inStack {
			return true
		}
		if state[i] == done {
			return false
		}
		state[i] = inStack
		for _, next := range subs[i].DependsOn {
			if walk(next) {
				return true
			}
		}
		state[i] = done
		return false
	}
	for i := range subs {
		if walk(i) {
			return fmt.Errorf("%w: %s", ErrInvalidBatch, ErrCyclicDependency)
		}
	}
	return nil
}

// validateDependencies checks that a proposed dependency list for taskID is
// well-formed: every dependency exists, belongs to projectID, isn't taskID
// itself, and doesn't introduce a cycle. Must be called within the same
// transaction as the mutation it's guarding, via q, so the check sees a
// consistent snapshot of the dependency graph. taskID is "" for a
// not-yet-created task (see Store.CreateTask).
func validateDependencies(ctx context.Context, q querier, taskID, projectID string, deps []string) error {
	if len(deps) == 0 {
		return nil
	}

	for _, depID := range deps {
		if depID == taskID {
			return ErrSelfDependency
		}
	}

	rows, err := q.Query(ctx, `SELECT id::text, project_id::text FROM tasks WHERE id = ANY($1::uuid[])`, deps)
	if err != nil {
		return err
	}
	found := map[string]string{} // depID -> its project ID
	for rows.Next() {
		var id, pid string
		if err := rows.Scan(&id, &pid); err != nil {
			rows.Close()
			return err
		}
		found[id] = pid
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, depID := range deps {
		pid, ok := found[depID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrDependencyNotFound, depID)
		}
		if pid != projectID {
			return fmt.Errorf("%w: %s", ErrCrossProjectDependency, depID)
		}
	}

	// Cycle check: load the project's existing dependency edges, then for
	// each candidate dependency walk its chain and see if it leads back to
	// taskID. If it does, adding the edge taskID -> depID would close a
	// cycle.
	adjacency, err := loadProjectDependencyGraph(ctx, q, projectID)
	if err != nil {
		return err
	}
	visited := map[string]bool{}
	var walk func(id string) bool
	walk = func(id string) bool {
		if id == taskID {
			return true
		}
		if visited[id] {
			return false
		}
		visited[id] = true
		for _, next := range adjacency[id] {
			if walk(next) {
				return true
			}
		}
		return false
	}
	for _, depID := range deps {
		if walk(depID) {
			return ErrCyclicDependency
		}
	}
	return nil
}

// loadProjectDependencyGraph returns, for every task in projectID that has
// dependencies, the IDs of the tasks it directly depends on.
func loadProjectDependencyGraph(ctx context.Context, q querier, projectID string) (map[string][]string, error) {
	rows, err := q.Query(ctx, `
		SELECT td.task_id::text, td.depends_on_task_id::text
		FROM task_dependencies td
		JOIN tasks t ON t.id = td.task_id
		WHERE t.project_id = $1::uuid`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	adjacency := map[string][]string{}
	for rows.Next() {
		var taskID, depID string
		if err := rows.Scan(&taskID, &depID); err != nil {
			return nil, err
		}
		adjacency[taskID] = append(adjacency[taskID], depID)
	}
	return adjacency, rows.Err()
}

// validateCanComplete checks that every dependency in deps is done. Must be
// called within the same transaction as the status transition it's
// guarding.
func validateCanComplete(ctx context.Context, q querier, deps []string) error {
	if len(deps) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, `SELECT title, status FROM tasks WHERE id = ANY($1::uuid[])`, deps)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var title string
		var status TaskStatus
		if err := rows.Scan(&title, &status); err != nil {
			return err
		}
		if status != StatusDone {
			return fmt.Errorf("%w: %q is not done", ErrBlockedByDependency, title)
		}
	}
	return rows.Err()
}
