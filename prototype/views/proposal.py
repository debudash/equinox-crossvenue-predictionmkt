"""Proposal prototype: one tab per step of the flow, in order, each opening with what happens there,
what goes in and what comes out.
Markets -> Venue policies -> 1 LLM reads -> 2 Rules measure -> 3 Human decides -> 4 Router executes -> Learning loop
"""

import html
import json
import os
from datetime import datetime, timezone

import pandas as pd
import streamlit as st

from proto import compare, config, core_db, evals, extract, feed, pipeline, policy, route_sim, store, values
from views.routing import CTR, REF, frozen, route_card
from views.ui import (ACCENT, CHECK, RELATION, TIER, action_pill, decision_pill, pattern_pills, pill,  # noqa: F401
                      step_header)

# ---- data -----------------------------------------------------------------------------------------
core, con = pipeline.open_stores()
cards = core_db.scope_cards(core)
by_ref = {c.ref: c for c in cards}
policy_version = policy.current_version(con)
proposals, counts = pipeline.build_proposals(con, cards)
decisions = store.latest_decisions(con)
latest_ids = pipeline.current_fingerprints(cards)
n_shadow = sum(1 for c in cards if pipeline.has_llm_shadow(con, c))
n_done = sum(1 for p in proposals if p.key in decisions and not store.is_stale(decisions[p.key], latest_ids))
n_routes = con.execute("SELECT COUNT(*) FROM route_log").fetchone()[0]
n_reviews = con.execute("SELECT COUNT(*) FROM field_reviews").fetchone()[0]

# ---- header: the whole flow at a glance -----------------------------------------------------------
st.title("Equivalence before clearing")
exact = core.execute("SELECT COUNT(DISTINCT market_a || market_b) FROM decision_log "
                     "WHERE record_type = 'match' AND match_result IN ('same', 'opposite')").fetchone()[0]
model_name = config.MODEL.replace("-", " ").title()   # claude-sonnet-5 -> Claude Sonnet 5
st.html(f"""
<div class="hero">
  <div class="lede">Exact field matching routes <b>{exact}</b> of <b>{counts['pairs checked']:,}</b> Kalshi × Polymarket Fed
  pairs. The same bets exist; the venues just write them differently. This proposal finds them
  <b>without letting a model decide</b>: <b>{counts['near_miss'] + counts['strong']}</b> candidate pairs, each one a human call.</div>
  <div class="beats">
    <div class="beat"><div class="h">Read</div>{html.escape(model_name)} reads each market's rules and fills the seven
      payoff fields. Every value carries a word-for-word quote; code drops any quote it cannot find.</div>
    <div class="beat"><div class="h">Ground in venue policy + normalize</div>Kalshi's contract terms and the Polymarket
      US rulebook are snapshotted daily; the model may cite only codified clauses. Every value is normalized into the shared
      vocabulary (operators, units, exact decimals, UTC times), and anything outside it is dropped. Each reading, decision
      and route records the policy version: <b>{html.escape(policy_version or 'none yet')}</b>.</div>
    <div class="beat"><div class="h">Measure</div>Deterministic rules name every difference between two markets. Anything
      that changes the payoff drops the pair; only near misses reach a person.</div>
    <div class="beat"><div class="h">Decide, then route</div>A reviewer approves or rejects with a reason. Approved pairs
      go through the same routing rules as Equinox: same prices, same refusals, logged.</div>
  </div>
</div>""")

flow = [("Markets", f"{len(cards)}", "Fed markets in Equinox"),
        ("Venue policies", policy_version or "none", "version in force"),
        ("1 · LLM reads", f"{n_shadow}/{len(cards)}", "cards read on this version"),
        ("2 · Rules measure", f"{counts['near_miss'] + counts['strong']}", f"near misses of {counts['pairs checked']:,} pairs"),
        ("3 · Human decides", f"{n_done}/{len(proposals)}", "pairs decided"),
        ("4 · Router executes", f"{n_routes}", "routes logged"),
        ("Learning loop", f"{n_reviews}", "field reviews")]
