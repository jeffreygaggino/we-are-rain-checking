# we-are-rain-checking

## Commits and pull requests

Do not add agent attribution. No `Co-Authored-By: Claude` trailer, no `Claude-Session:` trailer, no "Generated with Claude Code" line in a pull request body. A commit message ends with its own last line, and so does a PR description.

## Backend

The working agreement for the backend service — how we work together, and the authority on code. See
`docs/agents/backend.md`. Rationale, rejected alternatives and sequencing live in `backend/plans/`.

## Agent skills

### Issue tracker

Issues live as GitHub issues in `jeffreygaggino/we-are-rain-checking`, driven by the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles, each label string equal to its name. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
