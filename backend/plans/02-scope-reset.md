# 02 — Scope reset

Supersedes the framing of `01-backend-v1.md`, not its engineering. Everything 01 says about schema,
ingest, pacing, 404 discrimination and test seams stands. What changes is **what this repo is for**,
and therefore where it stops.

## The reset

01 was written under the framing in the README: the F1 service is a *substrate*, and the real question
is whether agent skills measurably do anything. That framing is dropped.

**This repo is a portfolio project.** A small, honest Go + Gin service over four seasons of F1 weather
and results, delivered through a real CI/CD pipeline to a publicly reachable host. The skills
experiment is a separate project — it needs a plugin to evaluate before an eval suite can exist, its
eval directory is expected to live *below the plugin*, and its hard part (controlling what each
ablation arm can read) is experimental design rather than engineering. It starts from
`claude plugin init`, not from here.

Consequence: the backend is no longer built to be *plausible*. It is the deliverable.

## Why the drift exhibit goes

The README's motivating evidence was a table of four skills that had diverged from their packaged
sources, ranked by diff line count. Re-read line by line, that table measures the wrong thing:

| skill | diff lines | what actually changed | behavioural |
|---|---|---|---|
| `domain-modeling` | 63 | em-dash → colon, in every hunk | no |
| `prototype` | 28 | mostly punctuation, but logic prototypes moved from *"a tiny interactive terminal app"* to *"a single shareable HTML file"* | yes |
| `grilling` | 33 | one-question-at-a-time → round-based frontier questioning | yes |
| `grill-with-docs` | 8 | prose invocation → "call the Skill tool twice" | marginal |

Two of four changed what the agent produces. Line count ranks a punctuation pass second-worst, which
is the same failure mode as ADR-0003: **a number that looks like a finding and is not measuring the
thing it claims to.** Rather than re-present it with a better metric on n=4 from one laptop, it goes.

## Deletions

| target | what goes |
|---|---|
| `docs/adr/0004-evals-run-against-a-fixture-repo.md` | whole file — records a decision for work not being done |
| `docs/drift/` | all five files |
| `CONTEXT.md` | the "The experiment" section: Drift, Ablation, Ablation Delta, Treatment/Baseline Arm, Fixture Repo |
| `README.md` | the experiment half — skills motivation, "The experiment", the ablation row of the principle table, eval entries under Scope |
| `.gitignore` | `evals/results/`, `*.results.json` |

The README's principle survives with its middle layer removed. A test goes green **because you saw it
red first**; a cache gets built **because you measured**. The second is why #11 is cut.

## Dangling references from the STRUCTURE deletion

`STRUCTURE-go-gin-backend.md` was removed in `0fff05e`. Eleven references outlived it.

| file | fix |
|---|---|
| `plans/01-backend-v1.md:4` | delete "where this plan and STRUCTURE disagree on code, STRUCTURE wins" — it points at nothing |
| `plans/01-backend-v1.md` "Deviations from STRUCTURE" | rewrite as **Conventions**, stated positively |
| `plans/01-backend-v1.md` fill-in table | image name → GHCR |
| `plans/01-backend-v1.md` rejected-TLS entry | keep the reasoning, drop the citation |
| `docs/adr/0001-no-auth-layer.md`, `docs/adr/0002-natural-keys-for-ingested-data.md` | bodies untouched; one italic line each recording that STRUCTURE was removed and where the conventions now live |
| `AGENTS-backend.md` | **done** — moved to `docs/agents/backend.md`, STRUCTURE line removed, placeholders filled, pointer added to `CLAUDE.md` |

**The ADR bodies are not edited.** An ADR records what was argued and against what. Rewriting one to
hide its opponent is precisely the silent divergence this repo was started over.

**Deviations become conventions.** Stated as deviations from a deleted file they are unreadable; the
seven items are the actual working rules — no permissions layer, natural keys on ingested tables and
literal ids on seeded ones, `derive/` for pure decision functions, `app/` as composition root, health
checks `db.Ping` directly, all routes in `SetupRouter` including docs.

