# Repo-owned driver identity

OpenF1 exposes no stable identifier for a driver. The `drivers` endpoint returns twelve fields, and the only candidates for an identity — `driver_number`, `full_name`, `name_acronym` — are respectively a per-season assignment and two display strings.

`driver_number` is actively dangerous. Number `1` belongs to the reigning champion rather than to a person, so it resolves to **Max Verstappen for 2023–2025 and Lando Norris for 2026**; `3` resolves to both Daniel Ricciardo and Max Verstappen across the same window. Keying results on it silently merges two people into one.

We therefore seed a `drivers` table with an id this repo owns, resolve `(session, racing number) → driver` at ingest via the upstream name, and fail fast on a name we have not seeded rather than inserting a new driver.

## Consequences

**Seed ids must be literal, not generated.** A seed migration calling `gen_random_uuid()` gives local, CI and the deployed host different ids for the same person, and any fixture or assertion that names a driver breaks the moment it crosses an environment. The ids are written into the migration as constants.

This mirrors the existing decision to seed circuit coordinates instead of geocoding them, for the same reasons: deterministic, testable, and one fewer network dependency.

## Why it is worth recording

The failure mode is not hypothetical. An early analysis of this data keyed winners on `driver_number` and produced an apparently real result — a ten-point wet-weather advantage for driver `1`. Resolving identity properly collapsed it to **one percentage point**: the number had merged two different drivers' wins. The bug did not announce itself as a bug; it announced itself as a finding.
