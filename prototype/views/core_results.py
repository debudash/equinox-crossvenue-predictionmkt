"""Equinox core: what the deterministic matcher and router decided, read from the core's own records.
Read-only, except the synthetic demo re-run, which runs the core's own seed script (scripts/seed)."""

import html
import os
import re
import subprocess
from collections import Counter

import pandas as pd
import streamlit as st

from proto import config, core_db, values
from views.ui import pill, step_header

demo = st.session_state.get("core_source", "Live") == "Synthetic demo"
core = core_db.connect(config.DEMO_DB if demo else None)
q = lambda sql, *a: core.execute(sql, a).fetchall()
# Readers see only completed cycles (amendment A2), the same rule the core's own feed follows.
DONE = "cycle_id IN (SELECT id FROM cycles WHERE status = 'completed')"
REF, CTR = config.REFERENCE_VENUE.capitalize(), config.COUNTER_VENUE.capitalize()

st.title("Equinox core")
st.caption("Fetch Kalshi and Polymarket, normalize each market into one card, match by exact payoff-field equality, "
           "and route the cheaper side from saved prices, automatically, for every match. No model anywhere in the decision.")
if demo:
    st.html(pill("SYNTHETIC DEMO", "violet") + pill("fixture markets built to match exactly; no venue is called", "muted"))

cycle = q("SELECT id, kind, completed_at FROM cycles WHERE status = 'completed' ORDER BY id DESC LIMIT 1")
cards = q(f"SELECT venue, COUNT(*) FROM (SELECT venue, market_id, MAX(id) FROM cards WHERE {DONE} "
          "GROUP BY venue, market_id) GROUP BY venue")
matchable = q(f"SELECT c.venue, COUNT(*) FROM (SELECT venue, market_id, MAX(id) id FROM cards WHERE {DONE} GROUP BY venue, market_id) l "
              "JOIN cards c ON c.id = l.id WHERE c.subject IS NOT NULL GROUP BY c.venue")
# Latest match record per pair, as the core router reads them.
latest = q("SELECT d.market_a, d.market_b, d.match_result, d.reason FROM decision_log d JOIN (SELECT MAX(id) id "
           f"FROM decision_log WHERE record_type = 'match' AND {DONE} GROUP BY market_a, market_b) l ON l.id = d.id")
results = Counter(r[2] for r in latest)
routes = q(f"SELECT COUNT(*), SUM(action != 'no_route') FROM decision_log WHERE record_type = 'route' AND {DONE}")[0]

st.html('<div class="flow">' + '<span class="arrow">→</span>'.join(
    f'<div class="step"><div class="k">{k}</div><div class="v">{v}</div><div class="s">{s}</div></div>' for k, v, s in [
        ("Fetch", sum(n for _, n in cards), "markets as cards, " + " · ".join(f"{v} {n}" for v, n in cards)),
        ("Normalize", sum(n for _, n in matchable), "cards in the Fed vocabulary"),
        ("Match", f"{len(latest):,}", "pairs compared"),
        ("Same or opposite", results["same"] + results["opposite"], "exact matches"),
        ("Route", routes[0] or 0, f"route records, {routes[1] or 0} bought"),
    ]) + "</div>")
if cycle:
    st.html(pill(f"latest completed cycle #{cycle[0][0]} · {cycle[0][1]}", "info")
            + pill(f"completed {cycle[0][2][:16].replace('T', ' ')} UTC", "muted")
            + pill("exact payoff-field equality", "muted") + pill("empty never matches", "muted"))

