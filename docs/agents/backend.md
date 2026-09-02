# Backend working agreement

Working agreement for AI agents on the backend service. **This file is the authority on code.** It arrived as a
generic `AGENTS-backend.md` alongside a `STRUCTURE-go-gin-backend.md` kickoff prompt; STRUCTURE was removed on
2026-09-03 once the skeleton stood, and the conventions it was overruled by live here and in `plans/01-backend-v1.md`.

## Project specifics

- **This service is called:** `we-are-rain-checking` — the backend lives in `backend/`. Not its container alias.
- **Services it talks to:** **OpenF1** (`api.openf1.org/v1`, owns Meetings, Sessions, Weather Samples, Session
  Results) and **Open-Meteo** (owns Forecasts). Both free, keyless, and reached only through `client/`.
- **Plans live in:** `backend/plans/` — markdown, numbered `NN-` by creation date, one per feature or investigation.
- **Verification gate:** `make gate` from `backend/` — `fmt-check` → `vet` → `docs-check` → `build` → `test`.
- **Never run:** `make migrate-down` (drops the schema), any `docker compose down -v` (drops the volume with it), and
  any **deploying** target once one exists. Check the Makefile before running a target: a target named for an
  environment acts on that environment. The local stack is `make local` / `make local-down`.

---

## How we work together

**Answer in the register asked.** "Help me understand this", "what are the groupings", "does this feel right" — the
analysis *is* the deliverable. Don't convert a thinking question into a numbered implementation sequence ending in
"shall I start?". A classification question wants a classification; a judgement question wants a verdict with reasons.
State a recommendation without a schedule. If action genuinely follows, one sentence is enough. The reverse holds too:
when I say "let's do it", stop analysing and build.

**A feature described is the finished state.** When I describe how something works, I mean the completed design, not
what's built today. Don't correct me with the current partial implementation. Raise the gap only when it changes the
decision in front of us ("this doubles upstream traffic once sync lands" is useful; "that doesn't work yet" is not).
Ask if it's genuinely ambiguous which I mean.

**Plan before code.** For any non-trivial feature — and for any investigation worth keeping — write or update a
markdown plan first and let me refine it. The plan is the alignment document and the home for depth: rationale,
rejected alternatives, measured numbers, sequencing.

**I read every diff.** Use file-edit tools, one logical change at a time, announced as "edit 1 of 3" when a step needs
several — I read them as they land and interrupt mid-step when something's wrong. That's the point. Never rewrite a
file via a script or heredoc: it bypasses review, and edit tools fail loudly on a stale match where a script silently
does the wrong thing. If an edit fails on whitespace, read the exact lines and match them rather than reaching for a
script.

**I commit my own work.** Never run `git commit`. Run the gate, report the state, leave staging and committing to me.

**Check state, don't assert it.** Never claim something is committed, uncommitted, running, migrated, or configured
without checking first. I commit iteratively, so the tree is usually cleaner than you'd assume. The same goes for
anything you read in a note or memory — those are point-in-time observations; verify against current code before
stating them as fact.

**Measure before theorising.** When a bug report says "sometimes" or "intermittent", stop generating hypotheses and
start instrumenting. A wrong theory doesn't cost your time, it costs mine testing it.

- Log the state that *distinguishes* the candidates, not the symptom.
- Include a control — a path that should be unaffected if the theory is right.
- Say what each possible output would mean *before* running it, so the result is interpretable on arrival.
- Distinguish "intermittent" from "deterministic": a symptom whose position varies with load or input pattern looks
  like a race and often isn't.
- Check the boring cause first. A transient gateway error is a restarted container until proven otherwise.

**Name things by their canonical name.** Use the repo/module name, not the container or deployment alias. Reserve the
alias for literal command output (`docker logs <alias>`). Mixing them makes it ambiguous which layer a fault is being
attributed to — exactly the confusion that matters when splitting a fault between two services.

**Keep commands scoped.** Prefer operating on named files over directory- or repo-wide invocations, and never run a
destructive or unbounded command against a shared environment to "check" something.

**Finish, then stop.** Run the gate, report what passed and what didn't with the actual output, and stop. Don't
volunteer adjacent refactors.

---

