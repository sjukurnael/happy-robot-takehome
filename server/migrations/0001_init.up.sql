CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- for gen_random_uuid()

CREATE TABLE projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    description TEXT NOT NULL DEFAULT '',
    metadata    JSONB NOT NULL DEFAULT '{}',
    last_seq    BIGINT NOT NULL DEFAULT 0,   -- per-project event sequence counter
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- updated_at is maintained by application code in later phases; no trigger here.
CREATE TABLE tasks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title         TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 500),
    -- 'blocked' is intentionally not a stored status: whether a task is
    -- blocked is derived from its dependencies' statuses at read time, not
    -- persisted, so it can never go stale relative to the tasks it depends on.
    status        TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','in_progress','done')),
    assigned_to   TEXT[] NOT NULL DEFAULT '{}',
    configuration JSONB NOT NULL DEFAULT '{}',  -- {priority, description, tags[], customFields}
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_tasks_project ON tasks(project_id);
CREATE INDEX idx_tasks_project_status ON tasks(project_id, status);

-- Cross-project dependency prevention (a task and its dependency must belong
-- to the same project) is enforced in application code, not as a DB
-- constraint here, since it requires comparing project_id across two rows.
CREATE TABLE task_dependencies (
    task_id            UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, depends_on_task_id),
    CHECK (task_id <> depends_on_task_id)      -- no self-dependency
);
CREATE INDEX idx_deps_reverse ON task_dependencies(depends_on_task_id);

CREATE TABLE comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    content    TEXT NOT NULL CHECK (length(content) BETWEEN 1 AND 10000),
    author     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_comments_task ON comments(task_id);

CREATE TABLE events (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    seq        BIGINT NOT NULL,
    type       TEXT NOT NULL,
    payload    JSONB NOT NULL,
    actor      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, seq)              -- also the replay index
);
