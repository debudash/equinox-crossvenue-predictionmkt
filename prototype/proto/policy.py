"""Venue policy snapshots, codified clauses and the policy version stamped on readings, decisions and routes.

Daily at 00:15 (scripts/refresh_policies.cmd via Windows Task Scheduler) `python -m proto policies refresh`:
1. fetches every document in policies/sources.yaml and extracts its text;
2. compares it with the last snapshot (whitespace-collapsed SHA-256);
3. checks every clause in policies/clauses.yaml is still in its source (letters and digits compared);
4. if any document or the clause file changed, issues a new policy version "YYYY-MM-DD.n" and flags it.
A failed fetch keeps the previous snapshot and is flagged; it never creates a version.
"""

import difflib
import hashlib
import io
import json
import re
from datetime import datetime, timezone

import yaml

from . import config, store

POLICY_DIR = config.PROTO / "policies"

SCHEMA = """
CREATE TABLE IF NOT EXISTS policy_snapshots (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id  TEXT NOT NULL,
  url        TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  sha256     TEXT NOT NULL,
  text       TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS policy_versions (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  version       TEXT NOT NULL UNIQUE,         -- YYYY-MM-DD.n
  created_at    TEXT NOT NULL,
  sources_json  TEXT NOT NULL,                -- {source_id: snapshot id}
  clauses_sha   TEXT NOT NULL,
  changed_json  TEXT NOT NULL,                -- what made this a new version
  missing_json  TEXT NOT NULL                 -- clause ids no longer found in their source
);
-- One row per refresh run, changed or not, so the page shows the daily check history.
CREATE TABLE IF NOT EXISTS policy_checks (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  checked_at TEXT NOT NULL,
  version    TEXT,
  result     TEXT NOT NULL,                   -- unchanged | new_version | failed
  detail     TEXT NOT NULL
);
"""


def ensure(con) -> None:
    con.executescript(SCHEMA)
    for table, col in (("shadow_cards", "policy_version"), ("shadow_cards", "provenance_json"),
                       ("decisions", "policy_version"), ("route_log", "policy_version"),
                       ("route_log", "decision_policy_version")):
        if col not in {r["name"] for r in con.execute(f"PRAGMA table_info({table})")}:
            con.execute(f"ALTER TABLE {table} ADD COLUMN {col} TEXT")
    con.commit()


# ---- the codified files ---------------------------------------------------------------------

def sources() -> list[dict]:
    return yaml.safe_load((POLICY_DIR / "sources.yaml").read_text(encoding="utf-8"))["sources"]


def clauses() -> list[dict]:
    return yaml.safe_load((POLICY_DIR / "clauses.yaml").read_text(encoding="utf-8"))["clauses"]


def clauses_sha() -> str:
    return hashlib.sha256((POLICY_DIR / "clauses.yaml").read_bytes()).hexdigest()


def letters(text: str) -> str:
    """Letters and digits only, lowercased. PDF extraction splits words ("sett le") and mangles quotes,
    so clause presence and policy quotes are compared on this form."""
    return re.sub(r"[^a-z0-9]", "", text.lower())


def series_of(ref: str) -> str:
    return ref.split(":", 1)[1].split("-", 1)[0]


def applicable(ref: str) -> list[dict]:
    """Clauses that apply to one market: its venue, and its series or every series ("*")."""
    venue, series = ref.split(":", 1)[0], series_of(ref)
    by_source = {s["id"]: s for s in sources()}
    out = []
    for c in clauses():
        s = by_source[c["source"]]
        if s["venue"] == venue and ("*" in s["series"] or series in s["series"]):
            out.append(c)
    return out


# ---- fetching -----------------------------------------------------------------------------------

def fetch_text(url: str) -> str:
    import requests
    from pypdf import PdfReader

    r = requests.get(url, timeout=60, headers={"User-Agent": "Mozilla/5.0 (equinox prototype policy snapshot)"})
    r.raise_for_status()
    if not r.content.startswith(b"%PDF"):
        raise ValueError(f"not a PDF ({r.headers.get('content-type')})")
    pages = PdfReader(io.BytesIO(r.content)).pages
    return "\n".join(p.extract_text() or "" for p in pages)


def _sha(text: str) -> str:
    return hashlib.sha256(" ".join(text.split()).encode()).hexdigest()


# ---- refresh -----------------------------------------------------------------------------------

def current(con):
    ensure(con)
    return con.execute("SELECT * FROM policy_versions ORDER BY id DESC LIMIT 1").fetchone()


def current_version(con) -> str | None:
    row = current(con)
    return row["version"] if row else None


def _latest_snapshot(con, source_id):
    return con.execute("SELECT * FROM policy_snapshots WHERE source_id=? ORDER BY id DESC LIMIT 1", (source_id,)).fetchone()


def _check(con, version, result, detail) -> None:
    con.execute("INSERT INTO policy_checks (checked_at, version, result, detail) VALUES (?,?,?,?)",
                (store.now(), version, result, json.dumps(detail)))
    con.commit()


