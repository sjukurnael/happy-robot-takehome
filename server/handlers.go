package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type API struct {
	store *Store
	hub   *Hub
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- Projects ---

func (a *API) listProjects(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.store.ListProjects())
}

func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	var p Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	created := a.store.CreateProject(&p)
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) getProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	p, ok := a.store.GetProject(id)
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) updateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	var patch struct {
		Name        *string        `json:"name"`
		Description *string        `json:"description"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	p, ok := a.store.UpdateProject(id, func(p *Project) {
		if patch.Name != nil {
			p.Name = *patch.Name
		}
		if patch.Description != nil {
			p.Description = *patch.Description
		}
		if patch.Metadata != nil {
			p.Metadata = patch.Metadata
		}
	})
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	a.hub.Broadcast(Event{Type: "project.updated", ProjectID: id, ResourceID: id})
	writeJSON(w, http.StatusOK, p)
}

func (a *API) deleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	if !a.store.DeleteProject(id) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	a.hub.Broadcast(Event{Type: "project.deleted", ProjectID: id, ResourceID: id})
	w.WriteHeader(http.StatusNoContent)
}

// --- Tasks ---

func (a *API) listTasks(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	writeJSON(w, http.StatusOK, a.store.ListTasksByProject(projectID))
}

func (a *API) createTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if _, ok := a.store.GetProject(projectID); !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var t Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	t.ProjectID = projectID
	created := a.store.CreateTask(&t)
	a.hub.Broadcast(Event{Type: "task.created", ProjectID: projectID, ResourceID: created.ID})
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) getTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "taskID")
	t, ok := a.store.GetTask(id)
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *API) updateTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "taskID")
	var patch struct {
		Title         *string            `json:"title"`
		Status        *TaskStatus        `json:"status"`
		AssignedTo    []string           `json:"assignedTo"`
		Configuration *TaskConfiguration `json:"configuration"`
		Dependencies  []string           `json:"dependencies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	t, ok := a.store.UpdateTask(id, func(t *Task) {
		if patch.Title != nil {
			t.Title = *patch.Title
		}
		if patch.Status != nil {
			t.Status = *patch.Status
		}
		if patch.AssignedTo != nil {
			t.AssignedTo = patch.AssignedTo
		}
		if patch.Configuration != nil {
			t.Configuration = *patch.Configuration
		}
		if patch.Dependencies != nil {
			t.Dependencies = patch.Dependencies
		}
	})
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	a.hub.Broadcast(Event{Type: "task.updated", ProjectID: t.ProjectID, ResourceID: t.ID})
	writeJSON(w, http.StatusOK, t)
}

func (a *API) deleteTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "taskID")
	t, ok := a.store.DeleteTask(id)
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	a.hub.Broadcast(Event{Type: "task.deleted", ProjectID: t.ProjectID, ResourceID: id})
	w.WriteHeader(http.StatusNoContent)
}

// --- Comments ---

func (a *API) listComments(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	writeJSON(w, http.StatusOK, a.store.ListCommentsByTask(taskID))
}

func (a *API) createComment(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	task, ok := a.store.GetTask(taskID)
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	var c Comment
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	c.TaskID = taskID
	created := a.store.CreateComment(&c)
	a.hub.Broadcast(Event{Type: "comment.created", ProjectID: task.ProjectID, ResourceID: created.ID})
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) deleteComment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "commentID")
	if !a.store.DeleteComment(id) {
		writeError(w, http.StatusNotFound, "comment not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
