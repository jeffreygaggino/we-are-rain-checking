# we-are-rain-checking

**Does weather predict who wins a Grand Prix?**

A Go + Gin service over four seasons of Formula 1 weather and results, built to answer that honestly — including when the honest answer is "there isn't enough data to say."

---

## The principle

> **I don't trust a result I haven't seen fail.**

Two layers, one idea:

| Layer | The pass | What makes it mean anything |
|---|---|---|
| Code | test goes green | **you saw it red first** |
| Optimisation | "it feels slow" | **you measured before you built the cache** |

A test you've never seen fail might assert nothing. The second line has already cost this repo a ticket: a forecast cache was designed, specified, and then cut, because nothing had measured the path it was meant to speed up.

**Test-first, always.** When an agent writes the code *and then* the tests, the tests are derived from the code it just wrote — they pass by construction and encode the bug faithfully. A test written first has nothing to copy from.

Tests live at **agreed seams**, never against internals.

## The honesty constraint

**The correlation endpoint reports its own sample size and says so when the sample is too small.** Shipping "RAIN FAVOURS X" off nine wet races would contradict the entire point.

That constraint was written before any Go, and it was immediately needed. Measured across 2023–2026 by direct probe (2022 is a hard 404):

| test | result |
|---|---|
| rain → winner (Verstappen) | +1pp wet vs dry |
| rain → winner (Norris) | +20pp, ≈1.2 SE — noise at n=9 |
| rain → retirements | t = 0.89 |
| wind → retirements | r = −0.087, r² = 0.008 |
| wind → winner | 48% vs 44% |

**Nine wet races in four seasons is the ceiling, and no further ingest raises it.** Asked per Race, the rainfall question is unanswerable and stays that way.

So the endpoint asks it per **Driver-Race** instead. "Did this Race have a wet winner" is one observation; "did this Driver retire from this Race" is about twenty:

| unit | wet | dry | total |
|---|---|---|---|
| Race | 9 | 73 | 82 |
| Driver-Race | ~180 | ~1,460 | ~1,640 |

Both numbers get reported, always. Those 180 Driver-Races come from only 9 distinct weather events, so the effective sample sits somewhere between the two — showing both makes the clustering visible instead of hiding it behind the bigger number.

Three axes — rainfall, wind band, track temperature band — are **reported separately and never merged**. Rain changes tyre choice, wind destabilises aero, a cold track breaks tyre warm-up. Combining them into one "bad weather" score would make any effect unattributable, and would let the threshold be tuned until the sample looked big enough.

## Two findings worth keeping

**`rainfall` is a binary flag, not a magnitude.** Across all 82 races with weather data the only values present are `{0, 1}`. Drizzle and a downpour are the same number, so rain *intensity* is not measurable from this source at all. No column pretends otherwise.

**`driver_number` is not a driver.** Number `1` belongs to the reigning champion, so it resolves to Verstappen for 2023–2025 and Norris for 2026; `3` covers both Ricciardo and Verstappen. The first version of the table above keyed winners on it and showed a **+10pp** wet-weather advantage for driver `1`. Resolving identity properly collapsed it to **+1pp** — the number had merged two people's wins.

**The bug did not announce itself as a bug. It announced itself as a finding.** That's why drivers are seeded with ids this repo owns. See ADR-0003.

## The substrate

Two upstreams, both free and keyless — deliberately, so there are two independent failure modes to handle.

**OpenF1** (`https://api.openf1.org/v1`), verified by probe:

- `session_result?session_key=` → `position`, `driver_number`, `points`, `dnf`. Winner is `position=1`.
- `weather?session_key=` → time series: `date`, `rainfall`, `air_temperature`, `track_temperature`, `humidity`, `wind_speed`, `wind_direction`, `pressure`.
- `meetings?year=` / `sessions?year=` → `location`, `country_name`, `circuit_short_name`, `date_start`. **No lat/lon.**
- **3 req/s, 30 req/min**, no API key. **404s on invalid queries** rather than returning an empty list — and 404s identically for "no results", a malformed year and an unknown path, so the response body is the only way to tell them apart.

