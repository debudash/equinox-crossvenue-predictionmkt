"""Learning loop, measuring half: reviewed decisions are the eval set.

- field accuracy: share of reviewed fields the reviewer marked correct (how good the stored extractions were)
- proposal precision: share of decided proposals the reviewer approved, per blocking pattern
- rerun: re-extract every reviewed card with the current prompt and examples (never its own review)
  and score against the gold values. This is the number a prompt change must not lower: the eval gate.
"""

from collections import defaultdict

from . import extract, store
from .learn import pick_examples


def review_metrics(con) -> dict:
    per_field = defaultdict(lambda: [0, 0])
    for r in con.execute("SELECT field, correct FROM field_reviews"):
        per_field[r["field"]][0] += r["correct"]
        per_field[r["field"]][1] += 1
    per_pattern = defaultdict(lambda: [0, 0])
    for d in store.latest_decisions(con).values():
        per_pattern[d["pattern"]][0] += d["verdict"] != "reject"
        per_pattern[d["pattern"]][1] += 1
    ok, n = sum(v[0] for v in per_field.values()), sum(v[1] for v in per_field.values())
    app, dec = sum(v[0] for v in per_pattern.values()), sum(v[1] for v in per_pattern.values())
    return {"field_accuracy": (ok, n), "per_field": dict(per_field),
            "proposal_precision": (app, dec), "per_pattern": dict(per_pattern)}


def rerun(con, reviewed: list, llm) -> dict:
    """reviewed: (card, gold) pairs from pipeline.reviewed_examples. Nothing is saved."""
    hits, total, misses = 0, 0, []
    for card, gold in reviewed:
        examples = pick_examples(card, reviewed)          # excludes the card itself
        fields, _, _, _ = extract.check(card, llm(extract.SYSTEM, extract.build_prompt(card, examples)))
        for f, want in gold.items():
            total += 1
            if fields.get(f) == want:
                hits += 1
            else:
                misses.append((card.ref, f, fields.get(f), want))
    return {"accuracy": (hits, total), "misses": misses}


def pct(pair) -> str:
    ok, n = pair
    return f"{ok}/{n} ({100 * ok // n}%)" if n else "n/a (nothing reviewed yet)"