**The working agreement moves to `docs/agents/backend.md`** and becomes the sole authority on code. It
already claimed the role — *"STRUCTURE is the kickoff prompt … this file governs everything after
that"* — and the skeleton stands. It sat at the repo root **untracked**, having been deleted from git
by `0fff05e` alongside STRUCTURE, so the conventions had no committed home at all. `docs/agents/`
already holds `domain.md`, `issue-tracker.md` and `triage-labels.md`, and `CLAUDE.md` already points
there, which makes it one entry point rather than two.

**Five sections were cut in the move.** It arrived generic and had never been trimmed. *Auth on every
handler* contradicted ADR-0001 outright — a working agreement mandating what an ADR forbids means the
next agent follows whichever it reads first. *Proxying and passthrough*, *One service owns a
resource*, *Cross-service contracts* and *Reading logs and metrics honestly* were written for a
service that streams bodies and shares infrastructure with a fleet; this one proxies nothing and talks
to two read-only upstreams. What remains describes this service. ADR-0001 now stands alone on auth.

## Ticket line

| ticket | state | reason |
|---|---|---|
| #13 empty-upstream guard | **build** | real bug: ingest exits non-zero on every scheduled run for ~two months a year |
| #7 Weather Samples | build | per 01 |
| #8 Session Results | build | per 01 |
| #9 next Race + Forecast | build | the only route with a live upstream — two clients, a timeout, a 503 naming which failed. It is what makes CI worth having |
| #10 correlation | build, rescoped | see below |
| #12 production defaults | **build** | see below |
| #21 gate in CI | **build** | new — the gate only runs where someone remembers to run it |
| #22 publish and deploy | **build** | new — GHCR, Tailscale, Funnel |
| #11 forecast cache | **cut** | see below |

**#11 is cut.** Its acceptance criteria are a TTL, hit/miss counters on `/health`, a
stale-never-served guarantee and a seam test counting upstream calls. No measurement motivated any of
it. Open-Meteo is free and keyless, the next Race changes weekly, and the hit rate would be ~100%
against no traffic — a number that measures nothing. Closed citing the repo's own principle; reopen
when the forecast path is measured slow or rate-limited.

**#12 is back in.** 01 could argue it was moot while nothing deployed. It deploys now, behind Tailscale
Funnel — so there is a proxy in front by design, `gin.Default()` trusts it and every other caller for
`ClientIP()`, and debug-mode route logging goes to a real host whose health check polls every two
seconds. Both criteria bite.

**Everything else is out.** No frontend, no hooks, no OTel/Grafana.

## #10 — the unit of analysis changes

01 leaves the rainfall axis permanently at `insufficient_sample`: nine wet Races against
`MinimumSampleSize = 30`, over a corpus that cannot grow. The tempting fix is to broaden "wet" into a
composite *inclement weather* term until the sample clears the threshold. **Rejected**, for two
reasons:

1. Each component is already measured null — rain → retirements `t = 0.89`, wind → retirements
   `r² = 0.008`, wind → winner 48% vs 44%. Combining null predictors adds researcher degrees of
   freedom, not information. A threshold tuned until *n* clears 30 was fitted to the sample size
   rather than to the weather.
2. Rain changes tyre choice and grip; wind destabilises aero; a cold track breaks tyre warm-up. Three
   mechanisms merged under one label because it is convenient is structurally ADR-0003's failure —
   different things collapsed into one identifier, quietly, producing a number that looks like a
   finding.

**The predictor was never the problem; the unit was.** "Did this Race have a wet winner" is one
observation per Race. "Did this Driver retire from this Race" is ~20:

| unit | wet | dry | total |
|---|---|---|---|
| Race | 9 | 73 | 82 |
| Driver-Race | ~180 | ~1,460 | ~1,640 |

`MinimumSampleSize` clears without touching the definition of Wet Session.

Three axes, **reported separately and never merged**: rainfall (binary), wind band (2/4/6 m/s), track
temperature band. Each carries counts, rates, and **both sample sizes** — Races and Driver-Races.
Those 180 Driver-Races come from 9 distinct weather events, so the effective sample for a weather
effect sits somewhere between the two; reporting both makes the clustering visible instead of
hiding it behind the larger number. Still no inference: counts and rates, no p-values, no intervals.

