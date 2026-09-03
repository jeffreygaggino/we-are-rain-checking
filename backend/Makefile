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

# Prefix for the targets that need .env. Nothing in the module reads that file — config.LoadConfig
# reads the environment — so the shell loads it: `set -a` exports every assignment the file makes,
# and turns off again after.
#
# The [ -f ] guard does not make .env optional. It holds the credentials, and a run without it still
# dies, on `config: DB_HOST is required`. The guard only decides which error you get: the loader
# naming the variable it wanted, rather than `.` complaining about a file that is not there. Its own
# failure is harmless — make judges the line by its last command, which is the go run.
#
# `DB_HOST is required` therefore has two causes. No .env is this one. An .env that was never
# sourced is the other, and is the confusing one — see the ingest target below.
DOTENV = set -a; [ -f .env ] && . ./.env; set +a;

.PHONY: tidy build docs run test local local-down migrate-up migrate-down ingest
tidy:  ; @go mod tidy
docs:  ; @swag init
build: ; @go build ./...

# #12 made .env load bearing here rather than a convenience: the serving mode now defaults to
# release, so a `make run` that could not see GIN_MODE=debug would serve a development machine with
# no route table at all.
run:   ; @$(DOTENV) go run .

# The whole stack from one command: Postgres, the migrations applied, then the API. --wait returns
# once the API's health check passes, so a green `make local` means the service answers.
local: ; @docker compose -f $(COMPOSE_LOCAL) up -d --build --wait
local-down: ; @docker compose -f $(COMPOSE_LOCAL) down

migrate-up:   ; @go run ./cmd/migrate -direction up
migrate-down: ; @go run ./cmd/migrate -direction down

# Fills the tables from OpenF1. Paced to the upstream's 30 req/min, so a first run takes about twenty
# seconds and a re-run about six — seasons already stored are skipped without a request. Safe to
# re-run and safe to interrupt.
#
# Without DOTENV, `make ingest` on a fresh clone dies on `DB_HOST is required` despite .env sitting
# right there. Same message a missing .env gives (see DOTENV above), and the file being present is
# exactly what makes it hard to read — you go looking at the file rather than at what reads it.
ingest: ; @$(DOTENV) go run ./cmd/ingest

# test-db brings up the local Postgres and creates the test database if it is missing. Tests apply
# the migrations themselves, so this only has to hand them an empty database that exists.
#
# Postgres alone, not `local`: the tests drive the router in-process through httptest, so building
# and running the API container would add a minute to `make test` and prove nothing extra.
.PHONY: test-db
test-db:
	@docker compose -f $(COMPOSE_LOCAL) up -d --wait postgres
	@docker compose -f $(COMPOSE_LOCAL) exec -T postgres \
		psql -U $(TEST_DB_USER) -d $(TEST_DB_USER) -tc \
		"SELECT 1 FROM pg_database WHERE datname = '$(TEST_DB_NAME)'" | grep -q 1 || \
	docker compose -f $(COMPOSE_LOCAL) exec -T postgres \
		createdb -U $(TEST_DB_USER) $(TEST_DB_NAME)

test: test-db
	@$(TEST_ENV) go test ./... -cover

# ── Gate ──────────────────────────────────────────────────────────────────────
# One command answers "is this done". Scoped to this module, never repo-wide.

.PHONY: gate fmt-check vet docs-check
fmt-check:
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || (echo "gofmt: files above need formatting" && exit 1)

vet: ; @go vet ./...

# The gate regenerates the spec rather than trusting the committed one, so a handler annotation
# edited without regenerating shows up as a diff the human reads before committing.
docs-check:
	@command -v swag >/dev/null || (echo "swag: not installed — go install github.com/swaggo/swag/cmd/swag@latest" && exit 1)
	@swag init --quiet

gate: fmt-check vet docs-check build test
	@echo "gate: green"
