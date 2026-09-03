IMAGE_NAME = jeffreygaggino/we-are-rain-checking-backend
PLATFORM   = linux/amd64

COMPOSE_LOCAL = docker-compose.local.yml

# Local dev database. Spelled out here rather than read from .env, which is what keeps `dump` and
# `restore` pointed at this machine's stack whatever an .env happens to say. The host and port are
# the ones compose publishes, and are used only by the migrate step in `restore` — pg_dump and
# pg_restore reach the server through `compose exec` and are given no host at all.
DEV_DB_HOST     = localhost
DEV_DB_PORT     = 55432
DEV_DB_NAME     = rainchecking
DEV_DB_USER     = rainchecking
DEV_DB_PASSWORD = rainchecking
DEV_DB_SCHEMA   = f1
DEV_DB_SSLMODE  = disable

DEV_ENV = DB_HOST=$(DEV_DB_HOST) DB_PORT=$(DEV_DB_PORT) DB_NAME=$(DEV_DB_NAME) \
          DB_USER=$(DEV_DB_USER) DB_PASSWORD=$(DEV_DB_PASSWORD) \
          DB_SCHEMA=$(DEV_DB_SCHEMA) DB_SSLMODE=$(DEV_DB_SSLMODE)

# Where `make dump` writes and `make restore` reads. Gitignored; why it is not committed, and where a
# shareable one would go instead, are in plans/06-dev-database-snapshot.md. Being a make variable, a
# second snapshot needs no feature: `make restore SNAPSHOT=snapshots/2026-09-04.dump`.
SNAPSHOT_DIR = snapshots
SNAPSHOT     = $(SNAPSHOT_DIR)/rainchecking.dump

# Both snapshot targets refuse the test database. DEV_DB_NAME is a constant only until someone
# overrides it on the command line, which make allows by design; this is what makes it enforced.
# On `dump` as well as `restore`, because a snapshot taken of the empty test database is a file that
# restores an empty dev database and reports success.
REFUSE_TEST_DB = test "$(DEV_DB_NAME)" != "$(TEST_DB_NAME)" || \
	(echo "snapshot: refusing to act on $(TEST_DB_NAME) — these targets are for the dev database" && exit 1)

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

# Fills the tables from OpenF1. Paced to the upstream's 30 req/min, so a cold run is ~530 requests and
# takes about 22 minutes — a re-run is seconds, because a season already stored, a Session that already
# has Weather Samples and a Race that already has results are each skipped without a request. Safe to
# re-run and safe to interrupt. `make restore` is the way to skip the cold run entirely.
#
# Without DOTENV, `make ingest` on a fresh clone dies on `DB_HOST is required` despite .env sitting
# right there. Same message a missing .env gives (see DOTENV above), and the file being present is
# exactly what makes it hard to read — you go looking at the file rather than at what reads it.
ingest: ; @$(DOTENV) go run ./cmd/ingest

# ── Snapshots ─────────────────────────────────────────────────────────────────
# The 22 minutes above, paid once. `make dump` writes the ingested data to a file; `make restore`
# loads it, so a fresh clone or a thrown-away volume picks up from there. The snapshot is a
# convenience and never a source of truth: ingest stays how data arrives, and `make ingest` after a
# restore fills whatever the snapshot missed.
#
# Both schemas, not just f1. golang-migrate keeps schema_migrations in public, and a snapshot that
# cannot say which migration it was taken at is worse than no snapshot. Note that the obvious
# `-n f1 -t public.schema_migrations` does NOT do this: pg_dump ignores -n when -t is given, so it
# writes that one table and nothing else, and exits 0. See plans/06-dev-database-snapshot.md.
#
# Both targets reach the database through `compose exec`, which is what makes them local-only — there
# is no host to hand pg_dump, and the client version is the server's by construction. Neither loads
# .env, so an .env pointing elsewhere has no say.
.PHONY: dump restore

# Writes .partial first: `pg_dump > $(SNAPSHOT)` truncates the file before pg_dump runs, so a failed
# dump would destroy the good snapshot it was refreshing and leave nothing in its place.
dump:
	@$(REFUSE_TEST_DB)
	@mkdir -p $(SNAPSHOT_DIR)
	@docker compose -f $(COMPOSE_LOCAL) exec -T postgres \
		pg_dump -U $(DEV_DB_USER) -d $(DEV_DB_NAME) -n f1 -n public -Fc > $(SNAPSHOT).partial \
		|| (rm -f $(SNAPSHOT).partial; exit 1)
	@mv $(SNAPSHOT).partial $(SNAPSHOT)
	@echo "dump: wrote $(SNAPSHOT) ($$(du -h $(SNAPSHOT) | cut -f1))"

# Replaces the dev database with the snapshot, then brings the schema up to this working tree's
# migrations. Dropping both schemas first means what lands is exactly the snapshot, with no residue of
# what was there before — including a table from a migration newer than the dump, which pg_restore
# --clean would leave standing for the migrate step to trip over. It also makes a failed restore
# repairable by re-running this target.
#
# Since that drop comes first, the archive has to be known good before it: decompressing to /dev/null
# reads all of the archive, where `pg_restore -l` reads only the table of contents at its front and so
# passes a truncated file. Measured, in plans/06-dev-database-snapshot.md.
#
# The migrate step is where a version mismatch is answered: a snapshot behind the repo has the pending
# migrations applied, and one ahead of it fails naming the version no migration file describes, rather
# than quietly serving a schema nothing here can account for.
restore:
	@test -f $(SNAPSHOT) || \
		(echo "restore: no snapshot at $(SNAPSHOT) — run 'make dump' against a filled database first" && exit 1)
	@$(REFUSE_TEST_DB)
	@docker compose -f $(COMPOSE_LOCAL) exec -T postgres pg_restore -f /dev/null < $(SNAPSHOT) || \
		(echo "restore: $(SNAPSHOT) is not a readable archive — refusing, the database is untouched" && exit 1)
	@rows=$$(docker compose -f $(COMPOSE_LOCAL) exec -T postgres \
		psql -U $(DEV_DB_USER) -d $(DEV_DB_NAME) -tAc 'SELECT count(*) FROM f1.meetings' 2>/dev/null || echo 0); \
	test "$$rows" = "0" || test -n "$(FORCE)" || \
		(echo "restore: $(DEV_DB_NAME) already holds $$rows meetings — 'make restore FORCE=1' to replace them" && exit 1)
	@docker compose -f $(COMPOSE_LOCAL) exec -T postgres \
		psql -U $(DEV_DB_USER) -d $(DEV_DB_NAME) -q -v ON_ERROR_STOP=1 \
		-c 'SET client_min_messages = warning; DROP SCHEMA IF EXISTS f1 CASCADE; DROP SCHEMA IF EXISTS public CASCADE;'
	@docker compose -f $(COMPOSE_LOCAL) exec -T postgres \
		pg_restore -U $(DEV_DB_USER) -d $(DEV_DB_NAME) --single-transaction < $(SNAPSHOT)
	@$(DEV_ENV) go run ./cmd/migrate -direction up
	@echo "restore: $(DEV_DB_NAME) filled from $(SNAPSHOT) — 'make ingest' fills whatever it missed"

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
