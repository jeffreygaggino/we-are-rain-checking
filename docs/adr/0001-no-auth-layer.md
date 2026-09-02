# No auth layer

*`STRUCTURE-go-gin-backend.md` was the generic kickoff prompt this service was scaffolded from. It was removed on 2026-09-03 once the skeleton stood; the conventions that replaced it live in `docs/agents/backend.md` and `backend/plans/01-backend-v1.md`. It is named below as the thing this decision argued against.*

`STRUCTURE-go-gin-backend.md` mandates a permission check on every handler and explicitly forbids adding a bypass for local development. This service has no users, serves public F1 data, and exposes nothing that a caller could mutate — so applying that rule here would mean inventing a role vocabulary with no roles in it, which the same working agreement forbids under *No speculative abstraction*.

We resolved the contradiction by removing the layer rather than faking it: there is no `permissions/` package, and handlers begin at the parameter-parsing step rather than at a permission check.

## Consequences

The one operation worth protecting is triggering ingest. That is why ingest is a `cmd/` binary run on a schedule rather than an HTTP endpoint — the decision to drop auth and the decision to keep ingest off the API are the same decision, and reversing either means revisiting both.

Adding auth later is a genuine retrofit: every handler grows a step and the seven-step body from `STRUCTURE` returns intact. That cost is accepted deliberately, on the grounds that a permission system with one permission is harder to reason about than none.

**The service is publicly reachable, and this decision still holds** (recorded 2026-09-03, when deployment was designed). Tailscale Funnel publishes the API at a `*.ts.net` hostname, so anyone holding the URL can call it. That does not reopen the question: every route is a `GET` over public F1 data, and ingest stays off the API by the same decision recorded above, so there is still nothing a caller could mutate and no one to distinguish from anyone else. What a public URL genuinely adds is unbounded read traffic against a self-hosted box — a capacity concern, not an authorisation one, and it must not be answered by inventing the permission layer this ADR removed.