## Code

### Reuse before building

Before writing a helper, client or middleware, look for one that exists — first in this service, then the framework,
then the standard library. Say what you checked and why it didn't fit rather than asserting "nothing fits"; that check
often surfaces the real constraint.

### No speculative abstraction

Build what the current step needs, not the seam a later step might. An interface with one implementation, a config
knob with one value, a layer that only forwards — each is a guess, and adding it later is cheap once the second
consumer exists. Related smell: parameters a function declares and only passes through.

### Key off stable identifiers, never display strings

Never branch on a label, category name, or human-facing title. Rename the row and the switch falls through to the zero
value silently — usually surfacing as a blanket permission denial or a skipped record with no obvious error. Key off
IDs, and prefer:

1. **seed-controlled IDs** (most robust — fixed, explicit, owned by the seed), or
2. **IDs loaded once at startup, failing fast if missing**.

Hardcoded ID constants are no better than labels if rows can be deleted and reinserted.

### Errors must keep their meaning

- **Don't flatten upstream statuses.** Collapsing every non-2xx from a dependency into a single gateway error turns a
  400 "unrecognised path" into a 502, and sends the next person debugging into the wrong service. Propagate the status
  and the upstream message.
- **Wrap sentinels so they survive.** A sentinel error wrapped with a non-preserving verb stops matching an
  `errors.Is` check further up, and every not-found quietly becomes a 500. Preserve the chain when wrapping (`%w`, not
  `%v`), and map sentinels to status codes in exactly one place per resource.
- **Fail fast on missing config** rather than falling back to a zero value. A config value with no fallback and no
  validation is a runtime mystery; a startup error is a five-second fix.
- **Watch where config actually comes from.** A value hardcoded in a compose/deployment file overrides the env file
  you're reading, and the loader may give it no default at all.

### Every outbound call gets a bound — but not always the same bound

A default HTTP client typically has **no timeout at all**. That's not a goroutine leak, it's minute-long stalls that
look like slow storage, because nothing in your code bounds the wait so you inherit the OS's. Every client needs an
explicit bound. Which bound depends on what's travelling:

- **Small JSON exchanges → one whole-request timeout.** The body is small, so "took too long" is a single fact.
- **Streamed or proxied bodies → never a whole-request timeout.** A client-level timeout covers *reading the body*, so
  it truncates a large pass-through mid-flight. Bound the phases instead: time to first byte, idle-connection reuse,
  and a keepalive ping — the last being the only one that catches a connection dying *while* in use.

Two adjacent traps: adding custom TLS config can silently disable HTTP/2 connection reuse unless you re-enable it, and
tuning the process-global default transport in place reaches every other caller in the binary — clone it.

### Migrations: check whether the old data can exist at all

Before proposing defaulting, coercion, or dual-reading "for existing rows", check whether those rows exist. If the
table is empty, or the environment is cleared before launch, a required field can simply be required — no backfill, no
tolerant reader. Where malformed input means a bug rather than legacy data, a **strict reader that drops the entry** is
the right call.

### Guard, don't assert

Handle the absent or error case explicitly and return early rather than asserting a value is there. Take the value into
a local, check it, bail — the claim is then proved rather than promised. Pick by whether "absent" has a meaningful
answer: a real fallback, or an early return. Never a silent zero value that flows onward into a response.

### Comment the non-obvious only

- **Keep:** a non-obvious ordering, a lock's scope, a constant's units, a rule that looks wrong but isn't, a gotcha
  that will bite ("both upstream calls happen before the transaction opens, so a season is stored whole or not at all").
- **Move to the plan:** why an alternative was rejected, comparison tables, sequencing, history. Rationale duplicated
  into a source header goes stale independently of the document that owns it.
- One line where one line does. A pointer to `backend/plans/NN §x` replaces the paragraph you were about to write.

### Don't optimise blind

Don't solve a projected problem. Record the measured numbers, name the experiment that would confirm the problem is
real (run two concurrent clients — does each still get full throughput, or is there a shared ceiling?), and park it
until there's an actual complaint. Note the constraints that rule options out — a quality-reducing optimisation is off
the table when the consumer needs the original fidelity — so they aren't proposed again.
