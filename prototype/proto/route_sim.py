"""Step 4 - router executes deterministically. A line-for-line port of internal/route.Decide.

Only human-approved, non-stale pairs reach it. Prices are integer micro-dollars, times are integer
nanoseconds parsed from the core's fixed-width text, and the reason sentences match the core's
word for word, so tests can check parity against the core router's own logged decisions.
"""

from dataclasses import dataclass
from datetime import datetime, timezone

from . import config, values
from .core_db import Price

ONE_DOLLAR = 1_000_000


def to_ns(text: str) -> int:
    """'2026-10-10T04:30:29.832911400Z' -> integer nanoseconds since the epoch, exactly."""
    t = text.rstrip("Z")
    head, _, frac = t.partition(".")
    secs = int(datetime.fromisoformat(head).replace(tzinfo=timezone.utc).timestamp())
    return secs * 1_000_000_000 + int((frac + "000000000")[:9])


def go_duration(ns: int) -> str:
    """Go's time.Duration.String, so stale-price reasons read exactly like the core's."""
    if ns == 0:
        return "0s"
    sign, ns = ("-", -ns) if ns < 0 else ("", ns)
    if ns < 1_000_000_000:
        for unit, size in (("ns", 1), ("µs", 1_000), ("ms", 1_000_000)):
            if ns < size * 1000:
                return sign + _frac(ns, size) + unit
    h, rem = divmod(ns, 3_600_000_000_000)
    m, rem = divmod(rem, 60_000_000_000)
    out = (f"{h}h" if h else "") + (f"{m}m" if h or m else "") + _frac(rem, 1_000_000_000) + "s"
    return sign + out


def _frac(v: int, size: int) -> str:
    whole, part = divmod(v, size)
    digits = len(str(size)) - 1
    f = f"{part:0{digits}d}".rstrip("0") if digits else ""
    return f"{whole}" + (f".{f}" if f else "")


@dataclass
class Route:
    relation: str
    ref_a: str
    ref_b: str
    side_a: str
    side_b: str | None = None
    price_a: str | None = None
    price_b: str | None = None
    fee_a: str | None = None
    fee_b: str | None = None
    priced_at_a: str | None = None
    priced_at_b: str | None = None
    fee_applied: str = "no"
    action: str = "no_route"
    reason: str = ""
    as_of: str | None = None


def _usable(p: Price | None, ref: str, side: str, as_of_ns: int, max_age_ns: int) -> str:
    venue, mid = ref.split(":", 1)
    if p is None or p.buy_price is None:
        return f"The {venue} {side} price is missing for {mid}."
    m = values.micros(p.buy_price)
    if m is None or m < 0 or m > ONE_DOLLAR:
        return f"The {venue} {side} price for {mid} is outside 0 to 1 and was not used."
    at = to_ns(p.priced_at)
    if at > as_of_ns:
        return f"The {venue} {side} price for {mid} was saved after as_of."
    if as_of_ns - at > max_age_ns:
        return (f"The {venue} {side} price is stale for {mid}: saved {go_duration(as_of_ns - at)} before as_of, "
                f"over the {go_duration(max_age_ns)} limit.")
    return ""


def decide(relation: str, ref_a: str, ref_b: str, side: str, pa: Price | None, pb: Price | None,
           as_of: str, max_age_s: int = None, roles=None) -> Route:
    """Pure: same inputs, same Route. ref_a is the reference venue card; side is stated relative to it."""
    reference, counter = roles or (config.REFERENCE_VENUE, config.COUNTER_VENUE)
    max_age_ns = (max_age_s if max_age_s is not None else config.PRICE_MAX_AGE_SECONDS) * 1_000_000_000
    r = Route(relation=relation, ref_a=ref_a, ref_b=ref_b, side_a=side, as_of=as_of)

    # Rule 1: not_same is never routed.
    if relation not in ("same", "opposite"):
        r.reason = "No route: the bets are not the same."
        return r

    # Rule 2: opposite compares the other side.
    r.side_b = side if relation == "same" else ("no" if side == "yes" else "yes")

    # Rule 3: a missing or stale price means no route, naming which.
    as_of_ns = to_ns(as_of)
    pa_problem = _usable(pa, ref_a, r.side_a, as_of_ns, max_age_ns)
    pb_problem = _usable(pb, ref_b, r.side_b, as_of_ns, max_age_ns)
    if pa_problem or pb_problem:
        r.reason = "No route: " + f"{pa_problem} {pb_problem}".strip()
        return r
    r.price_a, r.price_b = pa.buy_price, pb.buy_price
    r.fee_a, r.fee_b = pa.fee, pb.fee
    r.priced_at_a, r.priced_at_b = pa.priced_at, pb.priced_at
    cost_a, cost_b = values.micros(pa.buy_price), values.micros(pb.buy_price)

    # Rule 4: fees count only when known on both venues.
    va, vb = ref_a.split(":", 1)[0], ref_b.split(":", 1)[0]
    unknown = "both venues" if pa.fee is None and pb.fee is None else (va if pa.fee is None else vb)
    fee_note = f"Fees were not applied because a fee is unknown on {unknown}."
    if pa.fee is not None and pb.fee is not None:
        cost_a, cost_b = cost_a + values.micros(pa.fee), cost_b + values.micros(pb.fee)
        r.fee_applied = "yes"
        fee_note = f"Costs include fees of {values.micros_str(values.micros(pa.fee))} and {values.micros_str(values.micros(pb.fee))}."

    # Rules 5 and 6: lower cost wins; an exact tie goes to the reference venue.
    winner, why = reference, "is cheaper"
    if cost_b < cost_a:
        winner = counter
    elif cost_a == cost_b:
        why = "ties and wins exact ties as the reference venue"
    r.action = f"buy_on_{winner}"
    r.reason = (f"Compared {r.side_a} on {va} at {values.micros_str(cost_a)} with {r.side_b} on {vb} at "
                f"{values.micros_str(cost_b)}; {winner} {why}. {fee_note}")
    return r


def as_of_from_prices(pa: Price | None, pb: Price | None) -> str | None:
    """The locked design uses the prices' own timestamp as as_of: the later of the two saved prices.
    The earlier one must then be within the price age limit of it, or the route is refused as stale."""
    stamps = [p.priced_at for p in (pa, pb) if p is not None]
    return max(stamps) if stamps else None
