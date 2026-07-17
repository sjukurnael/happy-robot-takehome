package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

// Store is a naive in-memory store guarded by a single mutex.
// This is intentionally the simplest thing that works — it will be
// replaced by SQLite once the rest of the system is proven out.
type Store struct {
	mu       sync.RWMutex
	projects map[string]*Project
	tasks    map[string]*Task
	comments map[string]*Comment
}

func NewStore() *Store {
	return &Store{
		projects: map[string]*Project{},
		tasks:    map[string]*Task{},
		comments: map[string]*Comment{},
	}
}

func newID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, uuid.NewString())
}

// --- Projects ---

func (s *Store) CreateProject(p *Project) *Project {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.ID = newID("proj")
	p.CreatedAt = time.Now().UTC()
	p.UpdatedAt = p.CreatedAt
	s.projects[p.ID] = p
	return p
}

func (s *Store) ListProjects() []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Project, 0, len(s.projects))
	for _, p := range s.projects {
		out = append(out, p)
	}
	return out
}

func (s *Store) GetProject(id string) (*Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	return p, ok
}

func (s *Store) UpdateProject(id string, fn func(*Project)) (*Project, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return nil, false
	}
	fn(p)
	p.UpdatedAt = time.Now().UTC()
	return p, true
}

func (s *Store) DeleteProject(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[id]; !ok {
		return false
	}
	delete(s.projects, id)
	for tid, t := range s.tasks {
		if t.ProjectID == id {
			delete(s.tasks, tid)
			for cid, c := range s.comments {
				if c.TaskID == tid {
					delete(s.comments, cid)
				}
			}
		}
	}
	return true
}

// --- Tasks ---

func (s *Store) CreateTask(t *Task) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newID("task")
	// A brand-new task can't be anyone's dependency yet, so no cycle is
	// possible on creation — this still checks existence/same-project/self-ref.
	if err := validateDependencies(id, t.ProjectID, t.Dependencies, s.tasks); err != nil {
		return nil, err
	}
	t.ID = id
	t.CreatedAt = time.Now().UTC()
	t.UpdatedAt = t.CreatedAt
	if t.Status == "" {
		t.Status = StatusTodo
	}
	// Normalize nil slices/maps to empty so the JSON contract is always
	// `[]`/`{}`, never `null` — one less null-check every API consumer
	// would otherwise need.
	if t.Dependencies == nil {
		t.Dependencies = []string{}
	}
	if t.AssignedTo == nil {
		t.AssignedTo = []string{}
	}
	if t.Configuration.Tags == nil {
		t.Configuration.Tags = []string{}
	}
	if t.Configuration.CustomFields == nil {
		t.Configuration.CustomFields = map[string]any{}
	}
	s.tasks[t.ID] = t
	return t, nil
}

func (s *Store) ListTasksByProject(projectID string) []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Task{}
	for _, t := range s.tasks {
		if t.ProjectID == projectID {
			out = append(out, t)
		}
	}
	return out
}

func (s *Store) GetTask(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	return t, ok
}

// UpdateTask applies fn to the task under the store's lock so that
// validation against other tasks (e.g. dependency cycle/completion checks)
// and the mutation itself are atomic — fn is given the full tasks map to
// validate against, but must not mutate anything other than t. If fn
// returns an error, the mutation is discarded and UpdatedAt is left
// untouched.
func (s *Store) UpdateTask(id string, fn func(t *Task, tasks map[string]*Task) error) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	if err := fn(t, s.tasks); err != nil {
		return nil, err
	}
	t.UpdatedAt = time.Now().UTC()
	return t, nil
}

// DeleteTask removes a task and its comments, and strips the deleted ID out
// of any other task's Dependencies list. It returns the deleted task and the
// IDs of any tasks whose dependencies were modified, so callers can notify
// clients about both.
func (s *Store) DeleteTask(id string) (*Task, []string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, nil, false
	}
	delete(s.tasks, id)
	for cid, c := range s.comments {
		if c.TaskID == id {
			delete(s.comments, cid)
		}
	}
	var affected []string
	for tid, other := range s.tasks {
		for i, depID := range other.Dependencies {
			if depID == id {
				other.Dependencies = append(other.Dependencies[:i], other.Dependencies[i+1:]...)
				other.UpdatedAt = time.Now().UTC()
				affected = append(affected, tid)
				break
			}
		}
	}
	return t, affected, true
}

// --- Comments ---

func (s *Store) CreateComment(c *Comment) *Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = newID("cmt")
	c.Timestamp = time.Now().UTC()
	s.comments[c.ID] = c
	return c
}

func (s *Store) ListCommentsByTask(taskID string) []*Comment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Comment{}
	for _, c := range s.comments {
		if c.TaskID == taskID {
			out = append(out, c)
		}
	}
	return out
}

func (s *Store) DeleteComment(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.comments[id]; !ok {
		return false
	}
	delete(s.comments, id)
	return true
}
