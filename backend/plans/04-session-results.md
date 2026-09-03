# 04 — Session Results, attributed to Drivers

Implements #8, stage 3 of ingest. `01-backend-v1.md` holds the schema and the two stages before this
one; `docs/agents/backend.md` is the authority on code. This file holds what stage 3 decided and
what was measured to decide it.

ADR-0003 is the whole ticket. It has been a seeded table and a sentinel error since #5; this is where
it executes.

## What is fetched, and what is not

**Races only.** `/session_result` answers for any Session — practice and qualifying carry a
classification too — and stage 3 asks about Sessions named `Race` (`models.SessionNameRace`) that
have ended. Sprint is excluded by the same name check, as everywhere else in this service.

That is not only a cost decision, though it is one: 96 Races against 495 Sessions. It is what keeps
ADR-0003's fail-fast rule from firing on data no endpoint reads. Measured 2026-09-03, across every
Session OpenF1 carries from 2023:

| population | distinct `full_name` | unseeded |
|---|---|---|
| Race Sessions | 29 | **0** |
| every Session | 71 | **42** |

The 42 are rookie Friday runs and support-series entries — Paul ARON carrying number **1** in 2023
Budapest Practice 1 is the sharpest of them, the same number that belongs to Max VERSTAPPEN in every
Race of that season. Ingesting practice would abort every run on an unseeded name, and the only
repairs available would be seeding drivers who never start a Race or softening the rule ADR-0003
exists to hold. Restricting the stage to Races is the third option, and it is the one the data
supports: **every name that appears in a Race is seeded.**

## Resolution

Per Session, `racing number -> upstream full_name -> seeded driver_id`:

1. `/session_result?session_key=K` — the classification, keyed by `driver_number`.
2. `/drivers?session_key=K` — that Session's entry list, mapping `driver_number` to `full_name`.
3. `full_name` -> `driver_id`, from the seeded table read once at the top of the run (29 rows, the
   same shape as the Circuit resolution stage 1 does).

Per Session and not per season, because that is what makes a mid-season change resolve correctly —
a driver replaced after round 12 carries the number the replacement then carries, and only the
Session's own entry list says who held it that weekend.

**Results first, entry list second.** A Race the upstream holds no results for costs one request
rather than two, and there are 14 of them today (see below). Nothing else depends on the order.

**Four ways this fails, all loudly, all before the transaction opens:**

| condition | error |
|---|---|
| a classified `full_name` that is not seeded | `models.ErrDriverNotResolved`, naming the Session and the name |
| a `driver_number` the Session's entry list does not carry | `models.ErrDriverNotResolved`, naming the Session and the number |
| one `driver_number` mapped to two names inside one Session | `models.ErrDriverNotResolved`, naming both |
| two `driver_number`s classified under one Driver | `models.ErrDriverNotResolved`, naming both numbers |

The last two do not happen upstream today — measured zero across all 9,949 entry rows — and are
guards rather than hypotheses. Each is the same failure from a different side: an ambiguous number
resolved by last-write-wins is the silent merge ADR-0003 records, and two numbers resolving to one
Driver would write two rows on one primary key, the second overwriting the first inside the
transaction. Neither would produce an error. Both would produce a plausible-looking grid.

**Only a number a result row classifies is resolved.** The entry list is fetched whole, but a name on
it that no classification mentions — a withdrawal, a reserve listed and not run — resolves nothing
and so cannot abort a run that has nothing to store for them. Measured: zero such entries across the
82 classified Races today, which is what makes this a narrowing of the blast radius rather than a
feature.

## Resumption, and the one thing stage 2's rule does not transfer

The stage 2 rule one level down, and derived the same way: a Race that already holds result rows is
skipped without a request, and only Races whose `date_end` is past are considered at all.

**But a classification is provisional and weather is not.** Stage 2 rests on a completed Session's
weather never changing; the grid that crosses the line changes when the stewards apply a penalty
afterwards — positions, points and a `dsq` flag, which are three of the things #10 reads. Paired with
a skip that asks only whether rows exist, a run arriving an hour after the flag would freeze the
provisional grid permanently, and the upsert underneath would never be reached.

