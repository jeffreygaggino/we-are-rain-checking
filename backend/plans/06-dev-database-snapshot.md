# 06 — A snapshot of the ingested data

Ticket #30. `make dump` writes a compressed snapshot of the local dev database; `make restore` loads
one back. A fresh clone reaches a filled database in seconds rather than sitting through a cold
ingest. Motivation and the decision to keep the file out of git are settled in the ticket; this file
is the engineering, and the three traps that were measured rather than assumed.

## The numbers

Measured 2026-09-04 against this dev database — 101 Meetings, 495 Sessions, 48,218 Weather Samples,
1,661 Session Results, both schemas included:

| form | bytes |
|---|---|
| `pg_dump` plain SQL | 6,142,826 |
| plain SQL through `gzip -9` | 637,864 |
| `pg_dump -Fc` | 672,102 |

`-Fc` is 34 kB larger than the gzipped pipeline and is still the one to take. It is one file rather
than a pipe, it carries a table of contents `pg_restore` can act on, and — the reason that matters
here — `pg_restore --single-transaction` makes a failed load leave nothing behind. A truncated
`psql` stream does the opposite, and the way it goes wrong is expensive: ingest skips any Session
that already has Weather Samples, so a Session left half-filled by a broken restore is skipped
forever rather than repaired on the next run.

## The incantation in the ticket is silently wrong

The ticket suggests `-n f1 -t public.schema_migrations`. Measured, that produces **1,376 bytes** —
`public.schema_migrations` alone, with the entire `f1` schema absent — and it exits 0.

`pg_dump`'s documented behaviour: when `-t` is given, `-n` has no effect, because tables selected by
`-t` are dumped regardless of the schema switches. So the flag that was added to carry the version
*replaces* the data it was meant to accompany, and nothing says so. A 1.4 kB file named
`rainchecking.dump` restores an empty database and reports success.

The form that works is the ticket's own alternative: **`-n f1 -n public`**, both schemas named.

## `public` arrives with a `CREATE SCHEMA public`

Naming `public` explicitly makes `pg_dump` emit `CREATE SCHEMA public;`, which every target database
already satisfies — a new database gets `public` from its template. Left alone that is one error per
restore, ignored by `pg_restore` and printed at the end as `errors ignored on restore: 1`. A restore
that always prints an error it wants you to overlook is a restore whose real errors nobody reads.

So `restore` drops both schemas before loading:

```
SET client_min_messages = warning;
DROP SCHEMA IF EXISTS f1 CASCADE;
DROP SCHEMA IF EXISTS public CASCADE;
```

Measured: exit 0, zero errors, zero warnings, all four ingested tables and both seed tables at their
full counts.

### Why a reset rather than `pg_restore --clean --if-exists`

`--clean` emits a `DROP` for each object *the dump knows about*. An object created by a migration
newer than the snapshot is not in the dump, so it survives the load — and then `schema_migrations`
says the older version while the newer version's table is still standing. The next `migrate -up`
re-runs that migration against objects that already exist and fails halfway, dirty.

Dropping the two schemas outright means the database after a restore contains exactly the snapshot,
with no residue of what it held before. That is also what makes `restore` safe to simply re-run: it
resets first, so a failed load is repaired by running it again rather than by hand.

## The migration version, in both directions

Carrying `public.schema_migrations` is only half of it — the version has to be *acted* on. `restore`
ends by running the repo's own migrate binary against the dev database, and that one step covers
both ways the snapshot can disagree with the repo. Both were measured, on a scratch database holding
a real restored snapshot, with a throwaway `000003` added to and then removed from `db/migrations`:

| snapshot vs repo | what happens |
|---|---|
| older (snapshot at 2, repo at 3) | `migrate: up complete, version=3 dirty=false` — the pending migration is applied and the restored rows are intact |
| newer (snapshot at 3, repo at 2) | exit 1: `no migration found for version 3: read down for version 3 migrations: file does not exist` |

The second was the one in doubt. golang-migrate resolves the *current* version against the source
before looking for a next one, so a database ahead of the repo is a hard failure rather than a
no-change — which is what the acceptance criterion needs and what a `m.Up()` that only looked
forward would not have given.

`go run ./cmd/migrate` rather than the compose `migrate` service, deliberately: the migrations are
embedded in the binary, so running from source applies *this working tree's* migrations. The compose
service applies whatever migrations are baked into the last image that was built, which is exactly
the staleness this step exists to detect.

## Never anything but the local dev database

Three structural facts, in preference to a check that has to be remembered:

- **`pg_dump` and `pg_restore` run through `docker compose exec postgres`.** There is no host or
  port to give them; the server is the container in `docker-compose.local.yml` or the command fails.
  A remote host is not something the target refuses — it is something it cannot express. It also
  fixes the client version to the server's, which a host `pg_dump` does not.
- **Neither target loads `.env`.** No `DOTENV` prefix, so an `.env` pointing somewhere else has no
  say. The migrate step gets `DEV_ENV`, spelled out in the Makefile the way `TEST_ENV` already is,
  and its host is a literal `localhost`.
- **`DEV_DB_NAME` is a constant, distinct from `TEST_DB_NAME`.**

Two guards cover what remains, both on `restore`, because it is the destructive one:

1. **The target may not be the test database.** `DEV_DB_NAME` is a make variable, and make variables
   are overridable from the command line — the constant is only a constant until someone types
   `make restore DEV_DB_NAME=rainchecking_test`. One line makes it enforced.