**No pagination.** 01 routed `bands` through `NewPaginationResponse` to satisfy a spec story asking for
list responses with total counts, while conceding that paging two bands is "pointless in practice."
It is dropped.

`CONTEXT.md` gains **Driver-Race**: one Driver's participation in one Race — the unit of analysis for
retirement rates. `Insufficient Sample` now applies per axis rather than to the service as a whole,
which is the more interesting endpoint: some axes answer, some decline, and it says which.

**Open, deliberately:** track temperature band edges. Set them in `derive/` when #10 is written, with
the reasoning recorded the way the wind edges were.

## Pipeline

```
.github/workflows/ci.yml

gate      PR + push to main · mirrors `make gate` exactly · Postgres service container
          → required check under branch protection
release   main only, needs: gate · GHCR, tags sha-<short> and latest · GITHUB_TOKEN, packages: write
deploy    main only, needs: release · tailscale/github-action, OAuth client, tag:ci
          → SSH over the tailnet → pull → migrate → restart
```

**One gate job, not parallel lint/test.** It mirrors `make gate` so local and CI cannot diverge — the
same failure the deleted drift exhibit was about, applied to our own pipeline. Splitting it would be
an optimisation on a build measured at nothing, which is what #11 was cut for.

**Branch protection is the part that matters.** A required check that has actually blocked a merge is
evidence; a green badge over an unprotected main is decoration.

**GHCR over Docker Hub.** Same account, `GITHUB_TOKEN` with `packages: write`, no external credential,
and the package appears on the repo page where a reader will see it. Both `sha-<short>` and `latest`:
the sha tag is what makes a deployed container traceable to a commit.

**Ordering is pull → migrate → restart,** and a failed migration aborts before the new image serves.
01 made migrations a one-shot compose service so that "the schema changed" has its own exit code; that
property only holds if deploy runs it as a step that can fail on its own.

**OAuth client, not a reusable auth key.** Ephemeral tagged nodes clean themselves up and leave an
audit trail; auth keys expire and break the pipeline silently in 90 days. ACL scopes `tag:ci` to the
Proxmox host alone. Secrets: `TS_OAUTH_CLIENT_ID`, `TS_OAUTH_SECRET`, `DEPLOY_SSH_KEY`.

**Tailscale Funnel publishes the API** at a `*.ts.net` hostname. Without it the deployment is
tailnet-only and unreachable by the audience this repo is now for — a URL in a portfolio README that
returns nothing for everyone except its author. Funnel keeps the property that made the design
interesting: no inbound ports opened. See ADR-0001 for why a public URL does not reopen the auth
question.

**Postgres in CI comes from compose, not a `services:` block.** `make test` depends on `make test-db`,
which runs `docker compose up -d --wait postgres` and then `docker compose exec` to create the test
database. A GitHub `services:` container is not reachable through `docker compose exec`, so using one
would mean rewriting the Makefile to suit CI — a second divergence between local and CI, introduced by
the very job meant to prevent one. Runners have Docker; the same compose file serves both.

## Open

**Track temperature band edges** — set in `derive/` when #10 is written, reasoned the way the wind
edges were. The only thing left deliberately unsettled.

## Sequence

1. **This plan.** *(done)*
2. **Deletions and the dangling-reference fixes** — documentation only, one logical change at a time. *(done, gate green)*
3. **Issue tracker brought in line** — #11 closed, #10 rescoped, #1 rewritten, #21 and #22 opened. *(done)*
4. **#13** — the bug. Cheap, red-first, and it proves the loop still works before anything larger.
5. **#21** — the gate in CI plus branch protection. Before more features, so everything after it lands green, and the required check gets seen refusing a merge while the stakes are low.
6. **#7, #8, #12, #9, #10** — ingest, then production defaults, then the two remaining endpoints.
7. **#22** — release and deploy. Last, because it needs the OAuth client, the ACL tag, the SSH key and the host, all created by hand.

An ADR for the deployment topology only if it turns out to be one — hard to reverse, surprising, and
the result of a real trade-off. Funnel-over-Proxmox may qualify; decide when #22 is built, not before.
