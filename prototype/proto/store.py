"""data/proposals.db: shadow cards, review decisions and simulated routes. The only file the prototype writes."""

import json
import sqlite3
from datetime import datetime, timezone

from . import config

SCHEMA = """
-- One LLM extraction per card version (card_id is the core card row) and extractor version.
CREATE TABLE IF NOT EXISTS shadow_cards (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  card_id           INTEGER NOT NULL,
  ref               TEXT NOT NULL,            -- venue:market_id
  extractor_version TEXT NOT NULL,
  model             TEXT NOT NULL,
  fields_json       TEXT NOT NULL,            -- {field: value or null} after quote checks
  quotes_json       TEXT NOT NULL,            -- {field: verbatim quote or null}
  problems_json     TEXT NOT NULL,            -- {field: why it was emptied}
  example_refs_json TEXT NOT NULL,            -- reviewed cards used as few-shot examples
  created_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS shadow_by_card ON shadow_cards (card_id, extractor_version, id);

-- One human decision on a proposed pair. Append-only: a new decision supersedes the old one.
CREATE TABLE IF NOT EXISTS decisions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  ref_a       TEXT NOT NULL,                  -- reference venue card
  ref_b       TEXT NOT NULL,                  -- counter venue card
  card_id_a   INTEGER NOT NULL,               -- card versions the reviewer saw
  card_id_b   INTEGER NOT NULL,
  shadow_id_a INTEGER NOT NULL,
  shadow_id_b INTEGER NOT NULL,
  pattern     TEXT NOT NULL,                  -- blocking pattern the pair was grouped under
  verdict     TEXT NOT NULL CHECK (verdict IN ('same', 'opposite', 'reject')),
  reason      TEXT NOT NULL CHECK (length(trim(reason)) > 0),
  reviewer    TEXT NOT NULL,
  applied_to_group INTEGER NOT NULL DEFAULT 0,
  decided_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS decisions_pair ON decisions (ref_a, ref_b, id);

-- Per-field judgement on an extraction. These become few-shot examples and the eval set.
CREATE TABLE IF NOT EXISTS field_reviews (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  decision_id     INTEGER NOT NULL REFERENCES decisions (id),
  shadow_id       INTEGER NOT NULL REFERENCES shadow_cards (id),
  card_id         INTEGER NOT NULL,
  field           TEXT NOT NULL,
  extracted_value TEXT,
  correct         INTEGER NOT NULL CHECK (correct IN (0, 1)),
  corrected_value TEXT
);

-- Simulated routing choices with every input, like the core decision log.
CREATE TABLE IF NOT EXISTS route_log (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  decision_id INTEGER NOT NULL REFERENCES decisions (id),
  decided_at  TEXT NOT NULL,
  as_of       TEXT,
  ref_a TEXT NOT NULL, ref_b TEXT NOT NULL,
  relation    TEXT NOT NULL,
  side_a TEXT, side_b TEXT,
  price_a TEXT, price_b TEXT, fee_a TEXT, fee_b TEXT,
  priced_at_a TEXT, priced_at_b TEXT,
  fee_applied TEXT,
  action      TEXT NOT NULL,
  reason      TEXT NOT NULL
);

CREATE TRIGGER IF NOT EXISTS decisions_no_update BEFORE UPDATE ON decisions BEGIN SELECT RAISE(ABORT, 'decisions is append-only'); END;
CREATE TRIGGER IF NOT EXISTS decisions_no_delete BEFORE DELETE ON decisions BEGIN SELECT RAISE(ABORT, 'decisions is append-only'); END;
CREATE TRIGGER IF NOT EXISTS route_log_no_update BEFORE UPDATE ON route_log BEGIN SELECT RAISE(ABORT, 'route_log is append-only'); END;
CREATE TRIGGER IF NOT EXISTS route_log_no_delete BEFORE DELETE ON route_log BEGIN SELECT RAISE(ABORT, 'route_log is append-only'); END;
"""


