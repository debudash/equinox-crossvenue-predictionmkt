# Near-miss proposal prototype

A team-facing proposal, not a product change. It sits outside the Equinox core: it opens
`data/equinox.db` read-only, writes only `data/proposals.db`, and never imports from or edits
`internal/`, `cmd/` or `config/`. The core stays deterministic with no language model (AGENTS.md, ADR-006).

**LLM proposes, rules measure, human decides, router executes deterministically.**

## Why

The core checked 10,054 Kalshi x Polymarket Fed pairs and matched 0. True equivalents exist, but
the venues write them differently: Kalshi "above 3.75%" (gt 3.75) is Polymarket "4.00% or above"
(gte 4.00) on the 25 bp rate grid, and neither venue's rules text states the unclear-result rule.
Literal field counting ranks the wrong pairs closest (gt 4.00 vs gte 4.00).

## The four steps

| Step | File | What it does |
|---|---|---|
| 1. LLM proposes | `proto/extract.py` | Reads each card's rules text and fills the seven payoff fields as a *shadow card*. Every value needs a verbatim quote; code drops any value whose quote is not in the text or that falls outside `config/vocab.yaml`, and keeps the reason. Cached per card version. |
| 2. Rules measure | `proto/compare.py` | Deterministic deviation check on every reference x counter pair. Each difference is a named *blocking pattern*. Hard patterns (different threshold even on the grid, deadlines days apart) drop the pair; soft ones are counted: 0 strong, 1-2 near miss, 3+ dropped. Only pairs with 7+ days to the earlier close. |
| 3. Human decides | `app.py` | Review screen grouped by blocking pattern, with a category filter. Per-field correct/incorrect plus a corrected value, a verdict (same, opposite, reject), and a required reason. Can apply one verdict to a whole pattern group. A decision goes stale when either card changes. |
| 4. Router executes | `proto/route_sim.py` | Line-for-line port of `internal/route.Decide` for approved, non-stale pairs. as_of is the later of the two saved prices. Every route is logged with all inputs. |

Learning loop: `proto/learn.py` retrieves the closest reviewed cards as few-shot examples for the
next extraction; `proto/evals.py` turns reviews into the eval set (field accuracy, proposal
precision, and a rerun score that a prompt change must not lower). Learning never removes the human gate.

## Run

```
cd prototype
pip install -r requirements.txt
copy .env.example .env               # then set ANTHROPIC_API_KEY
python -m proto extract --core-copy  # baseline without the LLM: shadow = the fetcher's fields
python -m proto propose              # the funnel and the proposals
python -m proto extract --refresh    # LLM shadow cards (about 480 cards; --limit N to try a few)
streamlit run app.py                 # review, route, learning tabs
python -m proto route --side yes
python -m proto eval [--rerun]
python -m pytest -q
```

Model: `claude-sonnet-5` by default (`EQX_PROTO_MODEL` to change). Structured output via
`messages.parse`, cached system prompt, server-side refusal fallback. LangSmith tracing turns on
with `LANGSMITH_TRACING=true`.

## What the baseline already shows (2026-10-10 data)

`extract --core-copy` then `propose`: 10,054 pairs checked, 10,045 dropped, 9 near misses, and the 9 are
exactly the true equivalents `KXFEDFUNDSYEAR-28JAN01-T<X>` = `rvcus-...-gte<X+0.25>`, each with the
pattern `grid_boundary + unclear_result_unknown`. The gt 4.00 vs gte 4.00 trap is dropped as `cutoff_differs`.
The LLM's job is the rest: reading fields the fetchers leave empty (unclear_result, the FOMC
meeting-decision markets) and proposing values a reviewer can confirm against the quote.

## Venue policies (grounding and versioning)

`policies/sources.yaml` lists the published documents (Kalshi contract terms per series, linked from the
Kalshi series API; the Polymarket US rulebook filed with the CFTC). `policies/clauses.yaml` codifies the
sentences that bear on a payoff field, copied from the source. `python -m proto policies refresh`
snapshots every document, checks every clause is still present, and issues a new version
(`YYYY-MM-DD.n`) when anything changed. It runs daily at 00:15 through Windows Task Scheduler
(`scripts/refresh_policies.cmd`, task "Equinox prototype policy refresh"; remove it with
`Unregister-ScheduledTask -TaskName "Equinox prototype policy refresh"`).

The LLM gets the clauses that apply to a card's venue and series, may quote them only where the card's
own rules are silent, and code checks the quote against the clause. Every LLM reading, decision and
route records the policy version it relied on; the Policies tab shows the check history, versions,
diffs and clause status.

## Tests

`tests/test_route_parity.py` ports the core's required router tests 08-14 and replays every route
record the core wrote to `data/demo.db`, matching action, sides, fee flag and reason text exactly.
The other tests cover the quote gate, the deviation patterns and thresholds, the 7-day window,
append-only decisions, staleness, and gold values for the eval set. The LLM is stubbed in tests.

## Open question for the team

When a venue keeps discretion over an undeterminable outcome (Kalshi KXFEDFUNDSYEAR states no
unclear-result rule), is that `unclear_result_unknown` forever, or does a reviewer's reading of the
venue's general terms count as gold?
