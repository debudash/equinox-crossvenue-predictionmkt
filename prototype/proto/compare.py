"""Step 2 - rules measure. Deterministic deviation check on shadow-card pairs. No LLM here.

Each card's fields are the fetcher's values with LLM values filling the gaps (pipeline.merge).
Each difference between two cards is named as a blocking pattern:
- hard patterns mean a different bet (different subject, a different threshold even on the rate grid,
  a deadline more than a day apart). Any hard pattern drops the pair.
- soft patterns are what a human can rule on (a field the text does not state, the same threshold written
  as gt X versus gte X+step, deadlines hours apart).
Soft count: 0 = strong, 1-2 = near miss for human review, 3+ = dropped.
Only pairs with at least 7 days to the earlier close are proposed.
"""

from collections import Counter
from dataclasses import dataclass, field
from datetime import datetime, timedelta

from . import config, values

EXPLAIN = {
    "grid_boundary": "Same threshold written two ways (gt X vs gte X+0.25). Equal only if the rate moves in 25 bp steps.",
    "deadline_close": "Deadlines differ by under a day, often a time zone or close-time convention.",
    "subject_unknown": "One card's text does not state the subject.",
    "cutoff_unknown": "One card's text does not state a level threshold (e.g. a meeting-decision market).",
    "deadline_unknown": "One card's text does not state when the outcome is measured.",
    "decider_unknown": "One card's text does not state who publishes the settling figure.",
    "unclear_result_unknown": "One card's text does not say what happens if the outcome cannot be determined.",
    "unclear_result_differs": "The cards settle an undeterminable outcome differently.",
    "both_venue_discretion": "Both venues' published terms let the venue decide an undeterminable outcome. "
                             "Equal words, not proven equal payoffs.",
    "fetcher_llm_disagree": "The fetcher and the LLM read a field differently. The fetcher's value is used; check the quote.",
    "op_mismatch_same_value":"Same number, different comparison, no rate grid to reconcile them.",
}


@dataclass
class Proposal:
    a: dict                      # shadow card on the reference venue
    b: dict                      # shadow card on the counter venue
    relation: str | None         # same | opposite | None when the cutoff cannot say
    soft: list[str] = field(default_factory=list)
    hard: list[str] = field(default_factory=list)
    days_to_close: int | None = None

    @property
    def tier(self) -> str:
        if self.hard or len(self.soft) > config.NEAR_MISS_MAX:
            return "dropped"
        return "strong" if not self.soft else "near_miss"

    @property
    def pattern(self) -> str:
        return " + ".join(sorted(self.soft)) or "none"

    @property
    def key(self) -> tuple[str, str]:
        return self.a["ref"], self.b["ref"]


_COMPLEMENT = {"gt": "lte", "lte": "gt", "gte": "lt", "lt": "gte"}


def _on_grid(op: str, value: str, step: str) -> tuple[str, str]:
    """Rewrite strict/inclusive bounds onto one form: on a grid, gt X means gte X+step, lte X means lt X+step."""
    if op == "gt":
        return "gte", values.add_decimal(value, step)
    if op == "lte":
        return "lt", values.add_decimal(value, step)
    return op, value


def _cutoff(fa: dict, fb: dict, p: Proposal) -> None:
    keys = ("cutoff_op", "cutoff_value", "cutoff_unit")
    if any(fa[k] is None or fb[k] is None for k in keys):
        p.soft.append("cutoff_unknown")
        return
    if fa["cutoff_unit"] != fb["cutoff_unit"]:
        p.hard.append("cutoff_unit_differs")
        return
    oa, va, ob, vb = fa["cutoff_op"], fa["cutoff_value"], fb["cutoff_op"], fb["cutoff_value"]
    if va == vb and oa == ob:
        p.relation = "same"
        return
    if va == vb and _COMPLEMENT[oa] == ob:
        p.relation = "opposite"
        return
    step = config.CUTOFF_STEP.get(fa["subject"]) if fa["subject"] == fb["subject"] else None
    if step is None:
        (p.soft if va == vb else p.hard).append("op_mismatch_same_value" if va == vb else "cutoff_differs")
        return
    na, nb = _on_grid(oa, va, step), _on_grid(ob, vb, step)
    if na == nb:
        p.relation = "same"
        p.soft.append("grid_boundary")
    elif na[1] == nb[1] and _COMPLEMENT[na[0]] == nb[0]:
        p.relation = "opposite"
        p.soft.append("grid_boundary")
    else:
        p.hard.append("cutoff_differs")


def compare(a: dict, b: dict) -> Proposal:
    fa, fb = a["fields"], b["fields"]
    p = Proposal(a=a, b=b, relation=None)
    for f in ("subject", "decider"):
        if fa[f] is None or fb[f] is None:
            p.soft.append(f"{f}_unknown")
        elif fa[f] != fb[f]:
            p.hard.append(f"{f}_differs")
    _cutoff(fa, fb, p)
    if fa["deadline"] is None or fb["deadline"] is None:
        p.soft.append("deadline_unknown")
    elif fa["deadline"] != fb["deadline"]:
        gap = abs(values.parse_utc(fa["deadline"]) - values.parse_utc(fb["deadline"]))
        (p.soft if gap <= timedelta(days=1) else p.hard).append(
            "deadline_close" if gap <= timedelta(days=1) else "deadline_differs")
    if fa["unclear_result"] is None or fb["unclear_result"] is None:
        p.soft.append("unclear_result_unknown")
    elif fa["unclear_result"] == fb["unclear_result"] == "venue_discretion":
        p.soft.append("both_venue_discretion")
    elif fa["unclear_result"] != fb["unclear_result"]:
        p.soft.append("unclear_result_differs")
    if a.get("disagree") or b.get("disagree"):
        p.soft.append("fetcher_llm_disagree")
    return p


def _close(shadow: dict, core_deadline: str | None) -> datetime | None:
    d = shadow["fields"]["deadline"] or core_deadline
    return values.parse_utc(d) if d else None


def propose(shadows: list[dict], core_deadlines: dict[str, str | None], as_of: datetime):
    """All reference x counter pairs. Returns (proposals to review, counts for the funnel)."""
    ref = [s for s in shadows if s["ref"].startswith(config.REFERENCE_VENUE + ":")]
    ctr = [s for s in shadows if s["ref"].startswith(config.COUNTER_VENUE + ":")]
    counts, out = Counter(), []
    for a in ref:
        for b in ctr:
            counts["pairs checked"] += 1
            closes = [c for c in (_close(a, core_deadlines.get(a["ref"])), _close(b, core_deadlines.get(b["ref"]))) if c]
            if not closes or min(closes) - as_of < timedelta(days=config.MIN_DAYS_TO_CLOSE):
                counts["closes within 7 days or unknown"] += 1
                continue
            p = compare(a, b)
            p.days_to_close = (min(closes) - as_of).days
            counts[p.tier] += 1
            if p.tier != "dropped":
                out.append(p)
    out.sort(key=lambda p: (p.tier != "strong", p.pattern, p.key))
    return out, counts
