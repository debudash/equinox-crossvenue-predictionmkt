"""Step 1 - LLM proposes. Extract the seven payoff fields from a card's rules text into a shadow card.

The LLM must back every value with a verbatim quote. Code, not the LLM, then decides what survives:
a value whose quote is not found word for word in the card text, or that falls outside the shared
vocabulary, is emptied and the reason is kept. Shadow cards are cached per card version.
"""

import json
import re
from typing import Callable, Optional

from pydantic import BaseModel, Field

from . import config, policy, store, values
from .core_db import CoreCard
from .learn import pick_examples


class FieldValue(BaseModel):
    value: Optional[str] = Field(description="Normalized value, or null when the text does not state it.")
    quote: Optional[str] = Field(description="Exact words copied from the card text that state the value.")


class Extraction(BaseModel):
    subject: FieldValue
    cutoff_op: FieldValue
    cutoff_value: FieldValue
    cutoff_unit: FieldValue
    deadline: FieldValue
    decider: FieldValue
    unclear_result: FieldValue


SYSTEM = f"""You read the resolution rules of one prediction market and fill a seven-field payoff card.
Two cards with equal fields pay out in exactly the same worlds, so precision matters more than coverage.

Fields and allowed values:
- subject: one of {sorted(config.SUBJECTS)}. Use fed_funds_upper_bound only when the market resolves on the
  level of the upper bound of the federal funds target range.
- cutoff_op: gt (above, greater than), gte (at or above, greater than or equal to), lt, lte.
- cutoff_value: the threshold as a plain decimal, e.g. 3.75 (no % sign).
- cutoff_unit: one of {sorted(config.CUTOFF_UNITS)}.
- deadline: the instant the outcome is measured, as RFC 3339 with offset. Convert ET using the US daylight
  saving rules in force on that date (11:59 PM ET on December 31 is 04:59 UTC on January 1).
- decider: one of {sorted(config.DECIDERS)}; the body whose published figure settles the market.
- unclear_result: one of {sorted(config.UNCLEAR_RESULTS)}; what happens if the outcome cannot be determined.
  resolves_no: the terms say the market resolves No. venue_discretion: the venue decides the payout.

Rules:
- Every non-null value needs a quote copied character for character from the card text, or from a
  <venue_policy> clause when the card's own rules are silent on that field. No paraphrase.
- If the text does not state a field, return value null and quote null. Never guess, never use outside knowledge.
- A market that resolves on a change at a meeting (a cut, a hike, no change) is not a level; leave
  cutoff_op, cutoff_value and cutoff_unit null for it.
"""


def _card_text(card: CoreCard) -> str:
    return f"Event: {card.event_title}\nTitle: {card.title}\nRules:\n{card.rules_text}"


def build_prompt(card: CoreCard, examples: list[tuple[CoreCard, dict]]) -> str:
    parts = []
    for ex, gold in examples:
        shown = {f: gold.get(f) for f in config.FIELDS if f in gold}
        parts.append(f"<reviewed_example>\n{_card_text(ex)}\nReviewer-confirmed values (fields not listed were not "
                     f"reviewed): {json.dumps(shown)}\n</reviewed_example>")
    intro = ("These cards were checked by a human reviewer. Follow how they read the rules.\n\n" + "\n\n".join(parts)
             + "\n\n") if parts else ""
    return f"{intro}{policy.prompt_block(card.ref)}<card>\n{_card_text(card)}\n</card>\n\nFill the payoff card for <card>."


_WS = re.compile(r"\s+")


def _squash(s: str) -> str:
    return _WS.sub(" ", s).strip()


def _valid(field: str, value: str) -> str | None:
    """Normalized value, or None when it is outside the shared vocabulary."""
    match field:
        case "subject":
            return value if value in config.SUBJECTS else None
        case "decider":
            return value if value in config.DECIDERS else None
        case "cutoff_unit":
            return value if value in config.CUTOFF_UNITS else None
        case "cutoff_op":
            return value if value in config.OPS else None
        case "unclear_result":
            return value if value in config.UNCLEAR_RESULTS else None
        case "cutoff_value":
            return values.norm_decimal(value)
        case "deadline":
            return values.norm_instant(value)


