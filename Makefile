COMPOSE = docker compose
DB_CONTAINER = taskman_db
DB_URL = postgres://app:app@localhost:5432/taskman?sslmode=disable
MIGRATE_IMG = migrate/migrate:v4.17.1
MIGRATIONS_DIR = $(CURDIR)/server/migrations

.PHONY: db-up db-migrate db-seed db-reset db-psql

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
	$(COMPOSE) down -v
	$(MAKE) db-up
	$(MAKE) db-migrate
	$(MAKE) db-seed

db-psql:
	$(COMPOSE) exec db psql -U app -d taskman
