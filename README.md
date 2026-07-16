# Task Manager

Collaborative task management system with near-real-time sync across clients.

- **Backend**: Go (`server/`) — chi router, in-memory store, WebSocket hub for live updates
- **Frontend**: Vanilla React + Vite + TypeScript (`client/`)

This is a work in progress, built up incrementally. See commit history for the
order features were added in. A full architecture write-up (sync strategy,
scaling plan, tradeoffs) will be added here as the system matures.

## Running locally

### Backend

```sh
cd server
go run .
```

Serves the API and WebSocket endpoint on `:8080` (override with `PORT`).

### Frontend

```sh
cd client
npm install
npm run dev
```

Vite dev server proxies `/api` and `/ws` — see `client/vite.config.ts`.
