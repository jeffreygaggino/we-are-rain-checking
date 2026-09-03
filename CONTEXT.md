# we-are-rain-checking

The language of the F1 weather service. Terms defined here are the ones to use in issue titles, test names, commit messages and code — not their listed synonyms.

## Race weekend

**Meeting**:
A Grand Prix weekend at one circuit, spanning several sessions.
_Avoid_: Race weekend, event, round

**Session**:
One timed running of cars within a Meeting — practice, qualifying, or the race itself.
_Avoid_: Run, outing

**Race**:
The Session that awards championship points. The unit of analysis for every question this service answers.
_Avoid_: Grand Prix (that's the Meeting), main event

**Circuit**:
The physical venue a Meeting is held at. Its coordinates are owned by this repo, not looked up.
_Avoid_: Track (means the racing surface), venue, location

**Retirement**:
A driver failing to finish a Race they started.
_Avoid_: DNF as prose (fine as a field name), crash, failure

## Weather

**Weather Sample**:
One timestamped observation at the circuit during a Session, carrying temperature, wind, humidity, pressure, and rainfall.
_Avoid_: Reading, datapoint, measurement

**Rainfall**:
A **binary presence flag** on a Weather Sample — it rained, or it did not. This source carries no intensity, so drizzle and a downpour are indistinguishable.
_Avoid_: Precipitation, rain level, mm (all imply a magnitude that does not exist)

**Wet Fraction**:
The proportion of a Session's Weather Samples recording Rainfall.
_Avoid_: Rain percentage, wetness

**Wet Session**:
A Session whose Wet Fraction exceeds the documented threshold. Presence of any rain at all is *not* sufficient.
_Avoid_: Rainy race, damp session

**Forecast**:
Predicted weather for a Race that has not happened yet. Comes from a different upstream than a Weather Sample and is never stored as one.
_Avoid_: Prediction (reserved for the service's claims about outcomes)

## Identity

**Driver**:
A person who competes. Identified by an id this repo owns and seeds, because no upstream identifier is stable.
_Avoid_: Competitor, entrant

**Racing Number**:
The number a Driver carries **for one season**. It is not an identity: it is reassigned between seasons, and `1` belongs to the reigning champion rather than to a person.
_Avoid_: Driver number *as an identifier*, driver id

## Claims

**Insufficient Sample**:
The state where a question has too few observations to be answered. A first-class result the service returns, never an error or an omission.
_Avoid_: No data, empty result, null

**Signal**:
A relationship between weather and race outcome that survives its own sample size. Absence of Signal is a finding, not a failure.
_Avoid_: Correlation (implies a specific statistic), trend, effect

## Measurement

**Driver-Race**:
One Driver's participation in one Race — the unit of analysis. A Race contributes one observation about its winner but roughly twenty about Retirements, which is what makes the Retirement question answerable where the winner question is not.
_Avoid_: Entry, start, result row

**Axis**:
One weather property a claim is made about, reported on its own — Rainfall, wind band, track temperature band. Axes are never combined into a single measure of bad weather: they act through different mechanisms, and merging them makes an effect unattributable.
_Avoid_: Factor, variable, inclement weather

**Season Range**:
Every season the upstream carries — 2023 to the current year, derived from the clock rather than configured. It is the population every claim is made over, and it names the seasons rather than the rows: say "every Race in these seasons" or "the four seasons" instead of reaching for one collective noun.
_Avoid_: **Corpus** (see below), dataset, sample (means Weather Sample or Insufficient Sample here)

### Never "corpus"

The word reached 76 lines before being removed on 2026-09-03, and it was naming four different things: the Races every claim is made over, the seeded reference data, whatever OpenF1 holds, and whatever is currently stored. Name the one you mean — "every Race in these seasons", "the seed", "upstream", "seasons already stored".

Two of those senses are a real distinction rather than a stylistic one: 71 driver names exist upstream and 29 appear in a Race, and only the second set is analysed. Collapsing them under one convenient label is the prose form of what ADR-0003 records happening to `driver_number` — two things merged into one identifier, producing something that reads like a finding.

It survives in closed issues, which are left as the record of what was argued at the time.