**Open-Meteo** — free, keyless, forecast. Needed because OpenF1's weather is trackside telemetry with no forward view.

**Circuit → coordinates is a static seeded table (~26 rows), not geocoding.** Deterministic, testable, one fewer network dependency, and it avoids the ambiguity where "Monza" resolves to a town as readily as a circuit.

### Why this is interesting rather than a toy

- **The rate limit binds ingest, not serving.** OpenF1's 30 req/min ceiling constrains a batch job that can pace itself; the correlation endpoint reads Postgres and never touches OpenF1 at request time. Knowing *which* path a constraint actually binds is what stopped a cache being built where none was needed.
- **The join is real work.** Weather is a time series; results are one row per driver per session. Correlating them needs a windowed aggregate joined to the classification.
- **Two upstreams force the question that matters:** what does the endpoint return when one is down and the other isn't?
- **The demo is live.** "Next race" changes on its own, which proves it isn't hardcoded.

## Endpoints

| route | reads | degrades |
|---|---|---|
| `GET /api/v1/health` | DB ping | 503 when the database is unreachable |
| `GET /api/v1/races/next` | DB + **live Open-Meteo** | 503 naming `open-meteo`, carrying its status and reason |
| `GET /api/v1/correlation` | DB only | unaffected by either upstream |

Ingest is a `cmd/` binary run on a schedule, never a route. That is what makes the absent auth layer a decision rather than an oversight — the two stand or fall together. See ADR-0001.

## Delivery

Everything runs in GitHub Actions, because a pipeline nobody can see is a claim rather than a record.

```
gate      PR + push to main · invokes `make gate` · Postgres from the same compose file
          → required check under branch protection
release   main only · GHCR, tagged sha-<short> and latest
deploy    main only · Tailscale OAuth, tag:ci → SSH over the tailnet
          → pull → migrate → restart
```

The gate mirrors `make gate` rather than reimplementing it, so local and CI cannot drift apart. Migrations run as their own step: a failed migration aborts before the new image serves. The host publishes the API through **Tailscale Funnel** — public HTTPS, no inbound ports opened.

## Running it

One command each, from `backend/`:

| command | what it does |
|---|---|
| `make local` | the whole stack — Postgres, migrations applied, then the API. Returns once `/api/v1/health` answers |
| `make run` | the API alone against whatever `.env` points at |
| `make ingest` | fills the tables from OpenF1. Safe to re-run, safe to interrupt |
| `make dump` | writes the ingested data to a snapshot — gitignored, ~0.6 MB |
| `make restore` | loads that snapshot into an empty dev database |
| `make test` | starts Postgres, creates the test database, runs every test |
| `make gate` | fmt-check → vet → docs-check → build → test |
| `make local-down` | stops the stack |

**Two ways to fill a fresh clone, and the trade between them.** A cold ingest is ~530 paced requests against OpenF1's 30 req/min ceiling — 8 for Meetings and Sessions, 440 for the Weather Samples of every completed Session, ~167 for the classification of every settled Race. **About 22 minutes**, and that pacing is the upstream's rate limit rather than a defect.

```
clone → make local → make ingest    ~22 minutes, needs OpenF1
clone → make local → make restore   seconds, needs a snapshot someone made earlier
```

The snapshot carries the migration version it was taken at, so `make restore` applies any migrations the repo has gained since — and refuses, naming the version, if the snapshot is *ahead* of the repo rather than serving a schema nothing here describes. It is a convenience and never a source of truth: `make ingest` after a restore re-fetches only what the snapshot missed. Nothing in the service depends on a dump existing.

## Where things are written down

| file | holds |
|---|---|
| `CONTEXT.md` | the glossary — the words this repo uses and the ones it avoids |
| `docs/adr/` | decisions that were hard to reverse and surprising without context |
| `backend/plans/` | rationale, rejected alternatives, measured numbers, sequencing |
| `docs/agents/` | working agreements for agents, including the authority on code |
