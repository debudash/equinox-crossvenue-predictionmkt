"""Learning loop, retrieval half: past reviewed cards become few-shot examples in the extraction prompt.

Retrieval is plain word overlap (Jaccard) on rules text, preferring the same venue. Explainable on a
slide, no embeddings needed at this size. Learning only changes what the LLM proposes; the human gate
and the deterministic deviation check stay exactly as they are.
"""

import re

from .core_db import CoreCard

_WORD = re.compile(r"[a-z0-9.%]+")


def _words(text: str) -> set[str]:
    return set(_WORD.findall(text.lower()))


def pick_examples(target: CoreCard, reviewed: list[tuple[CoreCard, dict]], k: int = 3) -> list[tuple[CoreCard, dict]]:
    """reviewed: (card, gold fields) pairs. Never returns the target card itself (no leakage into evals)."""
    tw = _words(target.title + " " + target.rules_text)

    def score(item):
        card, _ = item
        w = _words(card.title + " " + card.rules_text)
        jac = len(tw & w) / len(tw | w) if tw | w else 0.0
        return (card.venue == target.venue, jac)

    pool = [r for r in reviewed if r[0].market_id != target.market_id]
    return sorted(pool, key=score, reverse=True)[:k]
