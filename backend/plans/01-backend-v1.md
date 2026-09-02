# 01 — Backend v1

Implements #1 via #2–#11. This file holds the rationale; the source holds the code and only the
non-obvious comments. `docs/agents/backend.md` is the authority on code; `02-scope-reset.md` supersedes
this file's framing and ticket line, not its engineering.

## Status

Stopped deliberately after the first ingest stage. Everything below the line is designed, not built.

| ticket | state |
|---|---|
| #4 derivation functions | **done** — red-first, 100% covered |
| #5 seed Circuits and Drivers | **done** — 26 Circuits, 29 Drivers, literal ids, stability test seen to fail |
| #2 service skeleton | **done** — `main.go`, `app/`, `routes/`, `handlers/`, generated `docs/` served at `/docs` |
| #3 Postgres and migrations | **done** — the health route pings the database, the HTTP seam runs against real Postgres, and `make local` brings the whole stack up |
| #6 ingest Meetings and Sessions | **done** — `cmd/ingest`, the OpenF1 client, seam 2 against real Postgres with a stubbed upstream |
| #7, #8, #9, #12, #13 | not started — see `02-scope-reset.md` |
| #10 correlation | not started, **rescoped** — Driver-Race unit, three separate axes, no pagination (`02-scope-reset.md`) |
| #11 forecast cache | **cut** — no measurement motivated it (`02-scope-reset.md`) |

Consequences for what is written below: the weather/results half of ingest and the endpoint table are
the intended design and nothing more — only `/health` exists. **The cache section below is dead**; it
is left in place because the reasoning that produced it is what later cut it.

## Running it

One command each, from `backend/`:

| command | what it does |
|---|---|
| `make local` | the whole stack — Postgres, migrations applied, then the API. Returns once `/api/v1/health` answers, so a green run means the service works, not that two containers exist |
| `make run` | the API alone against whatever `.env` points at |
| `make ingest` | fills Meetings and Sessions from OpenF1. ~20s cold, ~6s re-run — stored seasons are skipped without a request. Safe to re-run, safe to interrupt |
| `make test` | starts Postgres, creates the test database, runs every test |
| `make gate` | format → vet → docs → build → test |
| `make local-down` | stops the stack |

Migrations are a one-shot compose service, not a startup step: the API image never migrates on boot,
so "the schema changed" is always something with its own exit code.

## Fill-ins