2. **A dev database that already holds Meetings is not overwritten** without `FORCE=1`. The pickup
   path restores into an empty database; running it against a filled one throws away however much
   ingest it represents. The count is read with `SELECT count(*) FROM f1.meetings`, and a failure —
   no schema, no table, fresh volume — is read as empty, which is the answer that lets the pickup
   path through.

## A failed dump must not eat the previous snapshot

`pg_dump > $(SNAPSHOT)` truncates the file before `pg_dump` runs, so a dump that fails — container
down, disk full, interrupted — destroys the good snapshot it was refreshing and leaves nothing in
its place. `dump` writes `$(SNAPSHOT).partial`, removes it on failure, and moves it into place only
on success. The previous snapshot is either replaced or untouched.

### And a corrupt snapshot must not empty the database

`restore` drops both schemas before it loads, so the file has to be known good *first*. The obvious
check is `pg_restore -l`, and it is worthless here: the table of contents sits at the front of the
archive, so a snapshot truncated to its first 20 kB lists cleanly and **exits 0**. That was measured
by truncating a real snapshot — the check passed, the schemas were dropped, the load then died on
EOF, and `--single-transaction` rolled back the load but not the drops. The dev database was left
empty by a target whose own message said it was untouched.

`pg_restore -f /dev/null` reads and decompresses the whole archive instead, exits 1 on the same file,
and costs 0.15 s on a good one. Nothing is dropped until it passes.

What remains uncovered is a load that fails server-side after a verified archive has already passed
— disk full, say. That leaves an empty database, loudly, with the snapshot still on disk and
`make restore` re-runnable. It is not worth a temporary database and a swap to close.

## Where the file lives

`backend/snapshots/rainchecking.dump`, with `backend/snapshots/` gitignored. The ticket settles why
it is not committed: 0.6 MB of binary that changes on every refresh, in a repo whose diffs are meant
to be read, reproducible from a free upstream by definition.

`SNAPSHOT` is a plain make variable, so a second file needs no feature — `make restore
SNAPSHOT=snapshots/2026-09-04.dump` already works. That is make's behaviour rather than a knob built
here.

Publishing a snapshot for a second machine — a new laptop, a rebuilt host, seeding CI — is a GitHub
Release asset (`gh release upload`), not a commit. Out of scope for this ticket; recorded so it is
not re-argued.

## What `make ingest` costs after a restore

Nothing in the service depends on a snapshot existing: ingest remains how data arrives, and it is
resumable and idempotent. After a restore it re-fetches only what the snapshot could not carry —
seasons that have ended are skipped without a request, the season in progress is always re-fetched
because Sessions are still being added to it, and Weather Samples and Session Results are skipped per
Session and per Race.

Measured on a freshly restored database, 2026-09-04:

```
ingest: complete — 27 meetings and 131 sessions across [2026], skipped [2023 2024 2025];
0 weather samples from 15 sessions, skipped 425; 0 results from 0 races, skipped 82
```

**40 seconds and 17 requests**, against the ~530 and ~22 minutes a cold ingest costs: two for the
season in progress, fifteen for the Sessions whose Weather Samples the snapshot could not carry
because they have not run yet, none at all for the three settled seasons or for the 82 Races already
classified.

The row counts before and after that run are identical — 101 Meetings, 495 Sessions, 48,218 Weather
Samples, 1,661 Session Results. Nothing the snapshot carried was fetched again, and nothing it
carried was disturbed by fetching what it missed.

## What was run, 2026-09-04

The round trip has no automated test (below), so this is the record of it. Each row is a command that
was run against this machine's stack, not a claim about what should happen.

| run | result |
|---|---|
| `make dump` | 660 kB in ~1 s |
| the pickup path — an empty migrated database, then `make restore` with no `FORCE` | 101 Meetings, 495 Sessions, 48,218 Weather Samples, 1,661 Session Results, version 2, in **1.4 s** |
| `GET /api/v1/health` against the restored database, no ingest run | 200 |
| snapshot older than the repo | `version=3`, restored rows intact |
| snapshot ahead of the repo | exit 1, naming version 3 |
| `make ingest` on the restored database | complete in 40 s on 17 requests, three settled seasons skipped without one, row counts unchanged |
| `make dump` / `make restore` aimed at the test database | both refused |
| `make restore` with no snapshot at the path | refused |
| `make restore` from a snapshot truncated to 20 kB | refused, database intact |
| `make restore` over a filled database without `FORCE=1` | refused, naming the 101 Meetings it would have replaced |
| `make gate` | green |

The health route is the whole of the "serves against it" check because it is the whole of the API
today: `/races/next` and `/correlation` are the finished design rather than registered routes. It is
the right check regardless — it is the one route that reads the database, so a 200 is the restored
database answering.

The pickup path was run against a scratch database rather than the dev one, so a failure could not
cost the four seasons the snapshot exists to protect. Same target, same recipe, `DEV_DB_NAME`
pointed elsewhere.

## Why there is no Go test

There is no seam here that a Go test could hold. The behaviour is `pg_dump` and `pg_restore` driven
through docker, and the one piece of it written in this module — the migrate step — is already
covered in `tests/migrations_test.go`.

More decisively, `make test` must never touch the dev database; that separation is why
`TEST_DB_NAME` exists. A test that exercised `make restore` would have to reset the database the
target is pointed at, which is the one thing the test suite is built not to do. The verification is
the round trip run by hand, recorded above and in the ticket.
