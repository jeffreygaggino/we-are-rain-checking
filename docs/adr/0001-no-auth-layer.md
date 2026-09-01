# No auth layer

`STRUCTURE-go-gin-backend.md` mandates a permission check on every handler and explicitly forbids adding a bypass for local development. This service has no users, serves public F1 data, and exposes nothing that a caller could mutate — so applying that rule here would mean inventing a role vocabulary with no roles in it, which the same working agreement forbids under *No speculative abstraction*.

We resolved the contradiction by removing the layer rather than faking it: there is no `permissions/` package, and handlers begin at the parameter-parsing step rather than at a permission check.

## Consequences

The one operation worth protecting is triggering ingest. That is why ingest is a `cmd/` binary run on a schedule rather than an HTTP endpoint — the decision to drop auth and the decision to keep ingest off the API are the same decision, and reversing either means revisiting both.

Adding auth later is a genuine retrofit: every handler grows a step and the seven-step body from `STRUCTURE` returns intact. That cost is accepted deliberately, on the grounds that a permission system with one permission is harder to reason about than none.
