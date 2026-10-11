"""One pair through every step, printed.  python -m proto walkthrough

Works on a scratch copy (data/proposals-walkthrough.db) so real review decisions stay clean.
LLM shadow cards are copied from data/proposals.db; the example pair is extracted live if missing.
"""

import shutil
import textwrap

from . import compare, config, core_db, evals, extract, feed, pipeline, route_sim, store
from .learn import pick_examples

PAIR = ("kalshi:KXFEDFUNDSYEAR-28JAN01-T3.75", "polymarket:rvcus-fedfnds-ondt-2027-12-31-gte4pt00")
TRAP = "kalshi:KXFEDFUNDSYEAR-28JAN01-T4.00"     # gt 4.00 vs gte 4.00: looks closest, is a different bet
SCRATCH = config.PROPOSALS_DB.with_name("proposals-walkthrough.db")


def step(n, title):
    print(f"\n{'=' * 100}\nSTEP {n}  {title}\n{'=' * 100}")


def show_card(s, price_y, price_n):
    q = feed.quote(price_y, price_n)
    print(f"\n  {s['ref']}   [{s['model']}]\n  {s['title']}")
    print(textwrap.indent(textwrap.fill(s["rules_text"], 96), "    | "))
    print(f"    prices  yes ask {q.yes_ask or '-'}  bid* {q.yes_bid or '-'}   no ask {q.no_ask or '-'}  bid* {q.no_bid or '-'}"
          f"   (saved {q.priced_at or 'never'}; *implied)")
    print(f"    {'field':15} {'fetcher':24} {'LLM':24} {'flag':14} quote")
    for f in config.FIELDS:
        print(f"    {f:15} {str(s['core_fields'][f]):24} {str(s['llm_fields'][f]):24} {feed.field_flag(s, f):14} "
              f"{s['quotes'].get(f) or ''}")
    for f, why in s["problems"].items():
        print(f"    emptied by code: {f}: {why}")


def run() -> None:
    if config.PROPOSALS_DB.exists():
        shutil.copyfile(config.PROPOSALS_DB, SCRATCH)
    core, con = pipeline.open_stores(con=store.connect(SCRATCH))
    cards = core_db.scope_cards(core)
    by_ref = {c.ref: c for c in cards}
    for c in cards:                                   # anything never read gets the fetcher baseline
        if store.latest_shadow(con, c) is None:
            extract.copy_core_card(con, c)

    step(1, "LLM proposes: seven payoff fields from rules text, every value backed by a verbatim quote")
    for ref in PAIR:
        if not pipeline.has_llm_shadow(con, by_ref[ref]):
            print(f"  extracting {ref} with {config.MODEL} ...")
            extract.extract_card(con, by_ref[ref], extract.anthropic_llm(), refresh=True)
    shadows = {s["ref"]: s for s in pipeline.shadows_for(con, cards)}
    prices = feed.latest_prices(core)
    for ref in PAIR:
        show_card(shadows[ref], prices.get((ref, "yes")), prices.get((ref, "no")))
    print("\n  The shadow card the next step sees: fetcher values win, the LLM fills only empty fields.")

    step(2, "Rules measure: deterministic deviation check over every Kalshi x Polymarket pair (no LLM)")
    props, counts = pipeline.build_proposals(con, cards)
    for k, n in counts.items():
        print(f"  {k:>34}: {n}")
    p = next(x for x in props if x.key == PAIR)
    print(f"\n  Example pair: tier {p.tier}, proposed relation {p.relation}, {p.days_to_close} days to close")
    for pat in p.soft:
        print(f"    soft  {pat}: {compare.EXPLAIN.get(pat, '')}")
    trap = compare.compare(shadows[TRAP], shadows[PAIR[1]])
    print(f"\n  Trap pair {TRAP} vs {PAIR[1]}:\n    gt 4.00 vs gte 4.00 differs by one field, but on the 25 bp grid it is "
          f"gte 4.25 vs gte 4.00 -> {trap.tier} ({', '.join(trap.hard)})")

    step(3, "Human decides: verdict, required reason, per-field marks on what the LLM filled or left empty")
    reason = "Kalshi above 3.75% is Polymarket 4.00% or above on the 25 bp grid; neither text states an unclear-result rule"
    did = store.record_decision(con, p, "same", reason, "walkthrough",
                                [("a", "unclear_result", True, None), ("b", "unclear_result", True, None)])
    d = store.latest_decisions(con)[PAIR]
    print(f"  decision #{did}: {d['verdict']}  reason: {d['reason']}")
    print(f"  covers card content {d['fingerprint_a']} and {d['fingerprint_b']}; stale now? "
          f"{store.is_stale(d, pipeline.current_fingerprints(cards))} "
          "(a change to rules text or payoff fields would make it stale; a volume change does not)")

    step(4, "Router executes: port of the core router, only approved non-stale pairs, every input logged")
    approved = [x for x in pipeline.approved_pairs(con, core) if (x["ref_a"], x["ref_b"]) == PAIR]
    for side in ("yes", "no"):
        side_b = side if d["verdict"] == "same" else ("no" if side == "yes" else "yes")
        pa, pb = core_db.latest_price(core, PAIR[0], side), core_db.latest_price(core, PAIR[1], side_b)
        as_of = route_sim.as_of_from_prices(pa, pb)
        r = route_sim.decide(d["verdict"], *PAIR, side, pa, pb, as_of)
        store.save_route(con, approved[0]["id"], r.__dict__)
        print(f"  side {side}: {r.action}\n    as_of {as_of}\n    {r.reason}")

    step(5, "Learning loop: the review becomes a few-shot example and part of the eval set")
    neighbour = by_ref[TRAP]
    reviewed = pipeline.reviewed_examples(con, cards)
    picked = pick_examples(neighbour, reviewed)
    print(f"  next extraction of {neighbour.ref} would include as examples: {[c.ref for c, _ in picked]}")
    m = evals.review_metrics(con)
    print(f"  field accuracy (as reviewed): {evals.pct(m['field_accuracy'])}   "
          f"proposal precision: {evals.pct(m['proposal_precision'])}")
    print(f"\n  Scratch database: {SCRATCH}\n  Same flow on screen: streamlit run app.py (Feed, then tabs 1 to 4).")
