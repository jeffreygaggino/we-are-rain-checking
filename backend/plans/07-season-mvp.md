# 07 — Season MVP: this season's Driver-Races, and the next Race

Narrows the serving scope set out in `02-scope-reset.md`. Ingest, pacing, the envelope, 404
discrimination and the test seams are untouched. What changes is **what the API answers**: two
endpoints and a page that renders them, and no endpoint that makes a claim.

**The goal is to get something out.** Everything below is chosen for the shortest honest path to a
URL a stranger can open, and anything that does not serve that is deferred rather than argued about.

## The scope

| part | answers |
|---|---|
| `GET /api/v1/seasons/{year}/driver-races` | every Driver's finish in every Race run that season, each row carrying its Race's weather and wind |
| `GET /api/v1/races/next` | the next Race that has not started, with a live Forecast |
| `frontend/` | a React page: the season table, and a "what's coming" panel |

**End results only.** Where a Driver finished, not where they started — Starting Position is real,
measured and cheap, and it is still deferred to its own ticket. See *Deferred*, below.

## #10 is parked, not closed

The correlation endpoint stays open and unscheduled. **The service now reports rows, not Signal** —
it serves the observations and the reader draws the conclusion.

Three consequences, recorded so nobody reads silence as agreement:

- `derive.MinimumSampleSize`, `derive.SampleVerdict` and `derive.Verdict` lose their only intended
  consumer. They stay in the tree, pinned by their own unit tests — which is exactly the hazard #10's
  own acceptance criteria named: *"until a caller exists, those constants are pinned only by their
  own unit tests, so a wrong threshold would ship green."* That was true of the thresholds then and
  is true of the verdict now.
- `derive.WetSessionThreshold` and `IsWetSession` likewise. The season endpoint reports raw counts,
  so nothing in the serving path decides what "wet" means.
- `derive.WindBands()`'s doc comment says *"the correlation endpoint reports a split per band"*.
  That endpoint is not coming in this scope. The comment needs to stop naming it.

**Nothing in `derive/` is deleted.** Parked means parked, and the constants carry their reasoning.
But an unconsumed decision function is a decision nobody is making, and this plan is the record that
it was noticed rather than overlooked.

`models.ErrUnknownAxis` has no handler to be returned from either. Same treatment: left, noted.

**One that predates this plan:** `config.ForecastCacheTTL` is declared and loaded from
`FORECAST_CACHE_TTL`, and **nothing reads it**. It is config for #11, which 02 cut. A knob with one
value and no consumer is the shape the working agreement calls a guess, and this one outlived the
feature it was guessing at. Delete the field, the loader line and both `.env` entries when the
next change touches `config/`; #11's own reasoning is the record, and it comes back with the cache
if the forecast path is ever measured slow.

## Measured state, 2026-09-10

From the filled dev database, not estimated:

From the filled dev database after a re-ingest, not estimated:

| season | Races | of which cancelled |
|---|---|---|
| 2023 | 23 | 1 |
| 2024 | 24 | 0 |
| 2025 | 24 | 0 |
| 2026 | 25 | 2 |

2026: **13 Races run and ingested**, 22 Driver-Races each, 146–208 Weather Samples each. The next
Race is the **Spanish Grand Prix, 2026-09-13, `session_key` 11369**.

Today the season endpoint would return **286 Driver-Races**; at season end, 23 × 22 = **506**.

### Cancelled Races are real, and they are not an ingest gap

Bahrain (2026-04-12) and Saudi Arabian (2026-04-19) hold zero results and zero Weather Samples.
That was the open question this plan started with, and it is **settled**: every Session in both
Meetings carries `is_cancelled = true`, the upstream returns `No results found.` for both, and the
re-ingest that filled the Italian Grand Prix exited 0 without touching them. Those Meetings did not
happen.

2023 carries one as well, so this is a property of the seasons rather than a 2026 quirk.

**`f1.sessions.is_cancelled` already exists and is already populated correctly.** Ingest was right;
there is nothing to fix. What it changes is Endpoint A's query — see below.

## Endpoint A — the season

### Why a `{year}` path segment rather than "this season"

The ask was this season. The route still takes the year, for two reasons: three other seasons are
already stored and would otherwise be unreachable rows, and a literal `/current` is a route that has
to be replaced the first time anyone wants last season. One path segment over data that already
exists is not the speculative seam the working agreement warns about — it is the data's own shape.
The frontend calls the current year and gets exactly what was asked for.

