package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

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

// writeStoreError maps a store/validation error to the appropriate HTTP
// status: 404 for missing resources, 409 for a blocked status transition,
// 400 for malformed dependency references, 500 otherwise.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrBlockedByDependency):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrSelfDependency),
		errors.Is(err, ErrCyclicDependency),
		errors.Is(err, ErrDependencyNotFound),
		errors.Is(err, ErrCrossProjectDependency):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		// Unexpected errors are logged server-side but never echoed to the
		// client — raw driver/SQL error strings are an internals leak.
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// actorFrom identifies who's making a mutating request, for the events
// table's actor column. The frontend sends the user's display name (see
// identity.ts) on every mutating call; anything else (curl, a missing
// header) just gets attributed to "unknown" rather than rejected.
func actorFrom(r *http.Request) string {
	if a := r.Header.Get("X-Actor"); a != "" {
		return a
	}
	return "unknown"
}

// broadcastEvents publishes durable events over the hub after their
// transaction has already committed — store methods return events only
// once the mutation that produced them is safely persisted.
func (a *API) broadcastEvents(events []StoredEvent) {
	for _, ev := range events {
		a.hub.BroadcastEvent(ev)
	}
}

// --- Projects ---

func (a *API) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := a.store.ListProjects(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

// listProjectStats serves the dashboard's per-project aggregates (task
// counts, blocked counts, assignees, last-edited) in one response, so the
// project list never has to download every task of every project just to
// render stat cards.
func (a *API) listProjectStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.ListProjectStats(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	var p Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	created, err := a.store.CreateProject(r.Context(), &p)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	// project.created, like project.deleted, lives outside the per-project
	// event log (see events.go) — a project that doesn't exist yet has no
	// log to append to, and clients watching the dashboard only need a
	// "refetch the list" nudge, not a replayable event.
	a.hub.Broadcast(Event{Type: "project.created", ProjectID: created.ID, ResourceID: created.ID})
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) getProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	p, err := a.store.GetProject(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) updateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	var patch ProjectPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	p, events, err := a.store.UpdateProject(r.Context(), id, patch, actorFrom(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	a.broadcastEvents(events)
	writeJSON(w, http.StatusOK, p)
}

func (a *API) deleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	if err := a.store.DeleteProject(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	// project.deleted is intentionally not part of the per-project event
	// log (see events.go) — a project deleting itself would cascade-erase
	// its own event history, including this notification, before any
	// client could replay it. Kept as the old thin broadcast instead.
	a.hub.Broadcast(Event{Type: "project.deleted", ProjectID: id, ResourceID: id})
	w.WriteHeader(http.StatusNoContent)
}

// --- Tasks ---

// listTasks returns a project's tasks, with the project's lastSeq as the
// response ETag. lastSeq is bumped by every mutation in the project, so it
// doubles as a version stamp for this snapshot: Cache-Control: no-cache
// makes the browser revalidate with If-None-Match on each request, and an
// unchanged project answers 304 with no body — the client transparently
// reuses its cached copy (no api.ts changes needed), so a large task list
// only crosses the wire when something actually changed.
//
// The read order matters and mirrors the client's refresh(): lastSeq is
// read before the tasks, so the task snapshot is at-or-newer than the
// ETag. The ETag can understate what the body contains, which only causes
// one redundant refetch later — never a stale cache hit, because any
// mutation that slipped between the two reads has already bumped lastSeq
// past the ETag being returned.
func (a *API) listTasks(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	p, err := a.store.GetProject(r.Context(), projectID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	etag := fmt.Sprintf(`"%d"`, p.LastSeq)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	tasks, err := a.store.ListTasksByProject(r.Context(), projectID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (a *API) createTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if _, err := a.store.GetProject(r.Context(), projectID); err != nil {
		writeStoreError(w, err)
		return
	}
	var t Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	t.ProjectID = projectID
	created, events, err := a.store.CreateTask(r.Context(), &t, actorFrom(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	a.broadcastEvents(events)
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) getTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "taskID")
	t, err := a.store.GetTask(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *API) updateTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "taskID")
	var patch TaskPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	t, events, err := a.store.UpdateTask(r.Context(), id, patch, actorFrom(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	a.broadcastEvents(events)
	writeJSON(w, http.StatusOK, t)
}

func (a *API) deleteTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "taskID")
	_, events, err := a.store.DeleteTask(r.Context(), id, actorFrom(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	a.broadcastEvents(events)
	w.WriteHeader(http.StatusNoContent)
}

// --- Comments ---

func (a *API) listComments(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	comments, err := a.store.ListCommentsByTask(r.Context(), taskID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

func (a *API) createComment(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	var c Comment
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	c.TaskID = taskID
	created, events, err := a.store.CreateComment(r.Context(), &c, actorFrom(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	a.broadcastEvents(events)
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) deleteComment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "commentID")
	_, _, events, err := a.store.DeleteComment(r.Context(), id, actorFrom(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	a.broadcastEvents(events)
	w.WriteHeader(http.StatusNoContent)
}

// --- Events (catch-up) ---

// listEvents serves GET /api/projects/{projectID}/events?after=<seq>&limit=<n>
// — events with seq > after, ordered ascending, capped at 500. Passing
// after=0 (or omitting it) returns the project's entire history rather
// than nothing; that's technically valid but not the intended use — a
// fresh client should seed lastSeq from the project snapshot's lastSeq
// field instead of walking the whole log from zero.
func (a *API) listEvents(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := a.store.ListEventsSince(r.Context(), projectID, after, limit)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}
