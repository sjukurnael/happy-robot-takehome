DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS task_dependencies;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS projects;
-- pgcrypto is intentionally left installed: DROP EXTENSION could affect
-- other objects on a shared database and buys nothing on a fresh one.
