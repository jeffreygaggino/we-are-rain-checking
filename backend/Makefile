IMAGE_NAME = jeffreygaggino/we-are-rain-checking-backend
PLATFORM   = linux/amd64

COMPOSE_LOCAL = docker-compose.local.yml

# Test database. Separate from the dev database so a `make test` never truncates work in progress.
TEST_DB_HOST     = localhost
TEST_DB_PORT     = 55432
TEST_DB_NAME     = rainchecking_test
TEST_DB_USER     = rainchecking
TEST_DB_PASSWORD = rainchecking
TEST_DB_SCHEMA   = f1
# The local compose Postgres does not serve TLS. Every other environment keeps the default of on.
TEST_DB_SSLMODE  = disable

TEST_ENV = DB_HOST=$(TEST_DB_HOST) DB_PORT=$(TEST_DB_PORT) DB_NAME=$(TEST_DB_NAME) \
           DB_USER=$(TEST_DB_USER) DB_PASSWORD=$(TEST_DB_PASSWORD) \
           DB_SCHEMA=$(TEST_DB_SCHEMA) DB_SSLMODE=$(TEST_DB_SSLMODE)

# ── Local development ─────────────────────────────────────────────────────────

.PHONY: tidy build test local local-down migrate-up migrate-down
tidy:  ; @go mod tidy
build: ; @go build ./...
local: ; @docker compose -f $(COMPOSE_LOCAL) up -d --wait
local-down: ; @docker compose -f $(COMPOSE_LOCAL) down

migrate-up:   ; @go run ./cmd/migrate -direction up
migrate-down: ; @go run ./cmd/migrate -direction down

# test-db brings up the local Postgres and creates the test database if it is missing. Tests apply
# the migrations themselves, so this only has to hand them an empty database that exists.
.PHONY: test-db
test-db: local
	@docker compose -f $(COMPOSE_LOCAL) exec -T postgres \
		psql -U $(TEST_DB_USER) -d $(TEST_DB_USER) -tc \
		"SELECT 1 FROM pg_database WHERE datname = '$(TEST_DB_NAME)'" | grep -q 1 || \
	docker compose -f $(COMPOSE_LOCAL) exec -T postgres \
		createdb -U $(TEST_DB_USER) $(TEST_DB_NAME)

test: test-db
	@$(TEST_ENV) go test ./... -cover

# ── Gate ──────────────────────────────────────────────────────────────────────
# One command answers "is this done". Scoped to this module, never repo-wide.

.PHONY: gate fmt-check vet
fmt-check:
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || (echo "gofmt: files above need formatting" && exit 1)

vet: ; @go vet ./...

gate: fmt-check vet build test
	@echo "gate: green"
