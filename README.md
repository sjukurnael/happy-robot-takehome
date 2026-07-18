# Task Manager

Collaborative task management system with near-real-time sync across clients.

- **Backend**: Go (`server/`) — chi router, Postgres-backed store (via `pgx`), WebSocket hub for live updates
- **Frontend**: Vanilla React + Vite + TypeScript (`client/`)

This is a work in progress, built up incrementally. See commit history for the
order features were added in. A full architecture write-up (sync strategy,
scaling plan, tradeoffs) will be added here as the system matures.

## Prerequisites

To run this locally you need three things installed:

- **Docker Desktop** (or Docker Engine + the Compose plugin) — runs
  Postgres and the migration tool. Nothing else needs to be installed for
  the database; see [Database](#database) below. Get it at
  <https://www.docker.com/products/docker-desktop/>.
- **Go 1.25+** — to run the backend (`cd server && go run .`).
- **Node.js + npm** — to run the frontend (`cd client && npm install && npm run dev`).

The backend reads and writes Postgres directly (via `pgx`) — there's no more
in-memory store. That means **the database must be running before you start
the Go server**: run `make db-up` (or `make db-reset` for a clean slate)
first, then `cd server && go run .`.

## Database

Postgres 16, run via Docker, with schema migrations and seed data. Every
mutation claims a per-project sequence number (`projects.last_seq`) and
appends a row to the append-only `events` table — see
[Sync: the event log](#sync-the-event-log) below for how that's used.

**Prereqs:** Docker only (no local Postgres, no local `golang-migrate`).

```sh
make db-reset    # tear down, start postgres, run migrations, load seed data
```

Other targets:

```sh
make db-up       # start postgres and wait for it to be healthy
make db-migrate  # run migrations (via the migrate/migrate Docker image)
make db-seed     # load server/seed/seed.sql
make db-psql     # open a psql shell against the running db
```

**Schema** (`server/migrations/0001_init.up.sql`): `projects`, `tasks`,
`task_dependencies` (join table, not an array column, so dependency edges
have real FK/cascade integrity), `comments`, and `events`. Notes:

- `tasks.status` is `todo | in_progress | done` only — there's no stored
  `blocked` status. Whether a task is blocked is derived from its
  dependencies' statuses at read time (as the app already does), so it can
  never go stale relative to the tasks it depends on.
- Cross-project dependency prevention (a task and its dependency must share
  a project) is enforced in application code, not a DB constraint, since it
  needs a cross-row comparison a `CHECK` can't express.
- `updated_at` is maintained by application code (`server/store.go` sets it
  to `now()` on every update), not a DB trigger.

**Seed data** (`server/seed/seed.sql`): two projects with fixed UUIDs —
"Website Redesign" (8 tasks spanning todo/in_progress/done, a dependency
chain where `Deploy` depends on `Build API` and `Write tests`, and `Write
tests` depends on `Build API`, plus 5 comments from `alice`/`bob` on 2
tasks) and "Mobile App" (3 simple tasks, no dependencies). `last_seq` is 0
and `events` is empty for both — event history starts once the API exists.

## Sync: the event log

Every mutation (task create/update/delete, dependency change, comment
add/delete, project update) runs in one DB transaction that: claims the
next `seq` for that project (`UPDATE projects SET last_seq = last_seq + 1
... RETURNING last_seq` — the row lock this takes serializes concurrent
writers to the same project, which is intentional, not a bottleneck at
this scale), performs the actual mutation, and inserts one row into
`events`. Only after that transaction commits does the server broadcast
the event over WebSocket. If anything fails, the whole transaction rolls
back — the `seq` is never burned and no event is ever recorded without its
state change, so the log is always gapless and an event's existence always
means its mutation really happened. (`project.created`/`project.deleted`
are deliberately not part of this log — deleting a project would cascade-
delete its own event rows before any client could see them, and project-
list membership isn't something a client watching one project's internals
needs to replay anyway. They keep the old thin `{type, projectId}`
broadcast instead.)

**Payload shapes** (`server/events.go`): `task.created` carries the full
new task; `task.updated` carries only the changed scalar fields;
`task.dependencies_changed` is always its own event (a single PATCH
touching both scalar fields and dependencies emits two events, two
sequential `seq` numbers, one transaction); `task.deleted` carries the IDs
of other tasks whose dependency list just lost this one
(`removedFromDependenciesOf`), so clients patch those locally instead of
needing a separate event per affected task; `comment.added`/
`comment.deleted` and `project.updated` are similarly minimal. Every
mutating request is attributed via an `X-Actor` header (the user's display
name — see `client/src/identity.ts`), stored as the event's `actor`.

**Catching up.** `GET /api/projects/:id/events?after=<seq>&limit=<n>`
returns events with `seq > after`, ascending, capped at 500. A fresh
client seeds its `lastSeq` from the project snapshot's `lastSeq` field
(not from this endpoint — `after=0` would walk the entire history). Over
WebSocket, the `viewing` message now carries `lastSeq`; on an actual
project switch (not just moving focus between tasks in the same project)
the server replays everything the client missed directly to that
connection, in order, before it's otherwise eligible for live broadcasts.

**Client-side gap detection** (`client/src/ProjectDetail.tsx`): every
incoming event is checked against a locally-tracked `lastSeq`. Equal to
`lastSeq + 1` → apply directly and advance. Less than or equal → duplicate,
ignore. Greater → a gap: buffer the event, fetch the missing range from
the catch-up endpoint, apply it, then drain the buffer (which may itself
still be gapped if more arrived mid-fetch, in which case it fetches again).
Reconnecting after a dropped connection uses the same mechanism —
resubscribe with the last known `seq` and the gap gets replayed. Applying
a payload is idempotent (e.g. `task.created` dedupes by id before
appending), which is what makes it safe to reapply events that arrive both
via subscribe-time replay and an independent REST fetch.

Not implemented: a "client is way too far behind, just refetch a fresh
snapshot instead of replaying thousands of events" fallback — noted as a
TODO in `server/hub.go` (`replayTo`) and `client/src/ProjectDetail.tsx`
(`fillGap`), since ordinary reconnect gaps are small and `ListEventsSince`
caps a single query at 500 rows regardless.

## Running locally

### Backend

Requires the database to be up first (`make db-up` from the repo root).

```sh
cd server
go run .
```

Serves the API and WebSocket endpoint on `:8080` (override with `PORT`).
Connects to Postgres via `DATABASE_URL` (defaults to
`postgres://app:app@localhost:5432/taskman?sslmode=disable`, matching
`docker-compose.yml`, so no extra config is needed for local dev). The
server fails fast on startup if it can't reach the database.

### Frontend

```sh
cd client
npm install
npm run dev
```

Vite dev server proxies `/api` and `/ws` — see `client/vite.config.ts`.
