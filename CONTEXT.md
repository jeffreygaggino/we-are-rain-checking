# we-are-rain-checking

Two subjects share this repo. The **F1 weather service** is the substrate; the **skills experiment** is the object of study. The language below covers both, because both are domains here.

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

## The experiment

**Drift**:
Divergence between a copy of an agent instruction and the source it was copied from, occurring silently and changing agent behaviour.
_Avoid_: Staleness, version skew

**Ablation**:
Running the same task twice — once with a skill available, once without — to attribute a difference in outcome to that skill.
_Avoid_: A/B test, comparison

**Ablation Delta**:
The scored difference between the two Ablation arms. The only evidence that a skill does anything.
_Avoid_: Improvement, lift, gain

**Treatment Arm** / **Baseline Arm**:
The run with the skill available, and the run without it.
_Avoid_: Control (ambiguous about which side has the skill), experimental group

**Fixture Repo**:
A workspace built for an Ablation, whose contents are controlled per arm so the Baseline Arm cannot read what the skill is supposed to supply.
_Avoid_: Test repo, sandbox, scratch repo
