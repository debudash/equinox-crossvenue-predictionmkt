"""Read-only view of the core database (data/equinox.db). The prototype never writes here."""

import hashlib
import json
import sqlite3
from dataclasses import dataclass

from . import config


def connect(path=None) -> sqlite3.Connection:
    c = sqlite3.connect(f"file:{(path or config.CORE_DB).as_posix()}?mode=ro", uri=True)
    c.row_factory = sqlite3.Row
    return c


@dataclass(frozen=True)
class CoreCard:
    card_id: int           # one row per card version; a new id means the card changed
    venue: str
    market_id: str
    title: str
    rules_text: str
    event_title: str
    category: str
    core_fields: dict      # what the deterministic fetcher stored (None = empty)

    @property
    def ref(self) -> str:
        return f"{self.venue}:{self.market_id}"

    @property
    def fingerprint(self) -> str:
        """What decides the payoff: title, rules text, event and the fetcher's payoff fields.
        Volume, url and category are left out, so a volume change is not a new card to the prototype."""
        body = json.dumps([self.title, self.rules_text, self.event_title, self.core_fields], sort_keys=True)
        return hashlib.sha256(body.encode()).hexdigest()[:16]


# Latest version per market, limited to the Fed scope the core tries to match:
# cards with a subject, plus the Polymarket macro category (FOMC decision markets the
# seven-field card cannot express, so the core leaves their subject or cutoff empty).
_SCOPE_SQL = f"""
SELECT c.* FROM cards c
JOIN (SELECT venue, market_id, MAX(id) AS id FROM cards GROUP BY venue, market_id) l ON l.id = c.id
WHERE c.subject IS NOT NULL OR c.category = 'macro'
ORDER BY c.venue, c.market_id
"""


def _card(r: sqlite3.Row) -> CoreCard:
    return CoreCard(r["id"], r["venue"], r["market_id"], r["title"], r["rules_text"], r["event_title"],
                    r["category"], {f: r[f] for f in config.FIELDS})


def scope_cards(con: sqlite3.Connection) -> list[CoreCard]:
    return [_card(r) for r in con.execute(_SCOPE_SQL)]


def fingerprint_of(con: sqlite3.Connection, card_id: int) -> str | None:
    """Fingerprint of any saved card version, for migrating rows written before fingerprints existed."""
    r = con.execute("SELECT * FROM cards WHERE id=?", (card_id,)).fetchone()
    return _card(r).fingerprint if r else None


@dataclass(frozen=True)
class Price:
    venue: str
    market_id: str
    side: str
    buy_price: str | None   # decimal string, None = missing
    fee: str | None
    priced_at: str          # fixed-width UTC text, so text order is time order


def latest_price(con: sqlite3.Connection, ref: str, side: str, as_of: str | None = None) -> Price | None:
    """Latest saved price for one card side, optionally not after as_of (same rule as the core, D-07)."""
    venue, market_id = ref.split(":", 1)
    sql = "SELECT venue, market_id, side, buy_price, fee, priced_at FROM prices WHERE venue=? AND market_id=? AND side=?"
    args = [venue, market_id, side]
    if as_of:
        sql += " AND priced_at <= ?"
        args.append(as_of)
    r = con.execute(sql + " ORDER BY priced_at DESC, id DESC LIMIT 1", args).fetchone()
    return Price(*r) if r else None