def check(card: CoreCard, raw: Extraction) -> tuple[dict, dict, dict, dict]:
    """The code gate on LLM output. Returns (fields, quotes, problems, provenance).
    provenance per field: "rules" (the card's own text) or "policy:<clause id>" (a codified venue clause)."""
    haystack = _squash(_card_text(card))
    fields, quotes, problems, provenance = {}, {}, {}, {}
    for f in config.FIELDS:
        fv: FieldValue = getattr(raw, f)
        fields[f], quotes[f] = None, fv.quote
        if fv.value is None:
            continue
        if fv.quote and _squash(fv.quote) in haystack:
            where = "rules"
        elif fv.quote and (cid := policy.quote_source(card.ref, fv.quote)):
            where = f"policy:{cid}"
        else:
            problems[f] = f"quote not found verbatim in the card text or its venue policy (value was {fv.value!r})"
            continue
        v = _valid(f, fv.value.strip())
        if v is None:
            problems[f] = f"value {fv.value!r} is outside the shared vocabulary"
            continue
        fields[f], provenance[f] = v, where
    return fields, quotes, problems, provenance


# ---- the LLM call ------------------------------------------------------------------------------

LLM = Callable[[str, str], Extraction]   # (system, user) -> Extraction. Tests pass a stub.


def anthropic_llm() -> LLM:
    import anthropic

    client = anthropic.Anthropic()
    try:  # optional LangSmith tracing, only when the user turned it on
        import os
        if os.getenv("LANGSMITH_TRACING", "").lower() == "true":
            from langsmith.wrappers import wrap_anthropic
            client = wrap_anthropic(client)
    except ImportError:
        pass

    # Server-side refusal fallback, routed by refusal category, only on models that accept it.
    fallback = {}
    if config.MODEL in ("claude-sonnet-5-5", "claude-opus-5", "claude-opus-5-5", "claude-fable-5-1"):
        fallback = {"extra_headers": {"anthropic-beta": "server-side-fallback-2026-07-01"},
                    "extra_body": {"fallbacks": "default"}}

    def call(system: str, user: str) -> Extraction:
        resp = client.messages.parse(
            model=config.MODEL,
            max_tokens=16000,
            system=[{"type": "text", "text": system, "cache_control": {"type": "ephemeral"}}],
            messages=[{"role": "user", "content": user}],
            output_format=Extraction,
            **fallback,
        )
        if resp.stop_reason == "refusal" or resp.parsed_output is None:
            raise RuntimeError(f"no extraction (stop_reason={resp.stop_reason})")
        return resp.parsed_output

    return call


def copy_core_card(con, card: CoreCard) -> int:
    """Baseline without the LLM: the shadow card is the fetcher's own fields (model 'core-copy').
    Shows what the deterministic deviation check finds on its own; LLM extraction then runs with --refresh."""
    return store.save_shadow(con, card, "core-copy", dict(card.core_fields), {}, {}, [])


def extract_card(con, card: CoreCard, llm: LLM, reviewed: list[tuple[CoreCard, dict]] = (), refresh=False) -> dict:
    """Return the configured model's shadow card for this card content, calling the LLM only on a cache
    miss or refresh. A reading by another model, or grounded on an older policy version, is a miss."""
    pv = policy.current_version(con)
    if not refresh and (row := store.latest_shadow(con, card, config.MODEL, pv)):
        return store.shadow_dict(row)
    examples = pick_examples(card, list(reviewed))
    raw = llm(SYSTEM, build_prompt(card, examples))
    fields, quotes, problems, provenance = check(card, raw)
    sid = store.save_shadow(con, card, config.MODEL, fields, quotes, problems,
                            [ex.ref for ex, _ in examples], pv, provenance)
    return store.shadow_dict(store.latest_shadow(con, card, config.MODEL, pv)) | {"id": sid}
