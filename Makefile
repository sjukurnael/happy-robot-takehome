COMPOSE = docker compose
DB_CONTAINER = taskman_db
DB_URL = postgres://app:app@localhost:5432/taskman?sslmode=disable
MIGRATE_IMG = migrate/migrate:v4.17.1
MIGRATIONS_DIR = $(CURDIR)/server/migrations

.PHONY: up down db-up db-migrate db-seed db-reset db-psql

# Full stack in Docker: Postgres + migrations + Go server + built frontend.
up:
	$(COMPOSE) --profile app up -d --build --wait
	@echo "App running at http://localhost:3000 (API on :8080). Load demo data with: make db-seed"

down:
	$(COMPOSE) --profile app down

db-up:
	$(COMPOSE) up -d --wait db

db-migrate:
	docker run --rm --network container:$(DB_CONTAINER) \
		-v $(MIGRATIONS_DIR):/migrations \
		$(MIGRATE_IMG) \
		-path=/migrations -database "$(DB_URL)&x-migrations-table=ev_migrations" up

db-seed:
	$(COMPOSE) exec -T db psql -U app -d taskman -v ON_ERROR_STOP=1 < server/seed/seed.sql

db-reset:
	$(COMPOSE) --profile app down -v
	$(MAKE) db-up
	$(MAKE) db-migrate
	$(MAKE) db-seed

db-psql:
	$(COMPOSE) exec db psql -U app -d taskman