def now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def connect(path=None) -> sqlite3.Connection:
    c = sqlite3.connect(path or config.PROPOSALS_DB)
    c.row_factory = sqlite3.Row
    c.executescript(SCHEMA)
    # Content fingerprints (added after the first schema): readings and decisions follow what decides the
    # payoff, not the core's card version, which also changes when only the 24h volume moves.
    for table, col in (("shadow_cards", "fingerprint"), ("decisions", "fingerprint_a"), ("decisions", "fingerprint_b"),
                       # policy version (proto/policy.py) each reading, decision and route relied on
                       ("shadow_cards", "policy_version"), ("shadow_cards", "provenance_json"),
                       ("decisions", "policy_version"), ("route_log", "policy_version"),
                       ("route_log", "decision_policy_version")):
        if col not in {r["name"] for r in c.execute(f"PRAGMA table_info({table})")}:
            c.execute(f"ALTER TABLE {table} ADD COLUMN {col} TEXT")
    return c


def backfill_fingerprints(con, fingerprint_of) -> None:
    """Fill fingerprints on rows written before the column existed. fingerprint_of: core card id -> fingerprint."""
    for r in con.execute("SELECT id, card_id FROM shadow_cards WHERE fingerprint IS NULL").fetchall():
        if fp := fingerprint_of(r["card_id"]):
            con.execute("UPDATE shadow_cards SET fingerprint=? WHERE id=?", (fp, r["id"]))
    # Decisions are append-only. Filling a fingerprint the row was written without is a one-time
    # migration, so the update trigger is lifted for it and restored right after.
    rows = con.execute("SELECT id, card_id_a, card_id_b FROM decisions WHERE fingerprint_a IS NULL").fetchall()
    if rows:
        con.execute("DROP TRIGGER IF EXISTS decisions_no_update")
        for r in rows:
            con.execute("UPDATE decisions SET fingerprint_a=?, fingerprint_b=? WHERE id=?",
                        (fingerprint_of(r["card_id_a"]), fingerprint_of(r["card_id_b"]), r["id"]))
        con.commit()
        con.executescript(SCHEMA)
    con.commit()


# ---- shadow cards -------------------------------------------------------------------------

def save_shadow(con, card, model, fields, quotes, problems, example_refs, policy_version=None, provenance=None) -> int:
    cur = con.execute(
        "INSERT INTO shadow_cards (card_id, ref, fingerprint, extractor_version, model, fields_json, quotes_json,"
        " problems_json, example_refs_json, created_at, policy_version, provenance_json)"
        " VALUES (?,?,?,?,?,?,?,?,?,?,?,?)",
        (card.card_id, card.ref, card.fingerprint, config.EXTRACTOR_VERSION, model, json.dumps(fields),
         json.dumps(quotes), json.dumps(problems), json.dumps(example_refs), now(), policy_version,
         json.dumps(provenance or {})))
    con.commit()
    return cur.lastrowid


def latest_shadow(con, card, model: str | None = None, policy_version: str | None = None):
    """Latest reading of this card's content (any card version with the same fingerprint);
    with model, only a reading by that model; with policy_version, only one grounded on that version."""
    sql = "SELECT * FROM shadow_cards WHERE ref=? AND fingerprint=? AND extractor_version=?"
    args = [card.ref, card.fingerprint, config.EXTRACTOR_VERSION]
    if model:
        sql, args = sql + " AND model=?", args + [model]
    if policy_version:
        sql, args = sql + " AND policy_version=?", args + [policy_version]
    return con.execute(sql + " ORDER BY id DESC LIMIT 1", args).fetchone()


def shadow_dict(row) -> dict:
    return {"id": row["id"], "card_id": row["card_id"], "ref": row["ref"], "model": row["model"],
            "fingerprint": row["fingerprint"], "policy_version": row["policy_version"],
            "provenance": json.loads(row["provenance_json"] or "{}"),
            "fields": json.loads(row["fields_json"]), "quotes": json.loads(row["quotes_json"]),
            "problems": json.loads(row["problems_json"]), "examples": json.loads(row["example_refs_json"])}


