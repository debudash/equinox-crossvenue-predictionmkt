# Evals

The matcher has no score to tune, so evaluation is an audit, not a precision curve.

## Match audit (evals/match-audit.md)
After a real session (T-30):
1. Every `same` and `opposite` pair: a person reads both venues' full rules_text and confirms YES pays in exactly the same cases (or exactly the opposite cases). Target: zero false matches.
2. A sample of at least 10 `not_same` pairs within the Fed subject: confirm the reason names the field that actually differs.
3. Record any disagreement as a fetcher bug, with the fixture that reproduces it.

## Routing check
For each route record in the session, recompute the decision by hand from the logged prices, fees and as_of, and confirm the action and reason.
