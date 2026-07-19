# Task Manager

Collaborative task management system with near-real-time sync across clients.

- **Backend**: Go (`server/`) — chi router, Postgres-backed store (via `pgx`), WebSocket hub for live updates, an append-only event log for efficient sync
- **Frontend**: Vanilla React + Vite + TypeScript (`client/`)

## Contents

- [Quick start (Docker, one command)](#quick-start-docker-one-command)
- [Code architecture](#code-architecture)
- [Technology choices](#technology-choices)
- [Database](#database)
- [Sync: the event log](#sync-the-event-log)
- [How I'd scale it over time](#how-id-scale-it-over-time)
- [Tradeoffs](#tradeoffs)
- [Running locally (native dev)](#running-locally-native-dev)

## Quick start (Docker, one command)

With Docker Desktop running:

```sh
make up       # builds + starts Postgres, migrations, Go server, frontend
make db-seed  # optional: load demo projects/tasks/comments
```

Then open <http://localhost:3000> — open it in two browser windows to see
the real-time sync and presence. `make down` stops everything;
`make db-reset` wipes the database. No local Go or Node needed for this
path. For hacking on the code itself, see
[Running locally (native dev)](#running-locally-native-dev).

## Code architecture

### Repo layout

```
.
├── docker-compose.yml     # db only (default) or full stack (--profile app)
├── Makefile                # up / down / db-up / db-migrate / db-seed / db-reset / db-psql
├── server/                 # Go backend
│   ├── Dockerfile           # multi-stage build for the full-stack compose profile
│   ├── main.go              # entrypoint: DB pool, router, route table
│   ├── db.go                 # pgxpool connection + startup health check
│   ├── models.go              # Project / Task / Comment structs — the JSON API contract
│   ├── store.go                # all Postgres reads/writes; the only file that writes SQL
│   ├── validation.go            # dependency cycle / cross-project / completion checks
│   ├── events.go                 # event type constants, payload structs, recordEvent, ListEventsSince
│   ├── handlers.go                # HTTP handlers: decode request -> call store -> broadcast -> respond
│   ├── hub.go                      # WebSocket registry, presence, event broadcast + subscribe-time replay
│   ├── migrations/0001_init.{up,down}.sql   # schema (golang-migrate)
│   └── seed/seed.sql                          # demo projects/tasks/comments
├── client/
│   ├── Dockerfile           # build + nginx image for the full-stack compose profile
│   └── nginx.conf           # serves the SPA, proxies /api and /ws to the server
└── client/src/              # React frontend
    ├── main.tsx, App.tsx     # entry point + top-level view switch (project list <-> project board)
    ├── types.ts                # TS mirror of the Go JSON contract, including event payload shapes
    ├── api.ts                   # typed fetch wrapper for the REST API
    ├── live.ts                    # the single shared WebSocket connection for the whole tab
    ├── identity.ts                 # per-tab display name/clientId (sessionStorage), avatar colors
    ├── useWsEvents.ts                # React hooks over live.ts: raw events, presence, connection status
    ├── ProjectList.tsx                 # project grid: search, create/delete, per-project stats
    ├── ProjectDetail.tsx                 # board page — owns tasks/project state and the sync/gap-detection logic
    ├── KanbanBoard.tsx, TaskCard.tsx       # drag-and-drop board (dnd-kit)
    ├── TaskPanel.tsx                        # task detail side panel: fields, dependencies, comments
    ├── NewTaskForm.tsx                       # task creation modal
    ├── Presence.tsx, Avatar.tsx,
    │   IdentityBadge.tsx                      # "who's here" UI (viewer avatars, rename-yourself badge)
    ├── taskUtils.ts, format.ts                 # small pure helpers (blocked-task check, relative timestamps)
    └── ThemeToggle.tsx                           # light/dark toggle
```

### Backend

`main.go` builds a Postgres connection pool (`db.go`), constructs a `Store`
(`store.go`) and a `Hub` (`hub.go`), and wires them into a chi router. Every
HTTP handler in `handlers.go` follows the same shape: decode the request
body, call one `Store` method, and — if that method returned any durable
events — broadcast them over the hub, then write the JSON response.

`store.go` is the only file that talks SQL. Every mutating method
(`CreateTask`, `UpdateTask`, `DeleteTask`, `CreateComment`, `DeleteComment`,
`UpdateProject`) runs entirely inside one Postgres transaction: it performs
the write, validates via `validation.go` where relevant (dependency cycles,
cross-project references, "can't complete while blocked"), and — through
`events.go`'s `recordEvent` — claims a sequence number and appends an
`events` row, all before committing. See
[Sync: the event log](#sync-the-event-log) for why.

`hub.go` tracks connected WebSocket clients and does three things: relays
presence ("who's viewing what," driven by the client's `viewing` messages),
broadcasts durable events to every connected client after their transaction
commits, and — when a client subscribes to a project — replays anything
that client missed directly to its connection first.

### Frontend

`App.tsx` switches between two top-level views: `ProjectList` (the
dashboard) and `ProjectDetail` (a single project's board). Both fetch their
initial data over REST via `api.ts`.

`live.ts` opens exactly one WebSocket per browser tab (not one per
component) and exposes a small pub-sub — `useWsEvents.ts` wraps it in React
hooks (`useWsEvents` for raw messages, `usePresence`/`useAllProjectsPresence`
for roster state, `useConnectionStatus` for the "Live"/"Reconnecting…"
pill). `identity.ts` gives each tab a stable display name and client ID via
`sessionStorage`, so opening two tabs naturally simulates two collaborators.

`ProjectDetail.tsx` is where the real-time sync logic lives: it seeds a
`lastSeq` from the project snapshot, subscribes over WS with it, and from
then on applies incoming event payloads directly to local `tasks`/`project`
state instead of refetching — including gap detection and a catch-up fetch
if it ever misses one (see [Sync](#sync-the-event-log)). `TaskPanel.tsx`
does the same in miniature for a task's comments. `KanbanBoard`/`TaskCard`
handle drag-and-drop status changes (optimistic, with rollback on a
rejected request — e.g. moving a blocked task to Done).

### REST API reference

All routes are rooted at `/api`; all mutating routes accept an `X-Actor`
header (see [Sync](#sync-the-event-log)) and every response/request body is
JSON.

| Method | Path | Purpose |
|---|---|---|
| GET | `/health` | liveness check |
| GET | `/ws` | WebSocket upgrade |
| GET | `/projects/` | list projects |
| POST | `/projects/` | create project |
| GET | `/projects/{projectID}/` | get one project (includes `lastSeq`) |
| PATCH | `/projects/{projectID}/` | update name/description/metadata |
| DELETE | `/projects/{projectID}/` | delete a project and everything under it |
| GET | `/projects/{projectID}/tasks` | list a project's tasks |
| POST | `/projects/{projectID}/tasks` | create a task |
| GET | `/projects/{projectID}/events?after=&limit=` | catch-up: events since `after`, capped at `limit` (max 500) |
| GET | `/tasks/{taskID}/` | get one task |
| PATCH | `/tasks/{taskID}/` | update title/status/assignedTo/configuration/dependencies |
| DELETE | `/tasks/{taskID}/` | delete a task |
| GET | `/tasks/{taskID}/comments` | list a task's comments |
| POST | `/tasks/{taskID}/comments` | add a comment |
| DELETE | `/comments/{commentID}` | delete a comment |

### WebSocket protocol

Client → server:
- `{"type":"viewing","projectId","taskId","lastSeq"}` — sent whenever the
  user opens/changes a project or task; on an actual project switch this
  triggers subscribe-time replay (see [Sync](#sync-the-event-log))
- `{"type":"rename","name"}` — display name change

Server → client:
- `{"type":"event","projectId","seq","eventType","payload","actor"}` — a
  durable event (task/comment/project mutation), whether pushed live or
  replayed on subscribe
- `{"type":"presence.updated","projectId","presence":[...]}` — full viewer
  roster for a project, sent on any join/leave/move/rename
- `{"type":"project.created","projectId","resourceId"}` /
  `{"type":"project.deleted","projectId","resourceId"}` — thin
  notifications kept outside the durable event log (see
  [Sync](#sync-the-event-log) for why); clients on the dashboard refetch
  the project list on either

## Technology choices

- **Go + chi + pgx (backend).** The realtime core of this app is "hold
  thousands of cheap concurrent WebSocket connections and fan events out
  to them" — goroutines and `gorilla/websocket` make that almost free, and
  chi stays a thin route table rather than a framework the domain has to
  live inside. `pgx` is used directly (no ORM): every mutation is a
  hand-written transaction, and the event log's correctness depends on
  exactly what happens inside those transactions, so hiding them behind an
  abstraction would obscure the most important code in the repo.
- **Postgres (storage).** The spec rules out managed realtime DBs, but the
  deeper reason: the sync model needs a mutation and its event-log append
  to commit **atomically**, and relational transactions give that for
  free. JSONB covers the schemaless parts (`metadata`, `customFields`),
  real foreign keys cover dependency/comment integrity, and one `BIGINT`
  counter per project row provides the gapless sequence.
- **WebSockets (transport), not SSE or polling.** The protocol is
  bidirectional — clients push `viewing`/`rename` upstream for presence
  while events flow down — which SSE can't do over one connection, and
  polling would either waste round trips or add latency. Fallbacks aren't
  needed for this deployment target.
- **Vanilla React + Vite + TypeScript (frontend).** There's no SEO or
  server-rendering requirement — it's an app behind a login in any real
  deployment — so Next.js would add moving parts without buying anything.
  The interesting frontend logic (event application, gap detection) is
  plain TypeScript, mirrored 1:1 from the Go payload structs in
  `types.ts`.
- **dnd-kit** for drag-and-drop: small, headless, and doesn't fight the
  optimistic-update flow.

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
Reconnecting after a dropped connection is handled explicitly: a
reconnected socket is a brand-new server-side connection with no viewing
state, so when the connection comes back up the client re-sends `viewing`
with its current `lastSeq` — the server replays exactly the offline gap
and the client rejoins the presence roster. Applying
a payload is idempotent (e.g. `task.created` dedupes by id before
appending), which is what makes it safe to reapply events that arrive both
via subscribe-time replay and an independent REST fetch.

Not implemented: a "client is way too far behind, just refetch a fresh
snapshot instead of replaying thousands of events" fallback — noted as a
TODO in `server/hub.go` (`replayTo`) and `client/src/ProjectDetail.tsx`
(`fillGap`), since ordinary reconnect gaps are small and `ListEventsSince`
caps a single query at 500 rows regardless.

## How I'd scale it over time

Roughly in the order the bottlenecks would appear:

1. **Multiple server instances.** The one piece of state the Go process
   holds is the WebSocket hub. Today a mutation is only broadcast by the
   instance that handled it, so the first scaling step is publishing
   committed events to a shared channel — Redis pub/sub, or Postgres
   `LISTEN/NOTIFY` to stay dependency-free — with every instance's hub
   subscribing and fanning out to its own connections. Nothing about the
   client protocol changes: the event log stays the source of truth, and
   pub/sub is allowed to be lossy because seq-gap detection already
   catches anything a client misses. Presence rosters would move to Redis
   with per-entry TTLs so a crashed instance's clients age out.
2. **Snapshot fallback for far-behind clients.** The TODO noted above: if
   `lastSeq` is thousands behind, tell the client to refetch the project
   snapshot instead of replaying the log. This also unlocks **event-log
   compaction** — events older than the oldest snapshot any client could
   need can be archived or dropped.
3. **Pagination.** Task lists and comment threads are currently fetched
   whole. At "2MB+ project" scale, cursor pagination on
   `(project_id, created_at, id)` for tasks and comments, with the board
   lazily loading per column. The event-log design is what makes this
   cheap to add: clients already apply deltas, so a paginated initial load
   doesn't change the sync path at all.
4. **Backpressure.** Per-connection buffered send queues in the hub; a
   client that can't drain its queue gets disconnected and resyncs via the
   normal reconnect path, instead of one slow reader wedging a broadcast.
   Rate limiting on mutations (per client ID) at the same layer.
5. **Database growth.** The existing indexes cover the hot paths
   (`tasks(project_id)`, `events(project_id, seq)`). Beyond that: read
   replicas for the list/snapshot endpoints, then partitioning `events` by
   project hash if the log outgrows one table. The per-project `last_seq`
   row lock intentionally serializes writers *within* a project, but
   projects are independent — write throughput scales with the number of
   projects, so a hot global lock never emerges.

## Tradeoffs

- **Per-project write serialization.** Claiming a seq takes a row lock on
  the project, so concurrent writes to one project queue up. Deliberate:
  it's what makes the log gapless, and a human team's write rate to a
  single project is nowhere near the ceiling.
- **Last-write-wins at field granularity, no OT/CRDT.** Two people editing
  the same task description at once → last save wins. Field-level patches
  keep the blast radius small (editing the title never clobbers a
  concurrent status change), and for task metadata that's the right
  cost/benefit; a CRDT would only pay off for collaborative long-text
  editing.
- **Dashboard refetches instead of applying deltas.** `ProjectList`
  refetches stats on any task event — an N+1 that's simple and fine at
  this scale, in contrast to the board, which is fully delta-driven.
  Comment threads similarly refetch the open thread on comment events;
  the payloads are tiny.
- **Hand-mirrored TS types, not a generated contract.** `types.ts` is kept
  in sync with the Go structs by discipline. At this size that's fine;
  with more surface area I'd generate the contract (OpenAPI or tygo) so it
  can't drift.
- **No auth.** Identity is a self-chosen display name per tab
  (`sessionStorage`) — the right scope for demonstrating sync, and the
  `X-Actor` header/actor column is where a real authenticated principal
  would slot in.
- **Events are retained forever.** No compaction until the snapshot
  fallback (above) exists to bound how far back a client can need.

## Running locally (native dev)

For hacking on the code with hot reload. Requires **Go 1.25+**, **Node.js
+ npm**, and **Docker** (for Postgres only — the
[one-command path](#quick-start-docker-one-command) needs just Docker).

### Backend

The backend reads and writes Postgres directly (via `pgx`), so the
database must be up first: `make db-up` from the repo root (or
`make db-reset` for a clean slate).

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
Serves the app at `http://localhost:5173`.
