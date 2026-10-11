"""Equinox core: the live feed, rendered in the app's theme from Equinox's own records.
Same data as `equinox serve` (every market Equinox tracks, saved prices, match status), read-only;
Equinox's own feed page stays available through the link at the top."""

import html
import os
import re
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path

import streamlit as st

from proto import config, core_db, feed, values
from views.ui import pill, step_header

demo = st.session_state.get("core_source", "Live") == "Synthetic demo"
DB = config.DEMO_DB if demo else config.CORE_DB
FEED_URL = (os.getenv("EQX_CORE_DEMO_FEED_URL", "http://127.0.0.1:8081") if demo
            else os.getenv("EQX_CORE_FEED_URL", "http://127.0.0.1:8080"))
DONE = "cycle_id IN (SELECT id FROM cycles WHERE status = 'completed')"   # readers see completed cycles only (A2)
VENUE_PILL = {"kalshi": "good", "polymarket": "violet"}
REFUSAL = re.compile(r"Not the same bet: (\w+) (is empty|differs)")
DASH = "–"


@st.cache_data(ttl=60, show_spinner=False)
def load(db_path: str, cycle: int):
    """Markets, prices and per-market match status for one completed cycle (the cycle id keys the cache)."""
    core = core_db.connect(Path(db_path))
    core.row_factory = None
    rows = core.execute(
        "SELECT c.venue, c.market_id, c.title, c.event_title, c.category, c.volume_24h, c.url, c.deadline, c.subject "
        f"FROM cards c JOIN (SELECT venue, market_id, MAX(id) id FROM cards WHERE {DONE} GROUP BY venue, market_id) l "
        "ON l.id = c.id").fetchall()
    quotes = {}
    latest = feed.latest_prices(core)
    for venue, mid, *_ in rows:
        ref = f"{venue}:{mid}"
        q = feed.quote(latest.get((ref, "yes")), latest.get((ref, "no")))
        quotes[ref] = (q.yes_ask, q.yes_bid, q.no_ask, q.no_bid)
    matched, refusals = defaultdict(list), defaultdict(Counter)
    for a, b, res, reason in core.execute(
            "SELECT d.market_a, d.market_b, d.match_result, d.reason FROM decision_log d JOIN (SELECT MAX(id) id "
            f"FROM decision_log WHERE record_type = 'match' AND {DONE} GROUP BY market_a, market_b) l ON l.id = d.id"):
        if res in ("same", "opposite"):
            matched[a].append((res, b))
            matched[b].append((res, a))
        elif m := REFUSAL.search(reason):
            key = f"{m.group(1)} {'empty' if m.group(2) == 'is empty' else 'differs'}"
            refusals[a][key] += 1
            refusals[b][key] += 1
    return rows, quotes, dict(matched), {k: v.most_common(1)[0][0] for k, v in refusals.items()}


core = core_db.connect(DB)
cycle = core.execute("SELECT id, completed_at FROM cycles WHERE status = 'completed' ORDER BY id DESC LIMIT 1").fetchone()
st.title("Live feed")
step_header("Equinox live feed", "Every market Equinox tracks, as it saved them: prices, volume, closing time and match "
            "status. Fed markets are matched; the other categories are display only and never matched.",
            "Equinox's saved markets and prices, latest completed cycle", "nothing; this is a view")
if not cycle:
    st.info("No completed cycle yet: Equinox is on its first fetch.")
    st.stop()
rows, quotes, matched, refusal = load(str(DB), cycle[0])
now = datetime.now(timezone.utc)


def vol(v: str) -> float:
    try:
        return float(v or 0)          # display sort and label only; never used in a decision
    except ValueError:
        return 0.0


def compact(v: str) -> str:
    x = vol(v)
    return "volume n/a" if not x else f"${x / 1e6:.1f}M" if x >= 1e6 else f"${x / 1e3:.1f}K" if x >= 1e3 else f"${x:,.0f}"


def closes(d: str | None) -> str:
    if not d:
        return "close unknown"
    hours = (values.parse_utc(d) - now).total_seconds() / 3600
    return "closed" if hours < 0 else f"closes in {hours:.0f} h" if hours < 48 else f"closes in {hours / 24:.0f} d"