st.html('<div class="flow">' + '<span class="arrow">→</span>'.join(
    f'<div class="step"><div class="k">{html.escape(k)}</div><div class="v">{html.escape(v)}</div>'
    f'<div class="s">{html.escape(s)}</div></div>' for k, v, s in flow) + "</div>")

t_feed, t_policy, t_extract, t_measure, t_decide, t_route, t_learn = st.tabs([
    ":material/storefront: Markets", ":material/policy: Venue policies", ":material/smart_toy: 1 · LLM reads",
    ":material/rule: 2 · Rules measure", ":material/how_to_reg: 3 · Human decides",
    ":material/alt_route: 4 · Router executes", ":material/school: Learning loop"])

FLAG_BADGE = {feed.AGREE: ":green-badge[fetcher = LLM]", feed.FETCHER_ONLY: ":gray-badge[fetcher only]",
              feed.LLM_FILLED: ":blue-badge[LLM filled]", feed.DISAGREE: ":orange-badge[disagree]",
              feed.POLICY_FILLED: ":violet-badge[LLM filled from venue policy]",
              feed.EMPTY: ":red-badge[empty]"}
VENUE_BADGE = {"kalshi": ":green-badge[KALSHI]", "polymarket": ":violet-badge[POLYMARKET]"}
shadows = pipeline.shadows_for(con, cards)
prices = feed.latest_prices(core)
now = datetime.now(timezone.utc)


