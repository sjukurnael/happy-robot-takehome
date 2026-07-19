package main

// Integration tests for the store, validation, and event-log guarantees.
// They run against a real Postgres — the same engine the app runs on —
// because the properties under test (transactional atomicity of
// mutation+event, row-lock serialization of seq claims, cascade behavior)
// live in the database, not in Go.
//
// Run with `make test` (brings Postgres up first) or `go test ./...` with
// the compose db already running. If Postgres is unreachable the suite
// prints a loud warning and exits without failing, so a checkout without
// Docker still builds and tests cleanly.
//
// TestMain drops and recreates a dedicated `taskman_test` database and
// applies migrations/0001_init.up.sql, so tests never touch dev data.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var testStore *Store

const (
	testAdminURL = "postgres://app:app@localhost:5432/taskman?sslmode=disable"
	testDBURL    = "postgres://app:app@localhost:5432/taskman_test?sslmode=disable"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, testAdminURL)
	if err == nil {
		err = admin.Ping(ctx)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n!!! SKIPPING store tests: Postgres unreachable at %s\n!!! start it with `make db-up` (or run `make test` from the repo root)\n\n", testAdminURL)
		os.Exit(0)
	}

	if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS taskman_test WITH (FORCE)`); err != nil {
		fmt.Fprintln(os.Stderr, "drop test db:", err)
		os.Exit(1)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE taskman_test`); err != nil {
		fmt.Fprintln(os.Stderr, "create test db:", err)
		os.Exit(1)
	}
	admin.Close()

	// The migration file holds many statements in one script, which needs
	// the simple query protocol; the store's own pool below uses the
	// default (extended) protocol — the same mode production runs in.
	migrationSQL, err := os.ReadFile("migrations/0001_init.up.sql")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read migration:", err)
		os.Exit(1)
	}
	mig, err := pgxpool.New(ctx, testDBURL+"&default_query_exec_mode=simple_protocol")
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect test db:", err)
		os.Exit(1)
	}
	if _, err := mig.Exec(ctx, string(migrationSQL)); err != nil {
		fmt.Fprintln(os.Stderr, "apply migration:", err)
		os.Exit(1)
	}
	mig.Close()

	pool, err := pgxpool.New(ctx, testDBURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect test db:", err)
		os.Exit(1)
	}
	testStore = NewStore(pool)
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// --- helpers ---

func mustCreateProject(t *testing.T, name string) *Project {
	t.Helper()
	p, err := testStore.CreateProject(context.Background(), &Project{Name: name})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p
}

func mustCreateTask(t *testing.T, projectID, title string, deps []string) *Task {
	t.Helper()
	task, _, err := testStore.CreateTask(context.Background(), &Task{
		ProjectID:    projectID,
		Title:        title,
		Dependencies: deps,
	}, "test")
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return task
}

func mustUpdateStatus(t *testing.T, taskID string, status TaskStatus) {
	t.Helper()
	if _, _, err := testStore.UpdateTask(context.Background(), taskID, TaskPatch{Status: &status}, "test"); err != nil {
		t.Fatalf("set status %s: %v", status, err)
	}
}

func projectLastSeq(t *testing.T, projectID string) int64 {
	t.Helper()
	p, err := testStore.GetProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	return p.LastSeq
}

// --- projects ---

