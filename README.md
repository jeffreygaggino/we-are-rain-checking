# we-are-rain-checking

**Does weather predict who wins a Grand Prix — and can I build the answer through an AI-native SDLC end to end?**

Two questions, one repo. The second is the real one; the first exists to give it something real to act on.

---

## Why this exists

I had four Claude Code skills copy-pasted into my global skills directory. When I installed the packaged versions from a marketplace, all four had **drifted** from their sources:

| skill | local | packaged | diff lines |
|---|---|---|---|
| `grilling` | 12 | 28 | 32 |
| `domain-modeling` | 74 | 74 | 36 |
| `prototype` | 26 | 26 | 24 |
| `grill-with-docs` | 7 | 7 | 4 |

The `grilling` divergence wasn't cosmetic — the stale copy specified one-question-at-a-time interviewing, the packaged version specifies round-based frontier questioning. **The drift silently changed how the agent worked, mid-session, and I only noticed because I went looking.** I'd seen the same failure at larger scale elsewhere: an `AGENTS.md` that had drifted ~481 lines from the file it was copied from, and a second one sitting at a workspace root auto-loading nowhere.

Copy-paste distribution of agent instructions fails, fails silently, and fails at n=1. Versioned distribution is the obvious fix.

Which raises a harder question nobody seems to have answered: **the skill pack I installed declares ~1,609 always-on tokens across 25 registered skills and ships zero evals. Do any of them actually do anything?**

That's what this repo is for. The F1 service is the substrate; the experiment is whether the tooling around it earns its keep.

---

## The principle

> **I don't trust a result I haven't seen fail.**

One idea, three layers:

| Layer | The pass | What makes it mean anything |
|---|---|---|
| Code | test goes green | **you saw it red first** |
| Agent | grader scores well | **the no-plugin ablation arm scored lower** |
| Optimisation | "it feels slow" | **you measured before you built the cache** |

A test you've never seen fail might assert nothing. A skill you've never ablated might do nothing. Both manufacture confidence without producing evidence.

---

## Build discipline

Everything is built through the chain, from the first commit, so the git history records the process rather than describing it afterwards:

```
brief → /grill-with-docs → /to-spec → /to-tickets → /implement
                                                      ├─ /tdd  (reference, consulted each cycle)
                                                      └─ /code-review  (at the end)
```

**Test-first, always.** Not for orthodoxy — because when an agent writes the code *and then* the tests, the tests are derived from the code it just wrote. They pass by construction and encode the bug faithfully. Test-first has nothing to copy from: the test comes from the spec, before the implementation exists, so it's an unarguable target rather than a description. The faster the agent, the more this matters.

Tests live at **agreed seams**, never against internals. No test at an unconfirmed seam.

### Working principle: deliberate invocation

I'm the thinker; skills get invoked with judgement, not left to fire on their own — except where a spec or ticket names one explicitly.

This doesn't conflict with the eval plan. *Model-invocable* means a skill **can** fire autonomously, not that it must. And the tension is itself measurable, which is the interesting part: the **ablation delta** answers *does it help when it fires*, while the `tool_used: Skill` indicator answers *did it fire when it shouldn't*. "How much autonomy should this thing get?" becomes a question with evidence attached rather than a preference.

---

## The substrate

Two upstreams, both free and keyless — deliberately, so there are two independent failure modes to handle.

**OpenF1** (`https://api.openf1.org/v1`), verified by probe:

- `session_result?session_key=` → `position`, `driver_number`, `points`, `dnf`, `gap_to_leader`. Winner is `position=1`.
- `weather?session_key=` → **time series**: `date`, `rainfall`, `air_temperature`, `track_temperature`, `humidity`, `wind_speed`, `wind_direction`, `pressure`.
- `meetings?year=` / `sessions?year=` → `location`, `country_name`, `circuit_short_name`, `date_start`, `gmt_offset`. **No lat/lon.**
- **3 req/s, 30 req/min**, no API key. **404s on invalid queries** rather than returning an empty list.
- Data is "live" only ±30 min around a session; outside that window it's stable, so **fixtures are deterministic**.

**Open-Meteo** — free, keyless, forecast. Needed because OpenF1's weather is trackside telemetry with no forward view.

**Circuit → coordinates is a static seeded table (~24 rows), not geocoding.** Deterministic, testable, one fewer network dependency, and it avoids geocoding ambiguity — "Monza" resolves to a town as easily as a circuit.

### Why this substrate is interesting rather than a toy

