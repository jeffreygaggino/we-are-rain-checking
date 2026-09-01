# 01 — Backend v1

Implements #1 via #2–#11. This file holds the rationale; the source holds the code and only the
non-obvious comments. Where this plan and `STRUCTURE-go-gin-backend.md` disagree on code, STRUCTURE wins.

## Status

Stopped deliberately after the database layer. Everything below the line is designed, not built.

| ticket | state |
|---|---|
| #4 derivation functions | **done** — red-first, 100% covered |
| #5 seed Circuits and Drivers | **done** — 26 Circuits, 29 Drivers, literal ids, stability test seen to fail |
| #3 Postgres and migrations | **partial** — schema, migrations, DSN handling and the DB half of the harness are in; the health route and the HTTP seam are not |
| #2 service skeleton | **not started** — no `main.go`, `routes/`, `handlers/`, no `swag` docs |
| #6–#11 | not started |

Consequences for what is written below: the endpoint, ingest, cache and client sections are the
intended design and nothing more. The gate currently runs `gofmt`, `go vet`, `go build`, `go test`;
`swag init` joins it with #2, when there is a handler annotation to generate from.

## Fill-ins

| STRUCTURE placeholder | value |
|---|---|
| module path | `github.com/jeffreygaggino/we-are-rain-checking/backend` |
| DB schema | `f1` |
| service directory | `backend/` — pairs with a later `frontend/` (#2: "own top-level directory") |
| image name | `jeffreygaggino/we-are-rain-checking-backend` |

## Deviations from STRUCTURE, and why

STRUCTURE is a greenfield kickoff written for user-authored data behind an auth service. Four of its
rules do not survive contact with this domain. Three are already ADRs; the fourth is new.

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

One addition: **`tests/`**, holding the two agreed seams' harness (#3: "reusable by later tickets
without modification"). Keeping it out of `handlers/` stops the harness being mistaken for a unit
test of the handler package.

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