A year outside the **Season Range** is a client error naming the range, not an empty list. A year
inside it with nothing stored is an empty list — `[]`, never `null`.

### Cancelled Races are excluded, on the flag and not on emptiness

`WHERE NOT s.is_cancelled`. Three seasons in four carry at least one, and without the filter the
season table shows ghost Races — a row per cancelled Meeting with no Driver, no result and no
weather, which reads as a bug in the endpoint rather than a Grand Prix that was called off.

**Not "exclude Races with zero results".** That is the display-string mistake in another form:
inferring a state from the absence of rows rather than reading the flag that records it. The two
coincide today and stop coinciding the moment a Race is run and its results have not landed yet —
which is the normal state of every Sunday evening, and would silently drop a real Race from the
table. `is_cancelled` is the stable identifier here; emptiness is a symptom.

A cancelled Race is therefore absent from the response, not present-and-null. It was not run, so
there is no Driver-Race to report.

### One row per Driver-Race

The unit `CONTEXT.md` already names. The Race and weather blocks repeat across a Race's 22 rows;
that is the cost of the flat shape and it was chosen with that known. Estimated ~520 bytes a row,
so a full season lands near **260 KB** — an estimate from the field list, to be measured against the
real response before it is quoted as fact.

**No pagination.** 02 dropped it for the bands endpoint as machinery serving a spec artifact rather
than a caller; the same holds here with a number behind it. A season is the page — the caller asked
for a year and a year is bounded at **506 rows** and cannot grow past 24 Races.
`NewPaginationResponse` stays unused. Reopen if the measured response clears ~1 MB, which this shape
cannot reach.

### The weather block is raw, with no threshold in it

Counts and aggregates only. No Wet Fraction, no `wetSession` flag, no wind band — the response
carries no threshold, so the caller decides what wet and windy mean.

Per Race: total Weather Sample count, the count recording Rainfall, mean and max wind speed in m/s,
mean air and track temperature.

Two traps to build against:

- **`AVG` ignores NULL and `COUNT(*)` does not.** `wind_speed`, `air_temperature` and
  `track_temperature` are all nullable. A mean over 150 samples of which 40 have no wind reading is
  a mean of 110, and reporting it beside a sample count of 150 states something false. Each
  aggregate carries **its own** non-null count, or it is not reportable.
- **A Race with no Weather Samples reports `null`, not zeros.** Zero wind and zero rainfall samples
  encode as calm and dry. Guard and return the absent case explicitly — three of this season's
  Races are in exactly this state today.

### The query

Aggregate Weather Samples per Session in a CTE, *then* join. Joining ~550 result rows against
~150 samples each before grouping multiplies to ~80,000 rows and collapses them again.

Indexes: `sessions_year_name_idx` serves the year + `Race` filter, `weather_samples`' composite
primary key serves the group-by, and `session_results`' primary key serves the per-Session lookup.
**No new index is expected** — that is a claim to prove with `EXPLAIN` on the filled dev database
when the query is written, not to assert here.

## Deferred — Starting Position, the field Endpoint A does not carry

**Not in this MVP.** End results only. Recorded here because it was probed rather than guessed, so
its ticket starts from measurements instead of repeating the investigation.

**There is no starting position in the schema, in `models.SessionResult`, or in the OpenF1 client.**
It would be new ingest. From probing the upstream:

- `/starting_grid` exists and returns `position`, `driver_number`, `lap_duration`, `meeting_key`,
  `session_key`.
- **It is keyed on the Qualifying Session, not the Race.** The 2026 Australian Race is
  `session_key` 11234; its grid is filed under 11230, that Meeting's Qualifying.
- **A sprint weekend has two of them.** The Chinese Meeting carries 11236 `Sprint Qualifying` and
  11241 `Qualifying`, both with grids, both `session_type` `Qualifying`. Only the second is the
  Race's grid.
- **Coverage is partial.** The opening Races of 2023, 2024, 2025 and 2026 all return zero grid rows;
  rows appear from around April 2023 onward. So the field is **nullable by measurement**, not by
  caution.

Three things that make it more than a column:

1. **`driver_number` is not identity (ADR-0003).** The grid rows carry nothing else to resolve on.
   Resolve each number through the *same Race's* `f1.session_results` rows, which already hold
   `racing_number` alongside the resolved `driver_id`. A grid row whose number has no result row is
   **dropped**, not inserted — a strict reader, per the working agreement, because a number that
   does not classify is noise rather than legacy data.
