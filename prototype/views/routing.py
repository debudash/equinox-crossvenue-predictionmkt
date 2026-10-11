"""Route card: one routing decision explained as the five rules the router applies, in order.
Shared by the routing tab and the guided test."""

import html

from proto import config, core_db, route_sim, values
from views.ui import RELATION, pill

VENUE = {config.REFERENCE_VENUE: config.REFERENCE_VENUE.capitalize(), config.COUNTER_VENUE: config.COUNTER_VENUE.capitalize()}
REF, CTR = VENUE[config.REFERENCE_VENUE], VENUE[config.COUNTER_VENUE]


def hhmmss(text):
    return text[11:19] + " UTC" if text else "-"


def route_card(d, r, pa, pb, logged=None, name=lambda ref: ref) -> str:
    """One routing decision, explained as the five rules the router applies, in order."""
    li = []

    def step(state, title, body):
        icon = {"ok": "✓", "fail": "✗", "skip": "·"}[state]
        li.append(f'<li class="{state}"><span class="ic">{icon}</span><div><b>{html.escape(title)}</b>'
                  f'<div class="why">{body}</div></div></li>')

    step("ok", "Pair approved by a human",
         f"Approved as <b>{html.escape(d['verdict'])}</b> by {html.escape(d['reviewer'])}"
         f" (policy {html.escape(d['policy_version'] or 'not recorded')}): “{html.escape(d['reason'])}”")
    if d["verdict"] == "same":
        step("ok", "What is compared",
             f"Buy <b>{r.side_a.upper()}</b> on {REF} vs buy <b>{r.side_b.upper()}</b> on {CTR}: the two bets pay in exactly "
             "the same outcomes, so either buy gives the same position.")
    else:
        step("ok", "What is compared",
             f"Buy <b>{r.side_a.upper()}</b> on {REF} vs buy <b>{r.side_b.upper()}</b> on {CTR}: the bets are opposites, "
             f"so {r.side_a.upper()} on one pays exactly when {r.side_b.upper()} on the other pays.")

    reached = r.action != "no_route"
    gone = "not recorded (logged before routes kept refused inputs)" if logged else "missing"
    describe = lambda venue, p, side: (f"{venue} {side.upper()} ask <b>{p.buy_price}</b>, saved {hhmmss(p.priced_at)}"
                                       if p and p.buy_price else f"{venue} {side.upper()} ask <b>{gone if p is None else 'missing'}</b>")
    prices_txt = f"{describe(REF, pa, r.side_a)} · {describe(CTR, pb, r.side_b or r.side_a)}."
    if reached:
        step("ok", "Both prices usable",
             prices_txt + f" Judged at as_of {hhmmss(r.as_of)} (the later of the two); both are within the "
             f"{config.PRICE_MAX_AGE_SECONDS // 60}-minute limit.")
    else:
        step("fail", "A price is not usable, so no route", prices_txt + " " + html.escape(r.reason.replace("No route: ", "")))
        step("skip", "Fees", "Not reached.")
        step("skip", "Lower cost wins", "Not reached.")
        return _card(d, r, li, logged, name)

    if r.fee_applied == "yes":
        step("ok", "Fees known on both venues, so they count",
             f"{REF} fee {r.fee_a} · {CTR} fee {r.fee_b}, added to each buy price.")
    elif r.fee_a is None and r.fee_b is None:
        step("ok", "Fees unknown on both venues, so buy prices are compared alone",
             "Like for like: neither side carries a fee. Equinox stores a fee only when it can be derived exactly per "
             "contract; for these markets neither venue's fee can be, so both are empty.")
    else:
        known = REF if r.fee_a is not None else CTR
        step("ok", f"Fee known on {known} only, so no fee is applied",
             "Adding a fee to one side only would make the comparison unfair, so both are compared on buy price.")

    cost_a = values.micros(r.price_a) + (values.micros(r.fee_a) if r.fee_applied == "yes" else 0)
    cost_b = values.micros(r.price_b) + (values.micros(r.fee_b) if r.fee_applied == "yes" else 0)
    win_ref = r.action == f"buy_on_{config.REFERENCE_VENUE}"
    bars = "".join(
        f'<div class="bar"><span class="lbl">{v}</span><span class="track"><span class="fill {"win" if w else ""}" '
        f'style="width:{max(2, c * 100 // 1_000_000)}%"></span></span><span class="amt">{values.micros_str(c)}</span></div>'
        for v, c, w in ((REF, cost_a, win_ref), (CTR, cost_b, not win_ref)))
    if cost_a == cost_b:
        verdict = (f"Exact tie at {values.micros_str(cost_a)}: <b>{REF} wins as the reference venue</b>, "
                   "so the choice never flips between runs.")
    else:
        winner, gap = (REF, cost_b - cost_a) if cost_a < cost_b else (CTR, cost_a - cost_b)
        verdict = f"<b>{winner}</b> is cheaper by <b>{values.micros_str(gap)}</b> per contract."
    step("ok", "Lower cost wins", bars + verdict)
    return _card(d, r, li, logged, name)


def _card(d, r, li, logged=None, name=lambda ref: ref) -> str:
    if r.action == "no_route":
        head = pill("No route", "bad")
    else:
        venue = r.action.removeprefix("buy_on_")
        side = r.side_a if venue == config.REFERENCE_VENUE else r.side_b
        head = pill(f"Buy {side.upper()} on {VENUE.get(venue, venue)}", "good")
    status = (pill(f"logged {logged['decided_at'][:16].replace('T', ' ')} UTC · final", "info") if logged
              else pill("preview · not logged yet", "muted"))
    return (f'<div class="rcard"><div class="names">{html.escape(name(r.ref_a))} &nbsp;↔&nbsp; '
            f'{html.escape(name(r.ref_b))}</div>{head}{pill(d["verdict"], RELATION.get(d["verdict"], "muted"))}'
            f'{status}<ol class="trace">{"".join(li)}</ol></div>')


def frozen(row):
    """A logged route rebuilt from its log row, so the card shows exactly what was recorded."""
    r = route_sim.Route(**{k: row[k] for k in ("relation", "ref_a", "ref_b", "side_a", "side_b", "price_a", "price_b",
                                                "fee_a", "fee_b", "priced_at_a", "priced_at_b", "fee_applied", "action",
                                                "reason", "as_of")})
    price = lambda ref, side, p, fee, at: core_db.Price(*ref.split(":", 1), side, p, fee, at) if at else None
    return (r, price(r.ref_a, r.side_a, r.price_a, r.fee_a, r.priced_at_a),
            price(r.ref_b, r.side_b, r.price_b, r.fee_b, r.priced_at_b))
