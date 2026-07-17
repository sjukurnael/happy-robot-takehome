package main

import (
	"errors"
	"fmt"
)

var (
	ErrSelfDependency         = errors.New("a task cannot depend on itself")
	ErrCyclicDependency       = errors.New("dependency would create a cycle")
	ErrDependencyNotFound     = errors.New("dependency task not found")
	ErrCrossProjectDependency = errors.New("dependency must belong to the same project")
	ErrBlockedByDependency    = errors.New("blocked by an incomplete dependency")
)

// validateDependencies checks that a proposed dependency list for taskID is
// well-formed: every dependency exists, belongs to the same project, isn't
// taskID itself, and doesn't introduce a cycle. It must be called while
// holding the store's lock, since it reads the full tasks map.
func validateDependencies(taskID, projectID string, deps []string, tasks map[string]*Task) error {
	for _, depID := range deps {
		if depID == taskID {
			return ErrSelfDependency
		}
		dep, ok := tasks[depID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrDependencyNotFound, depID)
		}
		if dep.ProjectID != projectID {
			return fmt.Errorf("%w: %s", ErrCrossProjectDependency, depID)
		}
	}

	// Cycle check: for each candidate dependency, walk its dependency chain
	// and see if it leads back to taskID. If it does, adding the edge
	// taskID -> depID would close a cycle.
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
		t, ok := tasks[id]
		if !ok {
			return false
		}
		for _, next := range t.Dependencies {
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

// validateCanComplete checks that every dependency in deps is done. Must be
// called while holding the store's lock.
func validateCanComplete(deps []string, tasks map[string]*Task) error {
	for _, depID := range deps {
		dep, ok := tasks[depID]
		if !ok {
			continue
		}
		if dep.Status != StatusDone {
			return fmt.Errorf("%w: %q is not done", ErrBlockedByDependency, dep.Title)
		}
	}
	return nil
}
