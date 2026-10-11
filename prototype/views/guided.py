"""Guided test: one pair through the whole proposal flow, one screen per step, with real actions.
Pick a pair -> what the LLM read -> what the rules found -> decide -> route -> done."""

import html

import streamlit as st

from proto import compare, config, core_db, feed, pipeline, policy, store
from views.routing import REF, frozen, route_card
from views.ui import RELATION, TIER, pattern_pills, pill, step_header

STEPS = ["Pick a pair", "What the LLM read", "What the rules found", "Decide", "Route", "Done"]
ss = st.session_state
ss.setdefault("guided_step", 1)

core, con = pipeline.open_stores()
cards = core_db.scope_cards(core)
by_ref = {c.ref: c for c in cards}
policy_version = policy.current_version(con)
proposals, _ = pipeline.build_proposals(con, cards)
by_key = {p.key: p for p in proposals}
decisions = store.latest_decisions(con)
current = pipeline.current_fingerprints(cards)
name = lambda ref: feed.label(by_ref[ref]) if ref in by_ref else ref


def go(step: int) -> None:
    ss["guided_step"] = step
    st.rerun()


def nav(back: int | None, nxt: int | None, next_label: str = "Next", disabled: bool = False) -> None:
    left, _, right = st.columns([1, 4, 1])
    if back and left.button("Back", icon=":material/arrow_back:", key=f"back-{ss['guided_step']}", width="stretch"):
        go(back)
    if nxt and right.button(next_label, icon=":material/arrow_forward:", type="primary", disabled=disabled,
                            key=f"next-{ss['guided_step']}", width="stretch"):
        go(nxt)


st.title("Guided test")
step = ss["guided_step"]
pair = by_key.get(ss.get("guided_pair"))
if step > 1 and pair is None:          # the pair left the proposals (e.g. a card changed); start over
    step = ss["guided_step"] = 1

# Progress strip: where you are in the flow.
st.html('<div class="flow">' + '<span class="arrow">→</span>'.join(
    f'<div class="step" style="{"border-color:#2E5E8C;background:rgba(46,94,140,.22)" if i == step else ""}'
    f'{";opacity:.55" if i > step else ""}"><div class="k">Step {i}</div><div class="v" style="font-size:1rem">'
    f'{"✓ " if i < step else ""}{html.escape(t)}</div></div>' for i, t in enumerate(STEPS, 1)) + "</div>")
if pair is not None and step > 1:
    st.html(f'<div class="names" style="font-weight:700">{html.escape(name(pair.a["ref"]))} &nbsp;↔&nbsp; '
            f'{html.escape(name(pair.b["ref"]))}</div>')

# ---- 1. pick ------------------------------------------------------------------------------------------
if step == 1:
    step_header("Step 1 · Pick a pair", "These are the pairs the rules proposed: markets on the two venues that may be the "
                "same bet. Start with one that still needs a decision.", "the proposed pairs", "one pair to test")
    if not proposals:
        st.info("No proposals yet: the LLM has not read both venues' cards.")
    order = sorted(proposals, key=lambda p: (p.key in decisions and not store.is_stale(decisions[p.key], current), p.key))
    for i, p in enumerate(order):
        d = decisions.get(p.key)
        done = d is not None and not store.is_stale(d, current)
        with st.container(border=True):
            left, right = st.columns([5, 1], vertical_alignment="center")
            left.html(f'<div class="names">{html.escape(name(p.a["ref"]))} &nbsp;↔&nbsp; {html.escape(name(p.b["ref"]))}'
                      f'</div>' + pill(p.tier.replace("_", " "), TIER[p.tier])
                      + pill(f"proposed {p.relation or '?'}", RELATION.get(p.relation, "muted"))
                      + (pill(f"decided: {d['verdict']}", "good") if done else pill("needs a decision", "warn")))
            if right.button("Re-test" if done else "Start", key=f"pick-{i}", type="secondary" if done else "primary",
                            icon=":material/play_arrow:", width="stretch"):
                ss["guided_pair"], ss["guided_decision"] = p.key, None
                go(2)

# ---- 2. what the LLM read -------------------------------------------------------------------------------
elif step == 2:
    step_header("Step 2 · What the LLM read", "Each market's rules were read into seven payoff fields. The fetcher's value "
                "wins where it has one; the LLM fills gaps, quoting the market's rules or a codified venue policy clause.",
                "both markets' rules text + venue policy", "seven fields per market, each with where it came from")
    cols = st.columns(2)
    for col, s in zip(cols, (pair.a, pair.b)):
        c = by_ref[s["ref"]]
        with col.container(border=True):
            st.html(pill(c.venue.upper(), "good" if c.venue == "kalshi" else "violet")
                    + f'<div class="names" style="margin-top:6px">{html.escape(feed.label(c))}</div>'
                    f'<div class="why">{html.escape(c.title)}</div>')
            with st.expander("Rules text"):
                st.write(c.rules_text)
            rows = []
            for f in config.FIELDS:
                flag = feed.field_flag(s, f)
                kind = {"agree": "good", "fetcher only": "muted", "disagree": "warn", "empty": "bad"}.get(flag, "violet")
                src = s["provenance"].get(f, "")
                rows.append(f'<div class="row" style="padding:6px 10px"><b>{f}</b> &nbsp;'
                            f'<code>{html.escape(str(s["fields"][f]) if s["fields"][f] is not None else "–")}</code> '
                            f'{pill(flag, kind)}{pill(src.replace("policy:", "policy clause "), "info") if src.startswith("policy:") else ""}'
                            + (f'<div class="why">“{html.escape(s["quotes"][f])}”</div>' if s["quotes"].get(f) else "")
                            + "</div>")
            st.html("".join(rows))
    nav(1, 3)