2. **Finding the right Qualifying means keying off `session_name`,** a display string, which the
   working agreement forbids branching on. The repo already makes this exact exception —
   `models.SessionNameRace = "Race"`, with Sprint deliberately excluded. A matching
   `SessionNameQualifying` const, and the same deliberate exclusion of `Sprint Qualifying`, keeps
   one rule rather than two.
3. **Where it lives:** `starting_position integer` nullable on `f1.session_results`. Not a new
   table — it is one attribute of the same Driver-Race under the same primary key. The table is
   ingest-owned and re-ingest updates rather than deletes (ADR-0002), so no backfill and no tolerant
   reader is needed; the column is simply null until the grid step runs.

Cost: one extra upstream call per Race. ~96 Races at the upstream's 30 req/min is **about three
minutes** added to a cold ingest, and nothing on a warm one.

`CONTEXT.md` has no term for a grid slot. That is a real gap rather than invented language — note it
for `/domain-modeling` when the ticket is written, and do not ship a field naming a concept the
glossary has not defined.

**Adding it later costs one nullable field on a response the page already reads,** which is the
cheapest kind of change to defer and the reason deferring it is safe.

## Endpoint B — the next Race with a Forecast

**#9 stands as specified.** Its criteria — the next Race derived from data, Open-Meteo behind its own
client with an explicit timeout, a 503 naming which upstream failed, upstream statuses propagated
rather than flattened, the Forecast shape confined to its client, internal errors never echoed —
are unchanged and are the reason CI is worth having.

One criterion is worded for an endpoint that is no longer being built: *"The correlation path is
unaffected by Forecast upstream failure."* It becomes **the season path**, which is the same
property — a database-only endpoint stays available while a live upstream is down — asserted against
the endpoint that now exists. The issue needs that one edit.

`models.ErrNoUpcomingRace` already exists for the December case and is a real answer, not a fault.

## The page — `frontend/`, in this repo

**This reopens 02.** *"Everything else is out. No frontend"* was written when the deliverable was the
API alone. It is overruled deliberately and recorded here rather than quietly: a portfolio project
whose public URL returns JSON is a URL nobody looks at twice.

The repo was built expecting this. #2's first line: *"The service lives in its own top-level
directory so a frontend can be added later without restructuring."* So `backend/` gains a sibling
`frontend/`, and nothing moves.

**React, built by Vite.** The page is a React app; the *serving* of it is what stays with Go.

### Served by the Go binary, same origin

Vite builds to a `dist/`, and the API serves those files itself — `go:embed` and one route,
alongside the `/docs` route that already works this way. React changes what is in `dist/`; it does
not change who serves it.

Two properties this buys, and they are why it beats a separate static host:

- **No CORS, ever.** #2's criterion — *"No CORS configuration is present; there is no browser client
  yet to allow"* — was about to expire. Same origin keeps it true rather than importing
  `gin-contrib/cors` and picking an allowlist. 01's Deviation 3 stands unamended.
- **#22 does not change.** One image, one container, one Funnel hostname, one thing that can be
  down. A separate host would add a second deploy target at the exact moment the point is to get
  something out.

### The build-order trap, and how it is answered

A bundler means Node in the image and a `dist/` that is a build output. That is fine on its own. The
part that bites is specific:

**`//go:embed` fails at compile time when its pattern matches nothing.** Not at runtime — the Go
build itself errors with `pattern: no matching files found`. So on a fresh clone, or in CI, or for
anyone who has not run a Node build, `make gate`'s `build` step goes red for a reason that has
nothing to do with the Go code they are changing.

**The answer: a committed placeholder the Vite build never overwrites.**

- Vite builds to `frontend/dist`; `make frontend` copies it to `backend/web/`, which is what the
  embed points at.
- `backend/web/placeholder.html` is **committed**. `.gitignore` takes `backend/web/*` with an
  exception for it. Vite emits `index.html`, never that name, so the working tree is never dirtied
  by a build.
- The embed pattern therefore always matches at least one file, and `go build ./...` works on a
  fresh clone with no Node installed.
- The route serves `index.html` when it is there and the placeholder when it is not — the absent
  case handled explicitly and answered with "the page is not built, run `make frontend`", rather
  than a 404 or a zero value flowing onward.

