# Evals run against a fixture repo, not this one

The headline prediction of this repo's experiment is that `add-f1-endpoint` shows the largest ablation delta, because it encodes conventions the model has no other way to know. That prediction is only testable if the Baseline Arm genuinely cannot know them.

Running the eval inside this repository defeats it. The conventions live here as documentation — that is the point of documenting them — so an agent running here reads them whether or not the skill is loaded. The delta collapses toward zero and the honest conclusion, *"the control group was given the answer"*, is indistinguishable from the tempting one, *"the skill does nothing"*.

Evals therefore run against a fixture repo whose contents the harness controls per arm: convention documentation present for the Treatment Arm, absent for the Baseline Arm.

## Consequences

The same confounder applies to `tdd`, which is predicted to show near-zero delta. If a fixture carries an instruction to write tests first, the Baseline Arm does TDD without the skill and the near-zero result is manufactured rather than observed. Fixtures must be built so that each skill under test is the only source of its own instruction.

This costs realism: the fixture is not the real repo, so a delta measured there is evidence about the skill, not about working in this codebase. That trade is deliberate — attribution is the entire point, and a number nobody can attribute is worth less than a smaller number that means something.
