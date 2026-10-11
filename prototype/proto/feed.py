"""Card view helpers: quotes per market and a source flag per payoff field. Display only.

The core saves one buy price per side (the ask). A bid is not saved, so the yes bid shown is the
implied one, 1 - no ask, and the same for the no bid. Exact micro-dollar arithmetic, no floats.
"""

from dataclasses import dataclass

from . import values
from .core_db import Price


@dataclass
class Quote:
    yes_ask: str | None
    no_ask: str | None
    yes_bid: str | None      # implied: 1 - no ask
    no_bid: str | None       # implied: 1 - yes ask
    priced_at: str | None


def _implied(ask: str | None) -> str | None:
    m = values.micros(ask) if ask else None
    return values.micros_str(1_000_000 - m) if m is not None else None


def quote(yes: Price | None, no: Price | None) -> Quote:
    ya, na = yes.buy_price if yes else None, no.buy_price if no else None
    stamps = [p.priced_at for p in (yes, no) if p]
    return Quote(ya, na, _implied(na), _implied(ya), max(stamps) if stamps else None)


def latest_prices(core) -> dict[tuple[str, str], Price]:
    """(venue:market_id, side) -> latest saved price, in one query."""
    rows = core.execute(
        "SELECT p.venue, p.market_id, p.side, p.buy_price, p.fee, p.priced_at FROM prices p "
        "JOIN (SELECT venue, market_id, side, MAX(id) id FROM prices GROUP BY venue, market_id, side) l ON l.id = p.id")
    return {(f"{r[0]}:{r[1]}", r[2]): Price(*r) for r in rows}


_OP_WORDS = {"gt": "above", "gte": "at or above", "lt": "below", "lte": "at or below"}


def _pct(v: str) -> str:
    whole, _, frac = v.partition(".")
    return f"{whole}.{(frac + '00')[:max(2, len(frac))]}%"


def label(card, fields: dict | None = None) -> str:
    """Readable name: the event plus what this market pays on, e.g.
    'Fed funds rate after Dec 2026 meeting · above 3.00%'. Built from the card, never from the LLM alone."""
    f = fields or card.core_fields
    event = card.event_title.rstrip("?").strip()
    if f.get("cutoff_op") and f.get("cutoff_value"):
        v = _pct(f["cutoff_value"]) if f.get("cutoff_unit") == "percent" else f["cutoff_value"]
        return f"{event} · {_OP_WORDS[f['cutoff_op']]} {v}"
    return f"{event} · {card.title}"


# Flag per field: what the deviation check sees and where it came from.
AGREE, FETCHER_ONLY, LLM_FILLED, DISAGREE, EMPTY = "agree", "fetcher only", "LLM filled", "disagree", "empty"
POLICY_FILLED = "LLM filled from venue policy"


def field_flag(shadow: dict, field: str) -> str:
    core_v, llm_v = shadow["core_fields"][field], shadow["llm_fields"].get(field)
    if field in shadow["disagree"]:
        return DISAGREE
    if core_v is not None and llm_v is not None:
        return AGREE
    if core_v is not None:
        return FETCHER_ONLY
    if llm_v is None:
        return EMPTY
    return POLICY_FILLED if str(shadow.get("provenance", {}).get(field, "")).startswith("policy:") else LLM_FILLED