**The placeholder cannot reach production.** The Dockerfile is multi-stage: the Node stage builds
`dist` and the Go stage embeds it. If `npm run build` fails, that stage fails and no image is
produced — Docker gives this for free. The placeholder is reachable only from a local `go run .`
where nobody built the page, which is exactly who it is written for.

### The dev loop, where same-origin is easiest to lose

In development there are two processes — `make run` on 8080 and Vite on 5173 — and therefore two
origins. The reflex fix is `gin-contrib/cors` with `localhost:5173` allowed, and that is how CORS
ends up in the binary permanently after being added for a dev convenience.

**Vite's `server.proxy` forwards `/api` to the Go API instead**, so the browser sees one origin in
development as well. The no-CORS decision then holds in every environment rather than only the
deployed one, and the page's fetch paths are the same string in dev and in production.

No new Make target is required for this to work. A `make dev` running both processes is a
convenience, and belongs with `make frontend` in the page ticket — not before it.

**Rejected: making `make gate` depend on the Node build.** It is the obvious fix and it is honest —
the gate would prove the real deployed artifact compiles. It was rejected because it puts Node in
the gate's critical path, which means Node in CI, and 02 chose one gate job specifically so that
*"local and CI cannot diverge"*. A gate that needs two toolchains has twice the surface to diverge
on, and it slows the loop for every backend change to serve a frontend that changes on its own
schedule. Revisit if the placeholder is ever observed being served somewhere it should not be.

The gate stays `fmt-check → vet → docs-check → build → test`, Go-only, unchanged.

### What it renders

The season table from Endpoint A, and the next Race with its Forecast from Endpoint B — which
is the whole point of B being the one route with a live upstream. **The page must show the
degradation, not hide it:** when Open-Meteo is down, B answers 503 naming it, and the panel says the
Forecast is unavailable while the season table beside it carries on. A page that renders a spinner
forever throws away the property the two-upstream design exists to demonstrate.

**The empty weather case will not be on screen to catch you.** After the re-ingest every Race in
the response carries Weather Samples, and the two that carried none are cancelled and filtered out.
So the null weather block is unreachable in today's data and still has to be handled — it appears on
a Sunday evening, when a Race has run and one of the two ingest steps has landed without the other.
Handle it because the shape allows it, not because the page currently shows it.

## Sequence

Ordered so the deployable thing exists as early as it can, and everything after it lands on a live
URL rather than in front of one.

1. **This plan.**
2. **Re-ingest** — *done.* Filled the Italian Grand Prix; established that Bahrain and Saudi Arabian
   are cancelled rather than missing, which turned a scheduling question into Endpoint A's
   `is_cancelled` filter.
3. **#10 parked** — a comment recording the rescope and why it was parked rather than closed, and
   `ready-for-agent` removed so it stops reading as scheduled work.
4. **`derive/` comment fixed** — `WindBands()` stops naming an endpoint that is not coming.
5. **New ticket: Endpoint A** — the season's Driver-Races.
6. **#9** — reworded, then built.
7. **#22** — release and deploy. **Moved up:** 02 put it last because it needs an OAuth client, an
   ACL tag, an SSH key and a host, all made by hand. Those are still the long pole, which is the
   argument for starting them once there is something worth deploying rather than after everything
   else is done. Two endpoints is worth deploying.
8. **New ticket: the page** — against the deployed API, so "it works locally" is never the claim.
9. **New ticket: Starting Position** — the deferred field, on the shape the page already reads.

Steps 7 and 8 are the ones that satisfy "just want something out". Everything below step 8 is
improvement on a thing that already exists.

## Open

**Whether the deployment topology earns an ADR.** 02 left this to be decided when #22 is built.
Serving a browser client from the API container, behind Funnel, on a Proxmox host is more of a
trade-off than it was when the answer was JSON only — and ADR-0001 already turns on the reasoning
that a public URL does not reopen the auth question. Decide it while building #22, not before.

**What the season table should actually show.** 506 Driver-Races is a lot of table. Grouping,
default sort and whether every Race is expanded at once are page decisions, and they are the kind
that are faster to settle by looking at the rendered page than by specifying in advance. Left open
on purpose.

**React's own choices** — TypeScript or not, router or not, data fetching by hand or with a library
— are the page ticket's to make, not this plan's. One note that is not a preference: the season
response is ~260 KB of JSON on one request, so whatever fetches it should render an explicit loading
state rather than assuming it is instant on a phone.