if demo:
    c1, c2 = st.columns([1, 3])
    if c1.button("Re-run synthetic demo", icon=":material/replay:", type="primary"):
        with st.status("Running the core seed: real fetchers on fixtures → matcher → router, prices stamped now ...",
                       expanded=True) as box:
            # The core's own demo path (AGENTS.md: go run ./scripts/seed), in the Go container used by `make race`,
            # then swapped in under the demo feed. See prototype/scripts/reseed_demo.ps1.
            if os.name == "nt":   # laptop: Go runs in Docker, then the demo feed is swapped and restarted
                cmd = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
                       str(config.PROTO / "scripts" / "reseed_demo.ps1")]
            else:                 # hosted image: the compiled seed adds a fresh cycle to data/demo.db
                cmd = [os.getenv("EQX_SEED_BIN", "/app/seed"), "--config", "config/equinox.yaml", "--db", "data/demo.db"]
            out = subprocess.run(cmd, capture_output=True, text=True, timeout=600, cwd=config.ROOT)
            st.code((out.stdout + out.stderr).strip()[-3000:] or "(no output)")
            box.update(label="Seed finished" if out.returncode == 0 else f"Seed failed (exit {out.returncode})",
                       state="complete" if out.returncode == 0 else "error")
        if out.returncode == 0:
            st.rerun()
    c2.caption("Rebuilds the synthetic database with the core's own code: the fixture Kalshi and Polymarket markets are "
               "fetched, matched exactly, and routed on both sides at the current time; the demo feed restarts on it. "
               "Prices older than 10 minutes refuse, so re-run just before a demo.")

# ---- automatic routes, explained ---------------------------------------------------------------------
pat_cost = re.compile(r"Compared (\w+) on (\w+) at ([\d.]+) with (\w+) on (\w+) at ([\d.]+); (\w+) (is cheaper|ties)")
match_reason = {(a, b): reason for a, b, _, reason in latest}


def name_of(ref):
    venue, mid = ref.split(":", 1)
    row = q("SELECT event_title, title FROM cards WHERE venue = ? AND market_id = ? ORDER BY id DESC LIMIT 1", venue, mid)
    return f"{row[0][0].rstrip('?')} · {row[0][1]}" if row else ref


def core_route_card(r) -> str:
    li = []

    def step(state, title, body):
        icon = {"ok": "✓", "fail": "✗", "skip": "·"}[state]
        li.append(f'<li class="{state}"><span class="ic">{icon}</span><div><b>{html.escape(title)}</b>'
                  f'<div class="why">{body}</div></div></li>')

    step("ok", f"Matched exactly: {r['match_result']}", html.escape(match_reason.get((r["market_a"], r["market_b"]), "")))
    step("ok", "What is compared", f"Buy <b>{r['side_a'].upper()}</b> on {REF} vs buy <b>{(r['side_b'] or '').upper()}</b> on "
         f"{CTR}" + (": opposite bets, so YES on one pays exactly when NO on the other pays." if r["match_result"] == "opposite"
                     else ": the same bet on both venues."))
    if r["action"] == "no_route":
        step("fail", "A price is not usable, so no route", html.escape(r["reason"].replace("No route: ", "")))
        step("skip", "Fees", "Not reached.")
        step("skip", "Lower cost wins", "Not reached.")
        head = pill("No route", "bad")
    else:
        step("ok", "Both prices usable", f"{REF} {r['side_a'].upper()} ask <b>{r['price_a']}</b> · {CTR} "
             f"{r['side_b'].upper()} ask <b>{r['price_b']}</b>, judged at as_of {r['as_of'][11:19]} UTC "
             f"(within the {config.PRICE_MAX_AGE_SECONDS // 60}-minute limit).")
        fee_sentence = r["reason"].split(". ", 1)[1] if ". " in r["reason"] else ""
        step("ok", "Fees applied" if r["fee_applied"] == "yes" else "Fees not applied", html.escape(fee_sentence))
        m = pat_cost.search(r["reason"])
        if m:
            ca, cb = values.micros(m.group(3)), values.micros(m.group(6))
            win_ref = r["action"] == f"buy_on_{config.REFERENCE_VENUE}"
            bars = "".join(
                f'<div class="bar"><span class="lbl">{v}</span><span class="track"><span class="fill {"win" if w else ""}" '
                f'style="width:{max(2, c * 100 // 1_000_000)}%"></span></span><span class="amt">{values.micros_str(c)}</span></div>'
                for v, c, w in ((REF, ca, win_ref), (CTR, cb, not win_ref)))
            verdict = (f"Exact tie: <b>{REF} wins as the reference venue</b>, so the choice never flips." if ca == cb else
                       f"<b>{REF if ca < cb else CTR}</b> is cheaper by <b>{values.micros_str(abs(ca - cb))}</b> per contract.")
            step("ok", "Lower cost wins", bars + verdict)
        venue = r["action"].removeprefix("buy_on_")
        side = r["side_a"] if venue == config.REFERENCE_VENUE else r["side_b"]
        head = pill(f"Buy {side.upper()} on {venue.capitalize()}", "good")
    return (f'<div class="rcard"><div class="names">{html.escape(name_of(r["market_a"]))} &nbsp;↔&nbsp; '
            f'{html.escape(name_of(r["market_b"]))}</div>{head}{pill(r["match_result"], "info")}'
            f'{pill("routed automatically " + r["decided_at"][:16].replace("T", " ") + " UTC", "muted")}'
            f'<ol class="trace">{"".join(li)}</ol></div>')