# ---- decisions ----------------------------------------------------------------------------

def record_decision(con, pair, verdict, reason, reviewer, field_reviews=(), applied_to_group=False,
                    policy_version=None) -> int:
    """pair is a compare.Proposal. field_reviews: (side 'a'|'b', field, correct, corrected_value).
    policy_version: the venue policy version in force when the reviewer decided."""
    cur = con.execute(
        "INSERT INTO decisions (ref_a, ref_b, card_id_a, card_id_b, fingerprint_a, fingerprint_b, shadow_id_a,"
        " shadow_id_b, pattern, verdict, reason, reviewer, applied_to_group, decided_at, policy_version)"
        " VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
        (pair.a["ref"], pair.b["ref"], pair.a["card_id"], pair.b["card_id"], pair.a["fingerprint"],
         pair.b["fingerprint"], pair.a["id"], pair.b["id"], pair.pattern, verdict, reason, reviewer,
         int(applied_to_group), now(), policy_version))
    did = cur.lastrowid
    for side, field, correct, corrected in field_reviews:
        s = pair.a if side == "a" else pair.b
        con.execute(
            "INSERT INTO field_reviews (decision_id, shadow_id, card_id, field, extracted_value, correct, corrected_value)"
            " VALUES (?,?,?,?,?,?,?)",
            (did, s["id"], s["card_id"], field, s.get("llm_fields", s["fields"]).get(field), int(correct), None if correct else corrected))
    con.commit()
    return did


def latest_decisions(con) -> dict[tuple[str, str], sqlite3.Row]:
    rows = con.execute("SELECT d.* FROM decisions d JOIN (SELECT MAX(id) id FROM decisions GROUP BY ref_a, ref_b) l"
                       " ON l.id = d.id")
    return {(r["ref_a"], r["ref_b"]): r for r in rows}


def is_stale(decision, current: dict[str, str]) -> bool:
    """A decision covers the card content the reviewer saw (title, rules, event, payoff fields).
    current: ref -> fingerprint now. A changed rules text makes it stale; a volume change does not."""
    return (current.get(decision["ref_a"]) != decision["fingerprint_a"]
            or current.get(decision["ref_b"]) != decision["fingerprint_b"])


# ---- gold values for learning and evals -----------------------------------------------------

def gold_by_fingerprint(con) -> dict[str, dict]:
    """card content fingerprint -> {field: gold value} from the most recent review of each field.
    Gold is the extracted value when marked correct, else the reviewer's corrected value (None = empty)."""
    gold: dict[str, dict] = {}
    for r in con.execute("SELECT r.*, s.fingerprint FROM field_reviews r JOIN shadow_cards s ON s.id = r.shadow_id"
                         " ORDER BY r.id"):
        gold.setdefault(r["fingerprint"], {})[r["field"]] = r["extracted_value"] if r["correct"] else r["corrected_value"]
    return gold


# ---- routes ---------------------------------------------------------------------------------

def logged_route(con, decision_id: int, side: str):
    """The route already logged for this decision and requested side, if any. One per decision and side:
    a routing decision is recorded once; a new human decision on the pair can be routed again."""
    return con.execute("SELECT * FROM route_log WHERE decision_id=? AND side_a=? ORDER BY id LIMIT 1",
                       (decision_id, side)).fetchone()


def save_route(con, decision_id, rec: dict) -> int:
    cols = ["decided_at", "as_of", "ref_a", "ref_b", "relation", "side_a", "side_b", "price_a", "price_b",
            "fee_a", "fee_b", "priced_at_a", "priced_at_b", "fee_applied", "action", "reason",
            "policy_version", "decision_policy_version"]
    cur = con.execute(f"INSERT INTO route_log (decision_id, {', '.join(cols)}) VALUES (?{',?' * len(cols)})",
                      [decision_id, now()] + [rec.get(c) for c in cols[1:]])
    con.commit()
    return cur.lastrowid
