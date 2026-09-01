# Skill drift exhibit

Captured 2026-09-01, immediately before the stale copies in `~/.claude/skills/` were deleted.

Four skills had been copy-pasted into a global skills directory at some earlier point. When the packaged versions were installed from a marketplace, **all four had diverged from their sources.**

| skill | local | packaged | diff lines |
|---|---|---|---|
| `grilling` | 12 | 28 | 32 |
| `domain-modeling` | 74 | 74 | 36 |
| `prototype` | 26 | 26 | 24 |
| `grill-with-docs` | 7 | 7 | 4 |

`grilling.diff` is the one that matters. The stale copy instructed one-question-at-a-time questioning; the packaged version specifies round-based *frontier* questioning, where every decision whose prerequisites are settled gets asked together. **The divergence changed agent behaviour observably, and silently.**

This is the motivating evidence for the repo. If copy-paste distribution of agent instructions fails at n=4 on a single laptop, it isn't a distribution strategy — versioned distribution is. And if the instructions can drift without anyone noticing, then whether they *do anything measurable* is worth answering directly, which is what `evals/` is for.