core.row_factory = None
rr = pd.read_sql(f"SELECT d.* FROM decision_log d JOIN (SELECT MAX(id) id FROM decision_log WHERE record_type = 'route' AND {DONE} "
                 "GROUP BY market_a, market_b, side_a) l ON l.id = d.id ORDER BY d.market_b, d.side_a", core)
step_header("Automatic routes", "Every exact match is routed on both sides as soon as its prices are saved, and again "
            "whenever a needed price changes. Latest route per pair and side, explained by the five routing rules.",
            "same / opposite matches + saved prices", "a route record per pair and side")
if len(rr):
    st.html("".join(core_route_card(r) for _, r in rr.iterrows()))
else:
    st.caption("No route records: the router only routes same or opposite pairs, and exact matching found none here. "
               "Switch the sidebar to Synthetic demo to see a match routed automatically.")

# ---- refusals --------------------------------------------------------------------------------------------
step_header("Why pairs were refused", "Every refusal names the first payoff field that was empty or different. "
            "Grouped here by that field, so the blockers are visible at a glance.",
            "the latest match record for every Kalshi × Polymarket pair", "a count per blocking field")
pat = re.compile(r"Not the same bet: (\w+) (is empty|differs)")
blockers, examples = Counter(), {}
for _, _, res, reason in latest:
    if res == "not_same" and (m := pat.search(reason)):
        key = (m.group(1), "empty" if m.group(2) == "is empty" else "differs")
        blockers[key] += 1
        examples.setdefault(key, reason)
total = sum(blockers.values()) or 1
st.html("".join(
    f'<div class="row"><div class="bar"><span class="lbl" style="flex-basis:150px"><b>{field}</b></span>'
    f'<span class="track"><span class="fill" style="width:{max(2, n * 100 // total)}%"></span></span>'
    f'<span class="amt">{n:,}</span></div>{pill(kind, "bad" if kind == "differs" else "warn")}'
    f'<div class="why">e.g. {html.escape(examples[(field, kind)])}</div></div>'
    for (field, kind), n in blockers.most_common()) or "<div class='row'>No refusals recorded.</div>")

if not demo:
    st.html(f'<div class="note"><b>{results["same"] + results["opposite"]} of {len(latest):,} pairs are routable by exact '
            "equality.</b> The same bets exist, written differently: Kalshi \"above 3.75%\" is Polymarket \"4.00% or above\", "
            "and neither venue's market rules state what happens to an undeterminable outcome. The proposal prototype finds "
            "those near misses and puts each one in front of a person.</div>")
    st.page_link("views/proposal.py", label="Open the near-miss proposal", icon=":material/arrow_forward:")