func TestProjectCRUDAndMetadataRoundtrip(t *testing.T) {
	ctx := context.Background()
	created, err := testStore.CreateProject(ctx, &Project{
		Name:        "roundtrip",
		Description: "desc",
		Metadata:    map[string]any{"color": "red", "budget": float64(12)},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := testStore.GetProject(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata["color"] != "red" || got.Metadata["budget"] != float64(12) {
		t.Errorf("metadata did not roundtrip: %#v", got.Metadata)
	}

	newName := "renamed"
	updated, events, err := testStore.UpdateProject(ctx, created.ID, ProjectPatch{Name: &newName}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "renamed" {
		t.Errorf("name = %q, want renamed", updated.Name)
	}
	if len(events) != 1 || events[0].EventType != EventProjectUpdated || events[0].Actor != "alice" {
		t.Errorf("unexpected events: %+v", events)
	}
	if updated.LastSeq != 1 {
		t.Errorf("lastSeq = %d, want 1", updated.LastSeq)
	}

	if err := testStore.DeleteProject(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.GetProject(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get deleted project: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteProjectCascades(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "cascade")
	task := mustCreateTask(t, p.ID, "t1", nil)
	if _, _, err := testStore.CreateComment(ctx, &Comment{TaskID: task.ID, Content: "hi", Author: "bob"}, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := testStore.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.GetTask(ctx, task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("task survived project delete: err = %v", err)
	}
	if events, err := testStore.ListEventsSince(ctx, p.ID, 0, 0); err != nil || len(events) != 0 {
		t.Errorf("events survived project delete: %d events, err = %v", len(events), err)
	}
}

// --- event log ---

func TestTaskLifecycleEmitsSequencedEvents(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "lifecycle")

	task := mustCreateTask(t, p.ID, "build", nil)
	other := mustCreateTask(t, p.ID, "deploy", nil)

	// One PATCH touching scalar fields and dependencies must emit two
	// events with consecutive seqs, in that order, from one transaction.
	title := "build API"
	_, events, err := testStore.UpdateTask(ctx, other.ID, TaskPatch{Title: &title, Dependencies: []string{task.ID}}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (task.updated + task.dependencies_changed)", len(events))
	}
	if events[0].EventType != EventTaskUpdated || events[1].EventType != EventTaskDependenciesChanged {
		t.Errorf("event types = %s, %s", events[0].EventType, events[1].EventType)
	}
	if events[1].Seq != events[0].Seq+1 {
		t.Errorf("seqs not consecutive: %d then %d", events[0].Seq, events[1].Seq)
	}

	// task.updated carries only the changed fields.
	var upd TaskUpdatedPayload
	if err := json.Unmarshal(events[0].Payload, &upd); err != nil {
		t.Fatal(err)
	}
	if len(upd.Changes) != 1 || upd.Changes["title"] != "build API" {
		t.Errorf("changes = %#v, want only title", upd.Changes)
	}

	// Deleting a depended-on task reports who lost the dependency.
	_, delEvents, err := testStore.DeleteTask(ctx, task.ID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	var del TaskDeletedPayload
	if err := json.Unmarshal(delEvents[0].Payload, &del); err != nil {
		t.Fatal(err)
	}
	if len(del.RemovedFromDependenciesOf) != 1 || del.RemovedFromDependenciesOf[0] != other.ID {
		t.Errorf("removedFromDependenciesOf = %v, want [%s]", del.RemovedFromDependenciesOf, other.ID)
	}
	if got, err := testStore.GetTask(ctx, other.ID); err != nil || len(got.Dependencies) != 0 {
		t.Errorf("dependency not cleaned up: deps = %v, err = %v", got.Dependencies, err)
	}

	// The whole history is contiguous from 1 and matches last_seq.
	all, err := testStore.ListEventsSince(ctx, p.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, ev := range all {
		if ev.Seq != int64(i+1) {
			t.Fatalf("gap in event log: index %d has seq %d", i, ev.Seq)
		}
	}
	if last := projectLastSeq(t, p.ID); last != int64(len(all)) {
		t.Errorf("last_seq = %d but %d events exist", last, len(all))
	}
}

func TestFailedMutationBurnsNoSeq(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "no-burn")
	task := mustCreateTask(t, p.ID, "a", nil)
	before := projectLastSeq(t, p.ID)

	// Self-dependency is rejected; the transaction must roll back the seq
	// claim along with everything else.
	if _, _, err := testStore.UpdateTask(ctx, task.ID, TaskPatch{Dependencies: []string{task.ID}}, "x"); !errors.Is(err, ErrSelfDependency) {
		t.Fatalf("err = %v, want ErrSelfDependency", err)
	}
	if after := projectLastSeq(t, p.ID); after != before {
		t.Errorf("failed mutation burned a seq: %d -> %d", before, after)
	}
	events, err := testStore.ListEventsSince(ctx, p.ID, before, 0)
	if err != nil || len(events) != 0 {
		t.Errorf("failed mutation left events: %d, err = %v", len(events), err)
	}
}

// TestConcurrentWritesKeepLogGapless is the flagship guarantee: many
// writers hitting one project concurrently must produce a strictly
// contiguous seq sequence — no gaps, no duplicates — because each
// mutation claims its seq under the project row lock inside its own
// transaction.
func TestConcurrentWritesKeepLogGapless(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "concurrent")

	const writers = 25
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := testStore.CreateTask(ctx, &Task{ProjectID: p.ID, Title: fmt.Sprintf("task %d", i)}, "test")
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	events, err := testStore.ListEventsSince(ctx, p.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != writers {
		t.Fatalf("got %d events, want %d", len(events), writers)
	}
	for i, ev := range events {
		if ev.Seq != int64(i+1) {
			t.Fatalf("gap or duplicate at index %d: seq %d", i, ev.Seq)
		}
	}
	if last := projectLastSeq(t, p.ID); last != writers {
		t.Errorf("last_seq = %d, want %d", last, writers)
	}
}

func TestListEventsSincePagination(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "pagination")
	for i := 0; i < 5; i++ {
		mustCreateTask(t, p.ID, fmt.Sprintf("t%d", i), nil)
	}
	page, err := testStore.ListEventsSince(ctx, p.ID, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Seq != 3 || page[1].Seq != 4 {
		t.Errorf("page = %v, want seqs [3 4]", page)
	}
}

// --- dependency validation ---

func TestDependencyValidation(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "deps")
	other := mustCreateProject(t, "deps-other")

	a := mustCreateTask(t, p.ID, "a", nil)
	b := mustCreateTask(t, p.ID, "b", []string{a.ID})
	c := mustCreateTask(t, p.ID, "c", []string{b.ID})
	foreign := mustCreateTask(t, other.ID, "foreign", nil)

	cases := []struct {
		name    string
		taskID  string
		deps    []string
		wantErr error
	}{
		{"self", a.ID, []string{a.ID}, ErrSelfDependency},
		{"missing", a.ID, []string{"00000000-0000-0000-0000-000000000000"}, ErrDependencyNotFound},
		{"cross-project", a.ID, []string{foreign.ID}, ErrCrossProjectDependency},
		{"cycle", a.ID, []string{c.ID}, ErrCyclicDependency}, // a <- b <- c, so a -> c closes the loop
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := testStore.UpdateTask(ctx, tc.taskID, TaskPatch{Dependencies: tc.deps}, "x")
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestCompletionBlockedByIncompleteDependency(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "blocked")
	a := mustCreateTask(t, p.ID, "a", nil)
	b := mustCreateTask(t, p.ID, "b", []string{a.ID})

	done := StatusDone
	if _, _, err := testStore.UpdateTask(ctx, b.ID, TaskPatch{Status: &done}, "x"); !errors.Is(err, ErrBlockedByDependency) {
		t.Fatalf("completing blocked task: err = %v, want ErrBlockedByDependency", err)
	}

	mustUpdateStatus(t, a.ID, StatusDone)
	mustUpdateStatus(t, b.ID, StatusDone) // now allowed
	if got, _ := testStore.GetTask(ctx, b.ID); got.Status != StatusDone {
		t.Errorf("status = %s, want done", got.Status)
	}
}

// --- comments ---

func TestCommentLifecycle(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "comments")
	task := mustCreateTask(t, p.ID, "t", nil)

	if _, _, err := testStore.CreateComment(ctx, &Comment{TaskID: "00000000-0000-0000-0000-000000000000", Content: "x", Author: "a"}, "a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("comment on missing task: err = %v, want ErrNotFound", err)
	}

	created, events, err := testStore.CreateComment(ctx, &Comment{TaskID: task.ID, Content: "hello", Author: "alice"}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].EventType != EventCommentAdded {
		t.Errorf("event type = %s", events[0].EventType)
	}

	gotTaskID, gotProjectID, delEvents, err := testStore.DeleteComment(ctx, created.ID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if gotTaskID != task.ID || gotProjectID != p.ID {
		t.Errorf("resolved (%s, %s), want (%s, %s)", gotTaskID, gotProjectID, task.ID, p.ID)
	}
	if delEvents[0].EventType != EventCommentDeleted {
		t.Errorf("event type = %s", delEvents[0].EventType)
	}
	if comments, _ := testStore.ListCommentsByTask(ctx, task.ID); len(comments) != 0 {
		t.Errorf("comment survived delete")
	}
}

// --- stats ---

func TestProjectStats(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "stats")

	a := mustCreateTask(t, p.ID, "a", nil)
	mustUpdateStatus(t, a.ID, StatusDone)
	if _, _, err := testStore.UpdateTask(ctx, a.ID, TaskPatch{AssignedTo: []string{"alice", "bob"}}, "x"); err != nil {
		t.Fatal(err)
	}
	b := mustCreateTask(t, p.ID, "b", []string{a.ID}) // dep done -> not blocked
	if _, _, err := testStore.UpdateTask(ctx, b.ID, TaskPatch{AssignedTo: []string{"bob"}}, "x"); err != nil {
		t.Fatal(err)
	}
	mustCreateTask(t, p.ID, "c", []string{b.ID}) // b not done -> blocked

	all, err := testStore.ListProjectStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var st *ProjectStats
	for _, s := range all {
		if s.ProjectID == p.ID {
			st = s
		}
	}
	if st == nil {
		t.Fatal("project missing from stats")
	}
	if st.Total != 3 || st.Done != 1 || st.Blocked != 1 {
		t.Errorf("total/done/blocked = %d/%d/%d, want 3/1/1", st.Total, st.Done, st.Blocked)
	}
	if len(st.Assignees) != 2 {
		t.Errorf("assignees = %v, want alice+bob deduped", st.Assignees)
	}
}

// --- AI breakdown batch ---

func TestCreateTaskBatchMapsIndicesAndSetsFinishLine(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "batch")
	pre := mustCreateTask(t, p.ID, "pre-existing dep", nil)
	parent := mustCreateTask(t, p.ID, "big task", []string{pre.ID})
	seqBefore := projectLastSeq(t, p.ID)

	created, events, err := testStore.CreateTaskBatch(ctx, parent.ID, []BreakdownSuggestion{
		{Title: "A", Priority: "high", Tags: []string{"x"}},
		{Title: "B"},
		{Title: "C", DependsOn: []int{0, 1}},
	}, "ai-user")
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 3 {
		t.Fatalf("created %d tasks, want 3", len(created))
	}

	// C's index deps mapped onto A and B's new UUIDs.
	wantDeps := map[string]bool{created[0].ID: true, created[1].ID: true}
	if len(created[2].Dependencies) != 2 || !wantDeps[created[2].Dependencies[0]] || !wantDeps[created[2].Dependencies[1]] {
		t.Errorf("C.Dependencies = %v, want A+B ids", created[2].Dependencies)
	}
	if created[0].Configuration.Priority != "high" || created[0].Configuration.Tags[0] != "x" {
		t.Errorf("configuration not carried: %#v", created[0].Configuration)
	}

	// Parent keeps its pre-existing dep and gains only the terminal C —
	// nothing in the batch depends on C, while A and B are covered
	// transitively through it.
	got, err := testStore.GetTask(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotDeps := map[string]bool{}
	for _, d := range got.Dependencies {
		gotDeps[d] = true
	}
	if len(got.Dependencies) != 2 || !gotDeps[pre.ID] || !gotDeps[created[2].ID] {
		t.Errorf("parent deps = %v, want [pre, C]", got.Dependencies)
	}

	// 3 task.created in creation order, then the parent's
	// dependencies_changed, with gapless consecutive seqs.
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4", len(events))
	}
	for i, ev := range events {
		if ev.Seq != seqBefore+int64(i)+1 {
			t.Errorf("event %d seq = %d, want %d", i, ev.Seq, seqBefore+int64(i)+1)
		}
		if i < 3 && ev.EventType != EventTaskCreated {
			t.Errorf("event %d type = %s, want task.created", i, ev.EventType)
		}
	}
	if events[3].EventType != EventTaskDependenciesChanged {
		t.Errorf("last event type = %s, want task.dependencies_changed", events[3].EventType)
	}
	var depPayload TaskDependenciesChangedPayload
	if err := json.Unmarshal(events[3].Payload, &depPayload); err != nil {
		t.Fatal(err)
	}
	if depPayload.TaskID != parent.ID || len(depPayload.DependsOn) != 2 {
		t.Errorf("dependencies_changed payload = %+v, want parent with 2 deps", depPayload)
	}
	if projectLastSeq(t, p.ID) != seqBefore+4 {
		t.Errorf("lastSeq = %d, want %d", projectLastSeq(t, p.ID), seqBefore+4)
	}
}

func TestCreateTaskBatchAtomicity(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "batch-atomic")
	parent := mustCreateTask(t, p.ID, "parent", nil)
	seqBefore := projectLastSeq(t, p.ID)

	cases := map[string][]BreakdownSuggestion{
		"empty batch":        {},
		"out-of-range index": {{Title: "A", DependsOn: []int{5}}},
		"self dependency":    {{Title: "A", DependsOn: []int{0}}},
		"empty title":        {{Title: "   "}},
		"cycle":              {{Title: "A", DependsOn: []int{1}}, {Title: "B", DependsOn: []int{0}}},
	}
	for name, subs := range cases {
		if _, _, err := testStore.CreateTaskBatch(ctx, parent.ID, subs, "test"); !errors.Is(err, ErrInvalidBatch) {
			t.Errorf("%s: err = %v, want ErrInvalidBatch", name, err)
		}
	}

	tasks, err := testStore.ListTasksByProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Errorf("task count = %d after failed batches, want 1", len(tasks))
	}
	if got := projectLastSeq(t, p.ID); got != seqBefore {
		t.Errorf("lastSeq = %d after failed batches, want %d (no burned seqs)", got, seqBefore)
	}
}

func TestCreateTaskBatchParentNotFound(t *testing.T) {
	_, _, err := testStore.CreateTaskBatch(context.Background(), "00000000-0000-0000-0000-000000000000",
		[]BreakdownSuggestion{{Title: "A"}}, "test")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateTaskBatchFinishLineBlocksParentCompletion(t *testing.T) {
	ctx := context.Background()
	p := mustCreateProject(t, "batch-finish-line")
	parent := mustCreateTask(t, p.ID, "parent", nil)

	created, _, err := testStore.CreateTaskBatch(ctx, parent.ID, []BreakdownSuggestion{
		{Title: "step 1"},
		{Title: "step 2", DependsOn: []int{0}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}

	done := StatusDone
	if _, _, err := testStore.UpdateTask(ctx, parent.ID, TaskPatch{Status: &done}, "test"); !errors.Is(err, ErrBlockedByDependency) {
		t.Fatalf("completing parent: err = %v, want ErrBlockedByDependency", err)
	}
	// step 2 is itself blocked until step 1 is done.
	if _, _, err := testStore.UpdateTask(ctx, created[1].ID, TaskPatch{Status: &done}, "test"); !errors.Is(err, ErrBlockedByDependency) {
		t.Fatalf("completing step 2 early: err = %v, want ErrBlockedByDependency", err)
	}
	mustUpdateStatus(t, created[0].ID, StatusDone)
	mustUpdateStatus(t, created[1].ID, StatusDone)
	if _, _, err := testStore.UpdateTask(ctx, parent.ID, TaskPatch{Status: &done}, "test"); err != nil {
		t.Fatalf("completing parent after subtasks: %v", err)
	}
}
