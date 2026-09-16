---
description: Rough idea → zero-ambiguity PRD → grabbable issues. terrac's front-of-project flow.
---

# /spec

Drive a new piece of work from a rough idea to tracked issues. Gate at each phase —
terrac writes PRDs with zero ambiguity (goals AND explicit anti-goals) before any code.

1. **Grill.** Invoke the `grill-me` skill. Interrogate the idea until every branch of
   the decision tree is resolved. Force out the anti-goals, not just the goals. Do not
   move on while ambiguity remains.

2. **PRD.** Invoke `to-spec` to turn the grilled context into a PRD: goals, explicit
   anti-goals, scope, acceptance criteria. **Checkpoint** — show terrac the PRD and get
   sign-off before creating anything.

3. **Issues.** Invoke `to-tickets` to break the approved PRD into tracer-bullet,
   vertical-slice issues on the current repo's tracker.

Rules: stop between phases (each gate is terrac's to approve); implement later only
against the approved PRD scope; a new repo should inherit the CI/CD deploy pattern.