| setting | value |
|---|---|
| module path | `github.com/jeffreygaggino/we-are-rain-checking/backend` |
| DB schema | `f1` |
| service directory | `backend/` — its own top-level directory (#2) |
| image name | `ghcr.io/jeffreygaggino/we-are-rain-checking-backend` |

## Conventions

The service was scaffolded from a generic `STRUCTURE-go-gin-backend.md` written for user-authored data
behind an auth service. Seven of its rules did not survive contact with this domain, and the file was
removed on 2026-09-03 once the skeleton stood (`0fff05e`). These are the rules that replaced them —
stated positively, because a convention defined as a deviation from a deleted file is unreadable.
Three are ADRs; the rest live here.

1. **No `permissions/` package** (ADR-0001). Handlers begin at step 2 of the seven-step body. The
   seven steps are otherwise intact and in order.
2. **No UUID PKs, no `created_by`, no `deleted_at` on ingested tables** (ADR-0002). Upstream keys are
   the PKs. Seeded tables (`circuits`, `drivers`) do keep UUID PKs — they have no upstream key to
   inherit — and those UUIDs are **literal constants in the migration**, never `gen_random_uuid()`
   (ADR-0003).
3. **No CORS.** `gin-contrib/cors` is not imported. There is no browser client to allow yet (#2).
4. **New leaf package `derive/`.** STRUCTURE's layout has no home for pure decision functions, but
   #4 names four of them as their own test surface. `models` is for shapes, `services` take a
   `ctx` and orchestrate; neither fits a `float64 -> bool`. `derive` imports `models` only.
5. **The composition root is `app/`, not `main.go`.** STRUCTURE wires the graph in `main.go`, but a
   `package main` cannot be imported, so the HTTP harness would have to re-wire it — two copies that
   drift the first time an endpoint is added to one. `app.New(conn)` returns the router; `main.go`
   keeps everything a test has no business doing (load config, connect, set the spec's host, serve).
   It also takes the pool as an argument rather than reading `db.DB`, which is what lets a test hand
   it a database that is not there.
6. **`HealthService` calls `db.Ping` directly, with no `repository/` in between.** STRUCTURE's
   dependency rule is `services -> repository -> db`, and this skips a layer. A health repository
   would read no rows, and the thing it would wrap already exists: `db.Ping` bounds its own wait,
   which is the property the route needs — an unreachable database has to answer the health check,
   not hang it. The first repository arrives with #6, where there are rows to read; the rule holds
   from there.
7. **The docs route is registered in `routes.SetupRouter`,** where STRUCTURE puts it in `main.go`.
   "One `SetupRouter`, all route registration" is the stronger of the two rules, and it means the
   spec is served by the same object as the routes it documents — so a test can hold one against the
   other, which is how "the docs reflect the health route" is asserted rather than assumed.

One addition: **`tests/`**, holding the two agreed seams' harness (#3: "reusable by later tickets
without modification"). Keeping it out of `handlers/` stops the harness being mistaken for a unit
test of the handler package. Seam 1 lives in `tests/http.go`: later tickets add a route to `app.New`
and get the harness unchanged.

## Test seams — agreed, and nothing else

```
seam 1  HTTP boundary   httptest -> SetupRouter -> real PG, stub upstreams
seam 2  ingest entry    Ingest(ctx, deps) -> real PG, stub upstream
  +     derivation      WetFraction / IsWetSession / SampleVerdict / WindBand — no DB
```

Real Postgres, not a mock: idempotency is `ON CONFLICT` behaviour, and a mock asserting on query
strings would prove nothing about whether re-ingest actually deduplicates.

## Thresholds — each defined once, in `derive/`

| constant | value | reasoning |
|---|---|---|
| `WetSessionThreshold` | `0.25` | CONTEXT.md: a Wet Session *exceeds* the threshold, so the comparison is `>`, and exactly-at-threshold is dry. A quarter of a session's samples means rain was a condition of the Race rather than an incident during it. Presence of any rain is explicitly not sufficient. |
| `MinimumSampleSize` | `30` | Below ~30 observations the standard error on a rate swamps any effect this corpus could show. The measured wet corpus is 9 Races, so the honest steady state of the rainfall axis is `insufficient_sample` — that is the intended output, not a gap. |
| wind band edges | `2 / 4 / 6` m/s | OpenF1 `wind_speed` and Open-Meteo `wind_speed_10m` are both requested in m/s, so no conversion exists to get wrong. Lower edge inclusive, upper exclusive. |

Changing any of these is one edit and one failing test. That is the whole reason they are functions
taking values rather than SQL predicates.

## Schema

`f1` schema. Seeded tables first, ingested tables reference them.

| table | key | source | index, and why it is there |
|---|---|---|---|
| `circuits` | uuid, literal | seeded | `circuit_key` unique — ingest joins on it |
| `drivers` | uuid, literal | seeded | `full_name` unique — the only resolution key (ADR-0003) |
| `meetings` | `meeting_key` | ingested | — |
| `sessions` | `session_key` | ingested | `(date_start)` for next-Race; `(year, session_name)` for the corpus scan |
| `weather_samples` | `(session_key, observed_at)` | ingested | the PK **is** the index: session-leading composite btree serves both "samples for a Session" and "samples in a time range within a Session", which is the only way this table is read |
| `session_results` | `(session_key, driver_id)` | ingested | PK serves by-Session; a separate `(driver_id)` index serves by-Driver, which the PK cannot because `driver_id` is the trailing column |

**`gap_to_leader` is not stored.** Upstream returns a union — `0`, `11.987`, `"+1 LAP"`, `null` — in
one field. No endpoint consumes it, so it is dropped rather than given a `text` column and a lenient
decoder that nothing reads. Go ignores it on decode automatically.

**`position` is nullable.** A Retirement has no position; a sentinel like `0` or `99` would sort into
the results.

## Ingest

A `cmd/ingest` binary, never a route — that is what makes the missing auth layer safe rather than
negligent (ADR-0001). The two decisions stand or fall together.

- **Pacing.** 30 req/min documented, so a 2.1 s minimum interval between requests (~28/min) with a
  3 req/s ceiling. One limiter shared by every OpenF1 call.
- **404 is ambiguous upstream.** OpenF1 returns `404 {"detail":"No results found."}` for an empty
  result *and* 404 for a bad query. Verified by probe: `drivers?year=2024` 404s with exactly that
  body. The client inspects the body, mapping that detail to an empty slice and anything else to an
  error. Status alone cannot tell them apart.
- **Resumption is derived, not checkpointed.** A completed Session's weather and results never
  change (upstream is live only ±30 min around a Session), so ingest skips any Session that already
  has rows and only considers Sessions whose `date_end` is in the past. Interrupt it and re-run: it
  picks up at the first Session with nothing stored. A checkpoint table would be a second source of
  truth for a fact the data already carries.
- **Driver resolution** goes `(session, racing number) -> upstream full_name -> seeded driver_id`,
  per Session. An unseeded name aborts the whole ingest before inserting anything. See ADR-0003 —
  this is the decision most likely to be "corrected" by someone who has not read it.

### Stage 1 — Meetings and Sessions (#6)

**Seasons are a range, not a config knob.** `services.FirstSeason = 2023` is the earliest year OpenF1
carries; the last is the current year from the clock. A knob with one value is the abstraction
`docs/agents/backend.md` forbids, and the floor is a property of the upstream rather than of a deployment.
Probed: `meetings?year=2022` 404s, `year=2026` returns rows.

**Resumption at this stage is per season, and derived.** A season strictly before the current year
whose Meetings *and* Sessions are both already stored is skipped without a request. The current
season is always re-fetched, because Sessions are still being added to it. Nothing is checkpointed:
the rows already answer "did this year land".

That rule is only sound because **a season's writes are one transaction**. Both upstream calls happen
before the transaction opens, so a year is stored whole or not at all — a half-written 2025 would
otherwise be skipped forever on the next run.

**The 404 discrimination is defensive today.** Probed 2026-09-02: `year=2022` (no results), `year=abc`
(malformed), `?nonsense=1` (unknown filter) and a nonexistent path all return
`404 {"detail":"No results found."}` — the upstream currently collapses every one of them into the
same body. The client still matches on the detail string and errors on anything else, because the
alternative is treating *every* 404 as an empty season: a proxy's own 404, or an upstream that starts
distinguishing the cases, would then read as "this season has no races" and ingest would report
success over an empty table.

**The body check alone is not enough, and review caught that.** A wrong `OPENF1_BASE_URL` answers
`404 {"detail":"No results found."}` for every path — verified 2026-09-02 against
`/v1/meetingz?year=2023` — so a misconfigured run reads as "none of these seasons exist", commits
four empty transactions and exits 0 over an empty table. The exact outcome the body check was
written to prevent, arriving through the check rather than around it. So a run that fetched at least
one season and stored no Meeting at all fails with `ErrUpstreamEmpty`. It is scoped to *fetched*
seasons: a re-run that skips everything already stored has nothing to be empty about.

**An interrupt exits 0.** `context.Canceled` reaches `cmd/ingest` as an error, but a scheduler
draining the binary with SIGTERM leaves precisely the state this design intends — whole seasons,
resumable — and a non-zero exit there pages someone about a working system. It logs what it got
through and returns cleanly. Every other error still exits 1.

**Pacing is a slot reservation, not a sleep.** `client.pacer` hands each call the next free instant
under a mutex and waits for it, so the interval holds under concurrency and a cancelled context
returns rather than finishing the sleep. `INGEST_MIN_INTERVAL` is the one knob (2100ms), and tests
pass `0`.

**`is_cancelled` is stored on Sessions only.** Upstream carries it on both, but a cancelled Meeting is
not a thing any endpoint asks about, and `f1.meetings` has no column for it — so it is dropped at the
client boundary rather than given one.

Measured after the first real run: **15 cancelled Sessions**, three of them Races — 2023 Emilia
Romagna (flooding) and two 2026 rounds. They are stored, because dropping a Session upstream still
lists would make re-ingest look like it lost rows. **#9 and #10 must filter them**: a cancelled Race
is not the next Race, and it contributes no result to the corpus while still carrying Weather
Samples. This is the first thing to get wrong in either ticket.

**No interface at the ingest seam.** The test substitutes the upstream with an `httptest.Server`
speaking OpenF1's wire shapes, which exercises the client's decoding, its 404 branch and its pacing
in the same pass. An interface over the client would have moved all three out of the seam and left
one implementation plus a stub — the guess `docs/agents/backend.md` names.

## Endpoints — three, per the spec

| route | reads | degrades |
|---|---|---|
| `GET /api/v1/health` | DB ping + cache counters | 503 when the DB is unreachable |
| `GET /api/v1/races/next` | DB + **live Open-Meteo** | 503 naming `open-meteo`, carrying its status and reason |
| `GET /api/v1/correlation` | DB only | unaffected by either upstream |

**Why the correlation endpoint has no cache.** It touches no live upstream at request time, so the
30 req/min ceiling binds ingest — a batch job that can pace itself — and not serving. Caching it
would be caching Postgres, measured at nothing. The cache that has to exist is on the forecast path,
and its hit and miss counters are on `/health` so its value is a number rather than an assumption.

**Why the band list is paginated.** Story 14 asks for list responses with total counts, and the spec
fixes the surface at three endpoints — so there is no separate list route to carry it. The
correlation report's `bands` therefore go through `NewPaginationResponse`. Paging two bands is
pointless in practice; the point is that one client-side handler works on every list this API returns.

**The correlation endpoint performs no inference.** Counts and rates by band with the sample size
attached. No p-value, no confidence interval, nothing phrased as a prediction. Given nine wet Races
across a corpus that cannot grow, its ordinary output is that no Signal is present.

## Rejected

- **UUID PK + unique natural key on ingested tables.** Resumable ingest needs
  `ON CONFLICT (natural key)`, so the unique constraint exists regardless; the UUID would then be
  information-free and joined on by nothing. See ADR-0002.
- **A `/races` list endpoint** to satisfy story 14 honestly. The spec fixes three endpoints, and
  three means three.
- **Geocoding circuit coordinates.** 26 rows, seeded. Deterministic, testable, one fewer network
  dependency — and "Monza" geocodes to a town as readily as a circuit.
- **Storing a rain magnitude.** `rainfall` is `{0,1}` across all four seasons. Any column implying
  intensity would be a lie at the source.
- **Serving TLS from the API itself.** The scaffold branched on `TLS_CERT`/`TLS_KEY`, but neither #2
  nor #3 asks for it — every TLS criterion in #3 is about the *database* connection. A second serving
  path that nothing configures and no test covers is a branch that rots. `router.Run` only.
  Terminating TLS is the deployment's job, and `02-scope-reset.md` settles it: Tailscale Funnel does
  it, so the branch stays unbuilt rather than merely deferred.