def refresh(con, fetch=fetch_text) -> dict:
    """One daily run. Returns {result, version, changed, missing, failed}."""
    ensure(con)
    prev = current(con)
    prev_sources = json.loads(prev["sources_json"]) if prev else {}
    snap_ids, changed, failed, texts = {}, [], {}, {}
    for s in sources():
        last = _latest_snapshot(con, s["id"])
        try:
            text = fetch(s["url"])
        except Exception as e:  # one failed document never stops the run; keep the last snapshot
            failed[s["id"]] = str(e)
            if last:
                snap_ids[s["id"]], texts[s["id"]] = last["id"], last["text"]
            continue
        if last is None or last["sha256"] != _sha(text):
            cur = con.execute("INSERT INTO policy_snapshots (source_id, url, fetched_at, sha256, text) VALUES (?,?,?,?,?)",
                              (s["id"], s["url"], store.now(), _sha(text), text))
            snap_ids[s["id"]] = cur.lastrowid
        else:
            snap_ids[s["id"]] = last["id"]
        texts[s["id"]] = text
        if prev_sources.get(s["id"]) != snap_ids[s["id"]]:
            changed.append(s["id"])
    con.commit()

    missing = [c["id"] for c in clauses() if c["source"] in texts and letters(c["text"]) not in letters(texts[c["source"]])]
    csha = clauses_sha()
    if prev and prev["clauses_sha"] != csha:
        changed.append("clauses.yaml")
    if prev and json.loads(prev["missing_json"]) != missing:
        changed.append("clause presence")

    if failed and not snap_ids:
        _check(con, prev["version"] if prev else None, "failed", {"failed": failed})
        return {"result": "failed", "version": prev["version"] if prev else None, "failed": failed}
    if prev and not changed:
        _check(con, prev["version"], "fetch_failed" if failed else "unchanged", {"failed": failed, "missing": missing})
        return {"result": "unchanged", "version": prev["version"], "changed": [], "missing": missing, "failed": failed}

    day = datetime.now(timezone.utc).strftime("%Y-%m-%d")
    n = con.execute("SELECT COUNT(*) FROM policy_versions WHERE version LIKE ?", (day + ".%",)).fetchone()[0] + 1
    version = f"{day}.{n}"
    con.execute("INSERT INTO policy_versions (version, created_at, sources_json, clauses_sha, changed_json, missing_json)"
                " VALUES (?,?,?,?,?,?)", (version, store.now(), json.dumps(snap_ids), csha,
                                          json.dumps(changed or ["first snapshot"]), json.dumps(missing)))
    con.commit()
    _check(con, version, "new_version", {"changed": changed or ["first snapshot"], "missing": missing, "failed": failed})
    return {"result": "new_version", "version": version, "changed": changed, "missing": missing, "failed": failed}


def diff(con, version: str, source_id: str) -> str:
    """Unified diff of one source between the version before `version` and `version` itself."""
    rows = con.execute("SELECT version, sources_json FROM policy_versions ORDER BY id").fetchall()
    idx = next(i for i, r in enumerate(rows) if r["version"] == version)
    new_id = json.loads(rows[idx]["sources_json"]).get(source_id)
    old_id = json.loads(rows[idx - 1]["sources_json"]).get(source_id) if idx else None
    text = lambda sid: (con.execute("SELECT text FROM policy_snapshots WHERE id=?", (sid,)).fetchone() or [""])[0] if sid else ""
    split = lambda t: re.split(r"(?<=[.;:])\s+", " ".join(t.split()))
    return "\n".join(difflib.unified_diff(split(text(old_id)), split(text(new_id)), "previous", version, lineterm=""))


def prompt_block(ref: str) -> str:
    """Venue policy clauses for the extraction prompt. The LLM may quote them; code checks the quote."""
    cs = applicable(ref)
    if not cs:
        return ""
    lines = [f'- [{c["id"]}] ({c["field"]}) "{c["text"]}"' for c in cs]
    return ("<venue_policy>\nPublished venue terms that apply to this market. Use them only for what the card's own "
            "rules do not state, and quote them exactly when you do. The card's own rules win on any conflict.\n"
            + "\n".join(lines) + "\n</venue_policy>\n\n")


def quote_source(ref: str, quote: str) -> str | None:
    """The clause id a quote comes from, or None. Compared on letters and digits."""
    q = letters(quote)
    if len(q) < 8:
        return None
    for c in applicable(ref):
        if q in letters(c["text"]):
            return c["id"]
    return None


def schedule(con_factory, hour: int = 0, minute: int = 15) -> None:
    """Run refresh() every day at hour:minute local time (TZ). The hosted container's stand-in for the
    laptop's Windows scheduled task. One failed run is logged and the loop carries on."""
    import time
    from datetime import timedelta

    while True:
        now = datetime.now()
        nxt = now.replace(hour=hour, minute=minute, second=0, microsecond=0)
        if nxt <= now:
            nxt += timedelta(days=1)
        print(f"policies: next refresh {nxt:%Y-%m-%d %H:%M} local", flush=True)
        time.sleep((nxt - now).total_seconds())
        try:
            r = refresh(con_factory())
            print(f"policies: {r['result']} -> version {r['version']}", flush=True)
        except Exception as e:  # keep the schedule alive; the failure is visible in the log
            print(f"policies: refresh failed: {e}", flush=True)
