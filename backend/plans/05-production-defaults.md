# 05 — Production defaults

Ticket #12. The shipped binary serves the way a deployed service should rather than the way a
development machine does. Scope and motivation are settled in `02-scope-reset.md` § "#12 is back
in"; this file is the engineering.

## What was inherited

`routes.SetupRouter` calls `gin.Default()`. Three of its defaults were never chosen by anyone here:

| default | effect | verdict |
|---|---|---|
| debug mode | `Running in "debug" mode` warning, the whole route table at boot, a log line per request | noise |
| debug mode | the health check polls every 2s, so the noise is continuous, not one-off | noise that hides signal |
| trust all proxies | `ClientIP()` believes any caller's `X-Forwarded-For` | the one with teeth |

`GIN_MODE` appeared nowhere — not the Dockerfile, the compose file, `.env.example`, or the loader.
The only `gin.SetMode` in the repo belonged to the test harness.

The proxy default is live rather than hypothetical: the host publishes this API through Tailscale
Funnel, which proxies to the local service. Nothing reads `ClientIP()` today, which is exactly why
this is cheap now — there is no caller to re-check against the new answer.

## The mode: the loader owns it, release is the default

`config` gains `GinMode` from `GIN_MODE`, defaulting to **release**, and `main` applies it with
`gin.SetMode` before `app.New` builds the router. Route registration reads the mode, so the call has
to precede construction, not follow it.

This is the `DB_SSLMODE` shape, deliberately: the safe value is the default and the development
machine opts out, visibly, in `.env.example` and the local compose file. An operator who deploys
without reading anything gets the production behaviour.

Two rejected alternatives:

- **`ENV GIN_MODE=release` in the Dockerfile.** Smaller — gin's own `init()` reads `GIN_MODE`, so it
  needs no Go at all. Rejected because it puts the decision somewhere no test can reach: deleting
  the line is a silent regression, and the gate never builds the image. It also splits the two
  halves of this ticket, leaving the mode in the image and the proxies in the loader.
- **`gin.SetMode` inside `app.New`.** Tempting for parity with the proxy list below. Rejected
  because the harness sets `gin.TestMode` before building the router, and a composition root that
  overrode it would put every test at the mercy of the ambient `GIN_MODE`.

`GIN_MODE` keeps gin's own name rather than an `APP_ENV` of our own. gin reads the same variable in
its `init()`, so the two agree on every value an operator can set; the only divergence is the empty
case, where gin picks debug and this loader picks release. That divergence is the ticket.

Only `debug` and `release` are accepted from the environment. `test` is a real gin mode but not a
way to serve, and the harness sets it in Go where it belongs — so an unrecognised value is a startup
error naming the two, not a gin panic.

## The proxies: trust nothing, and say so in one place

`config` gains `TrustedProxies` from `TRUSTED_PROXIES`, a comma-separated list of IPs or CIDRs,
empty by default. `app.New` takes the list and calls `engine.SetTrustedProxies`. With `nil`, gin's
`isTrustedProxy` short-circuits: `ClientIP()` returns the peer address, `X-Forwarded-For` is ignored,
and `isUnsafeTrustedProxies` goes false — which is what removes the "You trusted all proxies" warning
from the boot log.

**Why `app.New` and not `main`.** The harness builds its router through the same function, so a
policy applied in `main` would leave tests trusting every proxy while production trusted none. The
first handler to read `ClientIP()` would then be tested against the wrong answer. `app.New` exists
precisely so main and the harness agree.

**Why no validation in the loader.** gin already validates the list, in
`prepareTrustedCIDRs`. A second parser in `config` would be a second definition of "valid CIDR" that
can drift from the one that actually decides. `app.New` returns gin's error, and `main` fails the
boot on it naming `TRUSTED_PROXIES` — the same fail-fast `config.mustEnv` gives a missing credential.

**Why the error travels back rather than exiting in `app.New`.** A `log.Fatalf` there is
`os.Exit(1)` in the one function main and the harness share: a test handing it a bad list would kill
its own binary, taking every later test in the package with it unreported. It would also put this
whole paragraph beyond proof — the argument for skipping loader-side validation only holds while the
failure it defers to is reachable, and test 5 below reaches it.

## `make run` had to change with it

Nothing in the module reads `.env` — `config.LoadConfig` reads the environment — and `make run` was
plain `go run .`. That gap was a documented convenience while gin's own default was debug. Flipping
the loader's default to release made it load bearing: a developer running `make run` would have got
the deployed service's silence on their own machine, which is the opposite of this ticket. `run` now
sources `.env` the way `ingest` already did.

**Why empty rather than the Funnel's address.** Nothing reads `ClientIP()`, so no value here is load
bearing yet; trusting nothing is the answer that cannot be wrong. When a handler needs the real
client address, the ticket that adds it sets `TRUSTED_PROXIES` to the Funnel's address and can
assert the result — rather than inheriting a value nobody chose, which is the whole complaint above.

## What release mode does not fix

The ticket motivates the mode with the health check: "the health check polls every two seconds, and
each poll prints a line". Release mode does not stop that line. The `[GIN]` per-request log comes
from `gin.Default`'s Logger middleware, which writes in every mode — the harness has known this since
it had to discard the writer separately from setting `TestMode`. What release mode removes is the
boot noise: the debug warning, the route table, and the middleware notice.

Left alone rather than fixed here. An access log on a deployed service is worth having, and dropping
or sampling the health route's line is a different decision from "stop inheriting gin's development
defaults". It is worth a ticket once there is a real log to read.

## Where the regression is caught

Six tests in `tests/serving_test.go`, none of which reads a log:

1. **The default is release.** `GIN_MODE` unset. This is the claim the ticket is actually about —
   not that some line calls `SetMode`, but that the value it applies is release when nobody has
   chosen one.
2. **`GIN_MODE=debug` is honoured**, which is the local half of the same criterion.
3. **A spoofed `X-Forwarded-For` is ignored.** A probe route on a real `app.New` engine returns
   `ClientIP()`. No handler exposes it yet, which is why the test registers its own route rather
   than asserting through an endpoint that does not exist.
4. **A configured proxy *is* believed**, on that same route. Without this pair, an `app.New` that
   read the list and dropped it would still pass test 3.
5. **A malformed entry is rejected rather than ignored**, which is what makes "gin validates, so the
   loader does not" a decision rather than an omission.
6. **The harness picks its own mode.** The gin package mode is wound to `DebugMode`, and has to be
   `TestMode` after `NewRouter`. Winding it rather than setting `GIN_MODE` is deliberate: gin reads
   that variable only in its `init()`, so setting it mid-run would assert nothing, and asserting
   against what an earlier test left behind would pass on its own.

There is no test for an unrecognised `GIN_MODE`. That path is `log.Fatalf` in the loader, in the
same idiom as `mustEnv`, and a test cannot survive it — which is the argument for `SetTrustedProxies`
returning its error rather than exiting, above.

The first two tests reload the process-global `config` and restore it afterwards; the package shares
that global with every other test. They reload through `tests.RequireConfig` rather than
`config.LoadConfig`, so a run without the test environment fails one test naming `make test`, rather
than exiting the binary and leaving every later test in the package unreported.