# ---- 3. what the rules found -----------------------------------------------------------------------------
elif step == 3:
    step_header("Step 3 · What the rules found", "Deterministic rules compared the two cards field by field. Each "
                "difference is named. Any difference that changes the payoff would have dropped the pair; these are the "
                "ones a person can rule on.", "the two shadow cards", "a proposed relation and the differences to judge")
    st.html(pill(pair.tier.replace("_", " "), TIER[pair.tier]) + pill(f"proposed {pair.relation or '?'}",
            RELATION.get(pair.relation, "muted")) + pill(f"{pair.days_to_close} days to close", "muted"))
    for part in sorted(pair.soft):
        st.html(f'<div class="row">{pill(part, "warn")}<div class="why">{html.escape(compare.EXPLAIN.get(part, ""))}</div></div>')
    if not pair.soft:
        st.success("No differences: every field is stated and equal on both cards.")
    st.caption("Your call in the next step: do these differences change who gets paid? If not, approve the proposed relation.")
    nav(2, 4)

# ---- 4. decide -------------------------------------------------------------------------------------------
elif step == 4:
    step_header("Step 4 · Decide", "Your verdict, with a reason, recorded against this exact card content and the venue "
                "policy version in force. Approving sends the pair to the router; rejecting ends it here.",
                "your judgement", f"a decision under policy {policy_version}")
    choices = {"Approve: same bet": "same", "Approve: opposite bets": "opposite", "Reject": "reject"}
    default = next((k for k, v in choices.items() if v == pair.relation), "Reject")
    with st.form("guided-decide", border=True):
        verdict = st.pills("Verdict", list(choices), default=default)
        reason = st.text_area("Reason (required)", placeholder="e.g. Above 3.25% is the same as at or above 3.50% when "
                              "the rate moves in 0.25 steps; both venues keep discretion on an undeterminable outcome.")
        reviewer = st.text_input("Your name", value=ss.get("reviewer", ""))
        st.caption(f"Recorded under venue policy version {policy_version}.")
        save = st.form_submit_button("Save decision", icon=":material/check:", type="primary")
    if save:
        if not (verdict and reason.strip() and reviewer.strip()):
            st.error("Choose a verdict, and write a reason and your name.")
        else:
            ss["reviewer"] = reviewer
            ss["guided_decision"] = store.record_decision(con, pair, choices[verdict], reason.strip(), reviewer.strip(),
                                                          policy_version=policy_version)
            go(5 if choices[verdict] != "reject" else 6)
    if st.button("Back", icon=":material/arrow_back:"):
        go(3)

# ---- 5. route ----------------------------------------------------------------------------------------------
elif step == 5:
    step_header("Step 5 · Route", f"Pick the side you want to buy, stated against the {REF} card. The router compares the "
                "two venues' saved prices by Equinox's rules and picks the cheaper one, or refuses and says why.",
                "your approved pair + the latest saved prices", "a route, logged once per side")
    d = decisions.get(pair.key)
    if d is None or d["verdict"] == "reject" or store.is_stale(d, current):
        st.warning("This pair has no current approval. Decide it first.")
        nav(4, None)
    else:
        side = {"Buy YES": "yes", "Buy NO": "no"}[st.pills("Side", ["Buy YES", "Buy NO"], default="Buy YES",
                                                            key="guided_side") or "Buy YES"]
        logged = store.logged_route(con, d["id"], side)
        if logged:
            st.html(route_card(d, *frozen(logged), logged=logged, name=name))
            st.success(f"Logged for {side.upper()}. Try the other side, or finish.", icon=":material/check_circle:")
        else:
            st.html(route_card(d, *pipeline.preview_route(core, d, side), name=name))
        left, mid, right = st.columns([1, 3, 1])
        if left.button("Back", icon=":material/arrow_back:", width="stretch"):
            go(4)
        if not logged and mid.button(f"Log this route for {side.upper()}", icon=":material/alt_route:", type="primary"):
            pipeline.log_route(con, core, d, side)
            st.rerun()
        if right.button("Finish", icon=":material/arrow_forward:", type="primary" if logged else "secondary",
                        width="stretch"):
            go(6)

# ---- 6. done ------------------------------------------------------------------------------------------------
else:
    step_header("Step 6 · Done", "What this test recorded. Everything is append-only and traceable to the card content, "
                "the policy version and your reason.", "this pair", "a decision and its routes")
    d = decisions.get(pair.key)
    if d is not None:
        routes = [store.logged_route(con, d["id"], s) for s in ("yes", "no")]
        st.html(pill(f"decision: {d['verdict']}", "good" if d["verdict"] != "reject" else "bad")
                + pill(f"by {d['reviewer']}", "muted") + pill(f"policy {d['policy_version'] or 'not recorded'}", "info")
                + "".join(pill(f"{s.upper()}: {r['action'].replace('_', ' ')}", "good" if r["action"] != "no_route" else "bad")
                          for s, r in zip(("yes", "no"), routes) if r)
                + ("" if any(routes) or d["verdict"] == "reject" else pill("no route logged", "muted")))
        st.html(f'<div class="row"><div class="why">Reason: “{html.escape(d["reason"])}”</div></div>')
    left, right = st.columns(2)
    if left.button("Test the next pair", icon=":material/replay:", type="primary", width="stretch"):
        ss.pop("guided_pair", None)
        go(1)
    if right.button("See everything in the proposal tabs", icon=":material/dashboard:", width="stretch"):
        st.switch_page("views/proposal.py")
