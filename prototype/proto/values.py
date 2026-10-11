"""Exact value handling shared by the deviation check and the routing simulation.

Ports of the core's card.ParseDecimal, card.ParseMicros and card.ParseInstant rules, so the
prototype normalizes the same way the core does. No floats anywhere (AGENTS.md hard rule 5).
"""

import re
from datetime import datetime, timezone

_DEC = re.compile(r"^-?\d+(\.\d+)?$")


def norm_decimal(s: str) -> str | None:
    """'03.500' -> '3.5', '4.00' -> '4'. None when not a plain decimal."""
    if s is None or not _DEC.match(s):
        return None
    neg = s.startswith("-")
    whole, _, frac = s.lstrip("-").partition(".")
    whole, frac = whole.lstrip("0") or "0", frac.rstrip("0")
    out = whole + ("." + frac if frac else "")
    return "-" + out if neg and out != "0" else out


def micros(s: str) -> int | None:
    """Decimal string -> integer micro-units, exactly. None when not representable in 6 places."""
    if s is None or not _DEC.match(s):
        return None
    neg = s.startswith("-")
    whole, _, frac = s.lstrip("-").partition(".")
    frac = frac.rstrip("0")
    if len(frac) > 6 or len(whole) > 12:
        return None
    v = int(whole) * 1_000_000 + int((frac + "000000")[:6])
    return -v if neg else v


def micros_str(m: int) -> str:
    sign, m = ("-", -m) if m < 0 else ("", m)
    whole, frac = divmod(m, 1_000_000)
    f = f"{frac:06d}".rstrip("0")
    return f"{sign}{whole}" + (f".{f}" if f else "")


def add_decimal(a: str, b: str) -> str:
    return micros_str(micros(a) + micros(b))


def norm_instant(s: str) -> str | None:
    """RFC 3339 with an offset -> 'YYYY-MM-DDTHH:MM:SSZ' in UTC. Rejects timestamps without an offset."""
    if not s:
        return None
    try:
        t = datetime.fromisoformat(s.replace("Z", "+00:00"))
    except ValueError:
        return None
    if t.tzinfo is None:
        return None
    return t.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def parse_utc(s: str) -> datetime:
    """Core fixed-width text (nanoseconds) or plain RFC 3339 -> aware datetime (microsecond precision)."""
    s = s.replace("Z", "")
    if "." in s:
        head, frac = s.split(".", 1)
        s = f"{head}.{frac[:6]}"
    return datetime.fromisoformat(s).replace(tzinfo=timezone.utc)


def core_text(t: datetime) -> str:
    """Aware datetime -> the core's fixed-width UTC text, so text comparison is time comparison."""
    return t.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f") + "000Z"