- **Caching is forced by the constraint, not decorative** — but by a different constraint than it first looks. OpenF1's 30 req/min ceiling binds *ingest*, which is a batch job that can simply sleep; the correlation endpoint reads Postgres and never touches OpenF1 at serve time. The cache that has to exist is on the **forecast** path, where Open-Meteo is hit live on request. Cache-aside with a TTL, hit rate measurable.
- **The join is real work.** Weather is a time series; results are one row per driver per session. Correlating them needs a windowed aggregate joined to the classification — actual SQL and index design.
- **Two upstreams force the question that matters:** what does the endpoint return when one is down and the other isn't?
- **The demo is live.** "Next race" changes on its own, which proves it isn't hardcoded.

### Honesty constraint

**The correlation endpoint reports its own sample size and says the signal is weak when it is.** Shipping "RAIN FAVOURS X" off forty races would contradict the entire premise of the repo. If the honest answer is "this is noise," that's the answer it gives.

---

## The experiment

Two eval targets, with predictions **committed before the run**:

1. **`add-f1-endpoint`** (this repo's own skill) → predicted **largest** delta. It encodes repo conventions the model has no way to know.
2. **`tdd`** → predicted **near-zero**. The model may already do this when asked.

If `tdd` comes back flat, that's the most interesting result available — a widely-used skill that measurably does nothing is worth more than another one that does.

**Only model-invocable skills are meaningfully ablatable.** `to-spec`, `to-tickets`, `implement` and `grill-with-docs` are `disable-model-invocation: true` — without the plugin the command doesn't exist, so the baseline arm fails trivially and the delta means nothing. `tdd` and `code-review` fire autonomously, so both are valid targets. This repo's own skill omits `disable-model-invocation` for the same reason.

Two targets, not the six-step chain: ablating one skill inside a chain can't be attributed, and attribution is the entire point. The harness is built so a third and fourth are cheap.

---

## Delivery

- **GitHub-hosted runner.** Build, test and eval have to run somewhere visible — the Actions log is the record.
- `go test`, then `claude plugin eval --json`, then `claude plugin validate`.
- Deploy joins a Tailscale tailnet **for the final step only**, onto a Proxmox host. No inbound ports open.

The pipeline is the artifact; the URL is just proof it ran.

---

## Scope

**In:** rate-limit-aware resumable ingest · Postgres schema, migrations, deliberate indexes · response cache with a recorded hit rate · correlation endpoint · next-race forecast · one in-repo skill · evals for two targets · GitHub Actions · Tailscale deploy.

**Out:** publishing a marketplace of one (consuming and *measuring* a real one is the more interesting half).

**Deferred:** a frontend, to display what the API returns — the backend ships first and CORS stays unbuilt until there's something to allow. Then hooks. First one to add: `PreToolUse` refusing edits to an already-applied migration — the guardrail is what makes the speed safe. Read `misc/git-guardrails-claude-code` first; it may already do this.

**Later, if it earns the time:** OTel out of the eval runs into Grafana Cloud, plotting eval score over commits.

---

## Findings so far

- The pack ships **35 skills but registers 25** — `in-progress/` and `misc/` are on disk but absent from `plugin.json`.
- An explicit-only skill still costs always-on description tokens in every session but can never fire by itself. Whether that's a fair trade is exactly what the harness is for.
- OpenF1 404s rather than returning empty on an invalid query, which makes "no data" and "bad request" look identical until you check the body.
- **`rainfall` is a binary flag, not a magnitude.** Across all 82 races with weather data the only values present are `{0, 1}`. Drizzle and a downpour are the same number, so rain *intensity* is not measurable from this source at all.
- **`driver_number` is not a driver.** Number `1` belongs to the reigning champion, so it resolves to Verstappen for 2023–2025 and Norris for 2026; `3` covers both Ricciardo and Verstappen. Keying on it merges two people — and it does so quietly, which is the dangerous part. See ADR-0003.
- **The headline question is answered, and the answer is no.** Measured before writing any Go, across 2023–2026 (2022 is a hard 404):

  | test | result |
  |---|---|
  | rain → winner (Verstappen) | +1pp wet vs dry |
  | rain → winner (Norris) | +20pp, ≈1.2 SE — noise at n=9 |
  | rain → retirements | t = 0.89 |
  | wind → retirements | r = −0.087, r² = 0.008 |
  | wind → winner | 48% vs 44% |

  Nine wet races in four seasons is the ceiling, and no further ingest raises it. The correlation endpoint's honest output is *insufficient sample* — which is the result the honesty constraint above was written to permit, arriving before a line of code rather than after a launch.
- **The first version of that table was wrong, and wrong persuasively.** Keying winners on `driver_number` showed a +10pp wet advantage for driver `1`. Resolving identity properly collapsed it to +1pp: the number had merged Verstappen's and Norris's wins. The bug presented itself as a finding, not as an error.