def card(r) -> str:
    venue, mid, title, event, category, volume, url, deadline, subject = r
    ref = f"{venue}:{mid}"
    ya, yb, na, nb = quotes[ref]
    if subject is None:
        status = pill("display only · never matched", "muted")
    elif ref in matched:
        status = "".join(pill(f"{res} with {o.split(':', 1)[0].capitalize()}", "good") for res, o in matched[ref][:2])
    elif ref in refusal:
        status = pill("no exact match", "bad") + pill(f"closest refusal: {refusal[ref]}", "warn")
    else:
        status = pill("not compared yet", "muted")
    link = f' · <a href="{html.escape(url)}" target="_blank">venue page</a>' if url else ""
    return (f'<div class="rcard fcard"><div>{pill(venue.upper(), VENUE_PILL.get(venue, "muted"))}{pill(category, "info")}</div>'
            f'<div class="names">{html.escape(event.rstrip("?"))}</div><div class="why">{html.escape(title)}</div>'
            f'<div style="margin:8px 0">{pill(f"YES ask {ya or DASH} · bid {yb or DASH}", "muted")}'
            f'{pill(f"NO ask {na or DASH} · bid {nb or DASH}", "muted")}</div>'
            f'<div>{status}</div><div class="why" style="margin-top:6px">{compact(volume)}{"" if not vol(volume) else " 24h volume"} · '
            f'{closes(deadline)}{link}</div></div>')


fed = [r for r in rows if r[8] is not None]
n_matched = sum(1 for r in fed if f"{r[0]}:{r[1]}" in matched)
st.html(pill(f"cycle #{cycle[0]} · {cycle[1][:16].replace('T', ' ')} UTC", "info")
        + pill(f"{len(rows):,} markets", "muted") + pill(f"{len(fed):,} Fed markets compared", "muted")
        + pill(f"{n_matched} with an exact match", "good" if n_matched else "muted")
        + (pill("SYNTHETIC DEMO", "violet") if demo else ""))
if FEED_URL:
    st.link_button("Open Equinox's own feed page", FEED_URL, icon=":material/open_in_new:")

def is_open(r) -> bool:
    return not r[7] or values.parse_utc(r[7]) > now


show_closed = st.toggle("Include closed markets", False)
rows = [r for r in rows if show_closed or is_open(r)]
cats = sorted({r[4] for r in rows})
view = st.segmented_control("View", ["Overview", "All markets"], default="Overview") or "Overview"
if view == "Overview":
    st.caption("Top 5 by 24h volume in each category, both venues represented where they list it.")
    for cat in cats:
        in_cat = sorted((r for r in rows if r[4] == cat), key=lambda r: -vol(r[5]))
        top = sorted([r for v in ("kalshi", "polymarket") for r in [x for x in in_cat if x[0] == v][:3]],
                     key=lambda r: -vol(r[5]))[:5]
        st.markdown(f"**{cat}** · {len(in_cat)} markets")
        cols = st.columns(5)
        for col, r in zip(cols, top):
            with col:
                st.html(card(r))
else:
    f1, f2, f3, f4 = st.columns([2, 1, 1, 2])
    pick = f1.multiselect("Category", cats, default=cats)
    venues = f2.multiselect("Venue", ["kalshi", "polymarket"], default=["kalshi", "polymarket"])
    order = f3.selectbox("Sort", ["24h volume", "Closing soon"])
    text = f4.text_input("Search").lower()
    sel = [r for r in rows if r[4] in pick and r[0] in venues and text in (r[2] + r[3] + r[1]).lower()]
    sel.sort(key=(lambda r: -vol(r[5])) if order == "24h volume" else (lambda r: r[7] or "9999"))
    pages = max(1, -(-len(sel) // 24))
    page = st.number_input(f"Page (of {pages}, {len(sel):,} markets)", 1, pages, 1)
    grid = st.columns(3)
    for i, r in enumerate(sel[(page - 1) * 24: page * 24]):
        with grid[i % 3]:
            st.html(card(r))