def age(text):
    mins = int((now - values.parse_utc(text)).total_seconds() // 60)
    return f"{mins} min ago" if mins < 120 else f"{mins // 60} h ago"


def name(ref):
    return feed.label(by_ref[ref]) if ref in by_ref else ref


def show_card(s):
    c = by_ref[s["ref"]]
    q = feed.quote(prices.get((s["ref"], "yes")), prices.get((s["ref"], "no")))
    with st.container(border=True):
        st.markdown(f"{VENUE_BADGE.get(c.venue, c.venue)} &nbsp; **{feed.label(c)}**")
        st.caption(f"{c.title} · {c.market_id}")
        st.markdown(f"| | Ask | Bid* |\n|---|---|---|\n| **Yes** | {q.yes_ask or '-'} | {q.yes_bid or '-'} |\n"
                    f"| **No** | {q.no_ask or '-'} | {q.no_bid or '-'} |")
        st.caption(("priced " + age(q.priced_at) if q.priced_at else "no saved price")
                   + " · *bid implied as 1 - opposite ask")
        lines = []
        for f in config.FIELDS:
            flag = feed.field_flag(s, f)
            v = s["fields"][f]
            extra = f" (LLM read `{s['llm_fields'][f]}`)" if flag == feed.DISAGREE else ""
            if flag == feed.POLICY_FILLED:
                extra = f" ({s['provenance'][f].split(':', 1)[1]})"
            lines.append(f"`{f}` **{v if v is not None else '-'}** {FLAG_BADGE[flag]}{extra}")
        st.markdown("  \n".join(lines))
        with st.expander(f"Rules text and quotes ({s['model']}, policy {s.get('policy_version') or 'none'})"):
            st.write(c.rules_text)
            for f in config.FIELDS:
                if s["quotes"].get(f):
                    st.caption(f"{f}: “{s['quotes'][f]}”")
                if s["problems"].get(f):
                    st.caption(f":red[{f} emptied: {s['problems'][f]}]")


# ---- Markets ----------------------------------------------------------------------------------------
with t_feed:
    step_header("Markets", "The Kalshi and Polymarket Fed markets Equinox tracks, as cards: prices, the "
                "seven payoff fields, and where each field's value came from.",
                "Equinox's normalized markets and saved prices, refreshed continuously", "nothing; this is a view")
    st.caption("Flags: **fetcher = LLM** both read the same · **fetcher only** · **LLM filled** fetcher was empty · "
               "**LLM filled from venue policy** quoted from a codified clause · **disagree** fetcher value used · "
               "**empty** unknown, never matches.")
    mode = st.segmented_control("View", ["All markets", "Proposed pairs side by side"], default="All markets")
    if mode != "Proposed pairs side by side":
        g1, g2, g3 = st.columns([1, 2, 2])
        venues = g1.multiselect("Venue", sorted({s["ref"].split(":")[0] for s in shadows}),
                                default=sorted({s["ref"].split(":")[0] for s in shadows}))
        flags = g2.multiselect("Only cards with a field flagged",
                               [feed.LLM_FILLED, feed.POLICY_FILLED, feed.DISAGREE, feed.EMPTY])
        text = g3.text_input("Search title or market id").lower()
        sel = [s for s in shadows if s["ref"].split(":")[0] in venues
               and (not flags or any(feed.field_flag(s, f) in flags for f in config.FIELDS))
               and (text in s["title"].lower() or text in s["ref"].lower())]
        per_page = 24
        pages = max(1, -(-len(sel) // per_page))
        page = st.number_input(f"Page (of {pages}, {len(sel)} cards)", 1, pages, 1)
        grid = st.columns(3)
        for i, s in enumerate(sel[(page - 1) * per_page: page * per_page]):
            with grid[i % 3]:
                show_card(s)
    else:
        if not proposals:
            st.info("No proposed pairs yet.")
        for pr in proposals[:30]:
            st.markdown(f"**{pr.tier.replace('_', ' ')}** · proposed **{pr.relation or '?'}** · "
                        f"pattern `{pr.pattern}` · {pr.days_to_close} days to close")
            left, right = st.columns(2)
            with left:
                show_card(pr.a)
            with right:
                show_card(pr.b)

# ---- Venue policies -----------------------------------------------------------------------------
with t_policy:
    step_header("Venue policies", "The venues' published terms, snapshotted daily at 00:15. A changed document or a "
                "codified clause that disappears issues a new policy version and is flagged here.",
                "Kalshi contract terms per series, the Polymarket US rulebook",
                "a policy version, plus the codified clauses the model may quote")
    checks = pd.read_sql("SELECT checked_at, result, version, detail FROM policy_checks ORDER BY id DESC LIMIT 30", con)
    versions = pd.read_sql("SELECT version, created_at, changed_json, missing_json FROM policy_versions ORDER BY id DESC", con)
    missing = set(json.loads(versions.iloc[0]["missing_json"])) if len(versions) else set()
    if len(checks):
        st.html(pill(f"version {policy_version}", "info") + pill(f"last check: {checks.iloc[0]['result']}",
                CHECK.get(checks.iloc[0]["result"], "muted"))
                + pill(f"{len(policy.clauses()) - len(missing)}/{len(policy.clauses())} clauses still in source",
                       "good" if not missing else "bad")
                + pill(f"checked {checks.iloc[0]['checked_at'][:16].replace('T', ' ')} UTC", "muted")
                + pill("daily refresh 00:15", "muted"))
    if st.button("Refresh policies now", icon=":material/refresh:"):
        r = policy.refresh(con)
        (st.success if r["result"] != "failed" else st.error)(f"{r['result']}: version {r['version']}")
        st.rerun()
    flagged = checks[checks["result"] != "unchanged"]
    if len(flagged) > 1 or (len(flagged) == 1 and len(versions) > 1):
        st.warning(f"{len(flagged)} flagged checks in the history below (new versions or failed fetches).")

    t_a, t_b, t_c, t_d = st.tabs(["Clauses", "What changed", "Check history", "Documents"])
    with t_a:
        st.dataframe(pd.DataFrame([{"id": c["id"], "source": c["source"], "field": c["field"], "value": c.get("value"),
                                    "still in source": c["id"] not in missing, "text": c["text"]}
                                   for c in policy.clauses()]), hide_index=True, width="stretch")
    with t_b:
        st.dataframe(versions, hide_index=True, width="stretch")
        if len(versions) > 0:
            v = st.selectbox("Version", versions["version"].tolist())
            src = st.selectbox("Document", [s_["id"] for s_ in policy.sources()])
            st.code(policy.diff(con, v, src) or "No change to this document in this version.", language="diff")
    with t_c:
        st.dataframe(checks, hide_index=True, width="stretch")
        used = pd.read_sql("SELECT policy_version, COUNT(*) AS decisions FROM decisions GROUP BY policy_version", con)
        st.caption("Decisions by the policy version they were made under (blank = made before versions existed).")
        st.dataframe(used, hide_index=True)
    with t_d:
        st.dataframe(pd.DataFrame([{"id": s_["id"], "venue": s_["venue"], "series": ", ".join(s_["series"]),
                                    "title": s_["title"], "url": s_["url"]} for s_ in policy.sources()]),
                     hide_index=True, width="stretch")

# ---- 1. LLM reads -------------------------------------------------------------------------------
with t_extract:
    step_header("1 · LLM reads", "The LLM reads each card's rules text, plus the venue clauses that apply, and fills the "
                "seven payoff fields. Every value needs an exact quote; code checks it and empties anything that fails.",
                "a card's rules text + its venue's codified clauses", "a shadow card per market, stamped with model and policy version")
    has_key = bool(os.getenv("ANTHROPIC_API_KEY"))
    missing_n = len(cards) - n_shadow
    c1, c2 = st.columns([1, 3])
    limit = c1.number_input("Cards per run", 1, max(missing_n, 1), min(25, max(missing_n, 1)))
    if c2.button(f"Read {min(limit, missing_n)} of {missing_n} unread cards with {config.MODEL}",
                 icon=":material/smart_toy:", disabled=not has_key or missing_n == 0):
        bar = st.progress(0.0)
        res = pipeline.run_extraction(con, cards, extract.anthropic_llm(), limit=int(limit),
                                      on_progress=lambda i, n, ref: bar.progress(i / n, text=ref))
        st.success(f"Read {res['extracted']}, failed {len(res['failed'])}.")
        st.rerun()
    if not has_key:
        st.info("Set ANTHROPIC_API_KEY in prototype/.env to run extraction.")

    rows = []
    for s in shadows:
        if s["model"] == "core-copy":
            continue
        c = by_ref[s["ref"]]
        for f in config.FIELDS:
            rows.append({"market": feed.label(c), "venue": c.venue, "field": f, "flag": feed.field_flag(s, f),
                         "fetcher": s["core_fields"][f], "LLM": s["llm_fields"][f], "from": s["provenance"].get(f),
                         "quote": s["quotes"].get(f), "emptied because": s["problems"].get(f),
                         "policy": s.get("policy_version"), "id": c.market_id})
    if rows:
        df = pd.DataFrame(rows)
        st.caption("Flags across all LLM readings: "
                   + " · ".join(f"**{k}** {v}" for k, v in df["flag"].value_counts().items()))
        only_attention = st.toggle("Only fields that need a look (LLM filled, from policy, disagree, or emptied by code)", True)
        if only_attention:
            df = df[df["flag"].isin([feed.LLM_FILLED, feed.POLICY_FILLED, feed.DISAGREE]) | df["emptied because"].notna()]
        st.dataframe(df, hide_index=True, width="stretch")

# ---- 2. Rules measure ---------------------------------------------------------------------------
with t_measure:
    step_header("2 · Rules measure", "Every Kalshi card is compared with every Polymarket card, deterministically, no LLM. "
                "Each difference is named. Any hard difference drops the pair; otherwise 0 soft = strong, 1-2 = near "
                f"miss, 3+ = dropped. Only pairs with {config.MIN_DAYS_TO_CLOSE}+ days to the earlier close.",
                "shadow cards (fetcher values win, the LLM fills gaps)", "proposed pairs, grouped by blocking pattern")
    f1, f2, f3, f4 = st.columns(4)
    f1.metric("Pairs checked", f"{counts['pairs checked']:,}")
    f2.metric("Dropped", f"{counts['dropped']:,}")
    f3.metric("Near miss", counts["near_miss"])
    f4.metric("Strong", counts["strong"])
    if counts.get("closes within 7 days or unknown"):
        st.caption(f"{counts['closes within 7 days or unknown']} pairs skipped: close within 7 days or unknown.")
    pats = sorted({part for p in proposals for part in p.soft})
    if pats:
        st.markdown("**What the differences mean**")
        st.html("".join(f'<div class="row">{pill(x, "warn")}<div class="why">{html.escape(compare.EXPLAIN.get(x, ""))}</div></div>'
                        for x in pats))
    if proposals:
        st.markdown("**Proposed pairs**")
        rows_html = []
        for p in proposals:
            d = decisions.get(p.key)
            rows_html.append(
                f'<div class="row"><div class="names">{html.escape(name(p.a["ref"]))} &nbsp;↔&nbsp; '
                f'{html.escape(name(p.b["ref"]))}</div>'
                + pill(p.tier.replace("_", " "), TIER[p.tier])
                + pill(f"proposed {p.relation or '?'}", RELATION.get(p.relation, "muted"))
                + pattern_pills(p) + pill(f"{p.days_to_close} days to close", "muted")
                + decision_pill(d, d is not None and store.is_stale(d, latest_ids)) + "</div>")
        st.html("".join(rows_html))

# ---- 3. Human decides ---------------------------------------------------------------------------
with t_decide:
    step_header("3 · Human decides", "Pick a pair, read both rules texts, mark what the LLM read, and give a verdict with "
                "a reason. A verdict can be applied to the whole pattern group. Every decision records the policy version.",
                "one proposed pair", "a decision (same / opposite / reject) tied to the card content and policy version")
    if not proposals:
        st.info("No proposals yet. Read cards for both venues first.")
    else:
        cats = sorted({by_ref[r].category for p in proposals for r in p.key})
        f1, f2, f3 = st.columns(3)
        cat = f1.multiselect("Category", cats, default=cats)
        tier = f2.multiselect("Tier", ["strong", "near_miss"], default=["strong", "near_miss"])
        hide_done = f3.toggle("Hide decided pairs", True)

        def visible(p):
            d = decisions.get(p.key)
            done = d is not None and not store.is_stale(d, latest_ids)
            in_cat = by_ref[p.a["ref"]].category in cat or by_ref[p.b["ref"]].category in cat
            return p.tier in tier and in_cat and not (hide_done and done)

        shown = [p for p in proposals if visible(p)]
        st.progress(n_done / len(proposals), text=f"{n_done} of {len(proposals)} pairs decided · "
                    f"showing {len(shown)}" + (" (decided pairs hidden)" if hide_done else ""))
        groups: dict[str, list] = {}
        for p in shown:
            groups.setdefault(p.pattern, []).append(p)
        if not groups:
            st.success("Nothing left to review with these filters.")
        else:
            pattern = st.selectbox("Blocking pattern", list(groups), format_func=lambda g: f"{g}  ({len(groups[g])} pairs)")
            for part in pattern.split(" + "):
                if part in compare.EXPLAIN:
                    st.caption(f"**{part}**: {compare.EXPLAIN[part]}")
            group = groups[pattern]
            idx = st.selectbox("Pair", range(len(group)),
                               format_func=lambda i: f"{name(group[i].a['ref'])}  ↔  {name(group[i].b['ref'])}"
                                                     f"  ({group[i].relation or '?'})")
            p = group[idx]
            prior = decisions.get(p.key)
            st.html(pill(p.tier.replace("_", " "), TIER[p.tier]) + pill(f"proposed {p.relation or '?'}",
                    RELATION.get(p.relation, "muted")) + pattern_pills(p)
                    + decision_pill(prior, prior is not None and store.is_stale(prior, latest_ids))
                    + pill(f"policy {policy_version}", "info"))
            if prior is not None:
                state = "STALE (a card changed since)" if store.is_stale(prior, latest_ids) else "current"
                pv = prior["policy_version"] or "not recorded"
                moved = f" Policy has moved on to {policy_version}." if prior["policy_version"] != policy_version else ""
                st.warning(f"Previous decision: {prior['verdict']} by {prior['reviewer']} ({state}), under policy "
                           f"version {pv}.{moved} Reason: {prior['reason']}")

            ca, cb = st.columns(2)
            for col, s in ((ca, p.a), (cb, p.b)):
                c = by_ref[s["ref"]]
                with col.container(border=True):
                    st.markdown(f"{VENUE_BADGE.get(c.venue, c.venue)} **{feed.label(c)}**")
                    st.caption(f"{c.title} · {c.market_id}")
                    st.text_area("Rules text", c.rules_text, height=150, disabled=True, key=f"rt-{c.ref}-{idx}")

            def src(s, f):
                if f in s["disagree"]:
                    return f"fetcher (LLM read {s['llm_fields'][f]})"
                where = s["provenance"].get(f, "")
                if s["source"][f] == "llm" and where.startswith("policy:"):
                    return f"venue policy {where.split(':', 1)[1]}"
                return s["source"][f] or "not stated"

            grid = pd.DataFrame([{
                "field": f,
                "Kalshi value": p.a["fields"][f], "Kalshi from": src(p.a, f), "Kalshi quote": p.a["quotes"].get(f),
                "Kalshi correct": True, "Kalshi corrected": "",
                "Poly value": p.b["fields"][f], "Poly from": src(p.b, f), "Poly quote": p.b["quotes"].get(f),
                "Poly correct": True, "Poly corrected": "",
            } for f in config.FIELDS])
            with st.form(f"decide-{p.key}"):
                st.caption("Fetcher values win; the LLM fills gaps. Mark the LLM-filled and empty fields: untick "
                           "'correct' and type the right value (blank = not stated). Marks on fetcher values are not saved.")
                edited = st.data_editor(grid, hide_index=True, width="stretch",
                                        disabled=["field", "Kalshi value", "Kalshi from", "Kalshi quote",
                                                  "Poly value", "Poly from", "Poly quote"])
                options = ["same", "opposite", "reject"]
                verdict = st.radio("Verdict", options, horizontal=True,
                                   index=options.index(p.relation) if p.relation else 2)
                reason = st.text_input("Reason (required)")
                reviewer = st.text_input("Reviewer", value=st.session_state.get("reviewer", ""))
                to_group = st.checkbox(f"Apply this verdict and reason to all {len(group)} pairs in this pattern group "
                                       "(field marks are saved for this pair only)")
                st.caption(f"Recorded under venue policy version **{policy_version}**.")
                if st.form_submit_button("Save decision", icon=":material/check:", type="primary"):
                    if not reason.strip() or not reviewer.strip():
                        st.error("A reason and a reviewer name are required on every decision.")
                    else:
                        st.session_state["reviewer"] = reviewer
                        marks = []
                        for _, row in edited.iterrows():
                            for side, col in (("a", "Kalshi"), ("b", "Poly")):
                                if (p.a if side == "a" else p.b)["source"][row["field"]] == "fetcher":
                                    continue   # reviews grade the LLM; fetcher values belong to the core
                                corrected = row[f"{col} corrected"]
                                corrected = str(corrected).strip() if corrected not in (None, "") and pd.notna(corrected) else None
                                marks.append((side, row["field"], bool(row[f"{col} correct"]), corrected))
                        store.record_decision(con, p, verdict, reason, reviewer, marks, policy_version=policy_version)
                        if to_group:
                            for other in group:
                                if other is not p:
                                    store.record_decision(con, other, verdict, reason, reviewer, applied_to_group=True,
                                                          policy_version=policy_version)
                        st.rerun()

# ---- 4. Router executes -------------------------------------------------------------------------
with t_route:
    step_header("4 · Router executes", "For each approved pair, the router answers one question: to buy this position, "
                "which venue is cheaper, and why. Same rules as Equinox, applied in order.",
                "approved pairs + the latest saved prices", "a route per pair (buy on a venue, or no route), logged with every input")
    st.html(f"""<div class="note"><b>Why {REF} is the reference venue.</b> It is a role, not a preference.
      (1) You state the side you want relative to the {REF} card, as in the build requirements: "the requested side,
      yes or no, stated relative to the Kalshi card". (2) On an exact cost tie the reference venue wins, so the choice
      never flips between runs (ADR-003). It never beats a cheaper price. The role is set in configuration; the router
      code names no venue.</div>""")
    approved = pipeline.approved_pairs(con, core)
    approved_keys = {(d["ref_a"], d["ref_b"]) for d in approved}
    waiting = [p for p in proposals if p.key not in approved_keys
               and not (p.key in decisions and not store.is_stale(decisions[p.key], latest_ids))]
    side = st.segmented_control(f"Side you want to buy (relative to the {REF} card)", ["yes", "no"], default="yes") or "yes"
    logged = {d["id"]: store.logged_route(con, d["id"], side) for d in approved}
    pending = [d for d in approved if not logged[d["id"]]]
    st.html(pill(f"{len(approved)} approved", "good") + pill(f"{len(approved) - len(pending)} logged for {side.upper()}", "info")
            + pill(f"{len(pending)} ready to log", "warn" if pending else "muted")
            + pill(f"{len(waiting)} waiting for a human decision", "muted"))
    if not approved:
        st.info("No approved pairs yet. Approve a pair in 3 · Human decides.")
    else:
        st.caption("Each pair is routed once per side. A logged route is final and shows what was recorded; "
                   "a pair not yet logged shows a live preview from the latest saved prices.")
        st.html("".join(route_card(d, *frozen(logged[d["id"]]), logged=logged[d["id"]], name=name) if logged[d["id"]]
                        else route_card(d, *pipeline.preview_route(core, d, side), name=name) for d in approved))
        if pending:
            if st.button(f"Log {len(pending)} route" + ("s" if len(pending) != 1 else "") + f" for {side.upper()}",
                         icon=":material/alt_route:", type="primary"):
                pipeline.route_approved(con, core, side)
                st.rerun()
        else:
            st.success(f"Every approved pair is logged for {side.upper()}. Approve more pairs in 3 · Human decides.",
                       icon=":material/check_circle:")
    if waiting:
        st.markdown(f"**Waiting for a human decision ({len(waiting)})**: these reach the router only after approval in "
                    "3 · Human decides.")
        for i, p in enumerate(waiting):
            with st.container(border=True):
                left, right = st.columns([5, 1], vertical_alignment="center")
                left.html(f'<div class="names">{html.escape(name(p.a["ref"]))} &nbsp;↔&nbsp; {html.escape(name(p.b["ref"]))}'
                          f'</div>' + pill("to review", "muted") + pill(f"proposed {p.relation or '?'}",
                                                                        RELATION.get(p.relation, "muted")) + pattern_pills(p))
                if right.button("Decide", key=f"decide-{i}", icon=":material/arrow_forward:", type="primary"):
                    st.session_state["guided_pair"] = p.key
                    st.session_state["guided_step"] = 2
                    st.switch_page("views/guided.py")

    log = pd.read_sql("SELECT decided_at, ref_a, ref_b, relation, side_a, side_b, action, price_a, price_b, fee_applied,"
                      " as_of, policy_version, decision_policy_version, reason FROM route_log ORDER BY id DESC LIMIT 200", con)
    if len(log):
        log.insert(1, CTR, log["ref_b"].map(name))
        log.insert(1, REF, log["ref_a"].map(name))
        st.markdown("**Route log** (append-only: every input, the policy version in force, and the one the decision was made under)")
        st.dataframe(log.drop(columns=["ref_a", "ref_b"]), hide_index=True, width="stretch")

# ---- Learning loop ------------------------------------------------------------------------------
with t_learn:
    step_header("Learning loop", "Reviewed fields become few-shot examples for the next reading and the eval set that "
                "scores a prompt change. Learning never removes the human gate.",
                "your decisions and field marks", "examples in the next prompt, and accuracy numbers")
    m = evals.review_metrics(con)
    c1, c2 = st.columns(2)
    c1.metric("Field accuracy (as reviewed)", evals.pct(m["field_accuracy"]))
    c2.metric("Proposal precision", evals.pct(m["proposal_precision"]))
    if m["per_pattern"]:
        st.dataframe(pd.DataFrame([{"pattern": k, "approved": v[0], "decided": v[1]} for k, v in m["per_pattern"].items()]),
                     hide_index=True)
    corrections = pd.read_sql("SELECT s.ref, r.field, r.extracted_value, r.corrected_value FROM field_reviews r "
                              "JOIN shadow_cards s ON s.id = r.shadow_id WHERE r.correct = 0 ORDER BY r.id DESC", con)
    st.markdown("**Corrections**")
    st.caption("The closest reviewed cards are added as examples on the next reading "
               "(`python -m proto extract --refresh`); `python -m proto eval --rerun` scores the prompt against them.")
    st.dataframe(corrections, hide_index=True, width="stretch")
    st.caption("Talking point, not built: a correction approved many times can graduate into a deterministic "
               "fetcher rule in the core.")