So a Race is not asked about until `services.ResultsSettleWindow` (24 h) after it ends. A day is a
judgement rather than a measurement: it clears same-evening stewarding comfortably and costs data
that is only ever read historically nothing at all. The alternative considered was re-fetching a Race
until its rows were written outside the window — derived from `updated_at`, so still no checkpoint
table — and it was rejected as a second resumption rule for one saved day of latency. **A decision
later than the window is not picked up**: delete that Race's rows and re-run, which the upsert makes
safe. A Race's
results are written in **one transaction opened after both upstream calls have returned**, so a Race
is stored whole or not at all — the property that makes "has rows" a safe skip.

**A Race the upstream has no results for is re-asked on every run**, exactly as stage 2's silent
Sessions are: nothing records that it was asked. Measured: 14 of 96 Races, being the 2026 Races still
to come and the three cancelled ones. Probed 2026-09-03 — `session_result?session_key=9086`, the
cancelled 2023 Emilia Romagna Race, answers `404 {"detail":"No results found."}`. A cancelled Race
therefore needs no special case here; the flag stays on the Session for #9 and #10 to read.

**Cost.** 82 Races with results at two requests each, plus one for each of the 14 without: 178
paced requests, about 6¼ minutes on a first run and about 30 seconds on a healthy re-run.

**A Race that stored nothing is counted as neither ingested nor skipped.** It is this stage's third
state, and reporting it as ingested would give the operator a tally that looked stable while the same
14 Races were re-asked on every run.

## The wire, and what is strict

Measured over all 1,661 Race result rows, 2026-09-03:

| field | nulls | stored as |
|---|---|---|
| `position` | 207 | `*int` — a Retirement has no position, and a sentinel would sort into the classification |
| `number_of_laps` | 7 | `*int` |
| `points` | 0 | `float64`, **strict** |
| `dnf` / `dns` / `dsq` | 0 / 0 / 0 | `bool`, **strict** |

`points` is null on practice and qualifying rows and never on a Race, so a null arriving here means
the shape changed rather than that a Driver scored nothing — and `0` is the score of most of the
grid, so a guessed one is indistinguishable from an observation. The retirement flags are strict for
the reason `rainfall` is: #10's headline number is the retirement rate, and a missing flag read as
`false` moves it with nothing to show for it.

**A missing position is not a Retirement, and #10 must read the flags.** Across all four seasons: 234 rows
carry `dnf`, `dns` or `dsq`, of which **31 still carry a position** — a car retired near the end is
classified if it covered enough of the distance — and 4 rows carry neither a position nor a flag.
So `position IS NULL` counts 207 where the Retirement count is 234, and the two answers differ by
13%. The flags are stored as three separate booleans rather than collapsed into one, because a DNS
is a Driver who never started and a DSQ is a result taken away afterwards; only one of the three is
the Retirement CONTEXT.md defines.

**`gap_to_leader` and `duration` are dropped at the client boundary.** `gap_to_leader` is a union of
`0`, `11.987`, `"+1 LAP"` and `null` in one field (01), and neither is read by any endpoint.

## Idempotence

`ON CONFLICT (session_key, driver_id) DO UPDATE`, one row per statement rather than a multi-row
`INSERT` — the same reason stage 2 writes samples singly: Postgres refuses to let one statement's
`ON CONFLICT DO UPDATE` touch a row twice, so an upstream that repeated a Driver would fail the
ingest instead of writing it once. Measured zero duplicate `(session_key, driver_number)` pairs
today, which is what makes this a cheap guarantee rather than a workaround.

## Rejected

- **Keying rows on the Racing Number**, with the Driver resolved at query time. It is the shape that
  produced the wet-weather advantage ADR-0003 records, and it moves a decision that has one right
  answer per Session into every query that later reads the table.
- **Seeding a Driver on first sight.** An unrecognised name is either a rookie who never starts a
  Race or a spelling change; inserting either splits one person's results across two ids, silently.
- **A `results_ingested_at` column** to close the re-ask gap on Races with no results. Same answer
  as stage 2: a second source of truth for a fact the rows carry, bought with 30 seconds of a
  scheduled run.
- **Ingesting every Session's results** so the table can answer questions nobody has asked. It costs
  42 unseeded names, and every one of them is an aborted run.
