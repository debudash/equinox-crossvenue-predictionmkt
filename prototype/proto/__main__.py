"""CLI: python -m proto extract | propose | route | eval | walkthrough | policies   (run from prototype/)"""

import argparse
from datetime import datetime, timezone

from . import core_db, evals, extract, pipeline, policy, store


def main() -> None:
    ap = argparse.ArgumentParser(prog="proto", description="LLM proposes, rules measure, human decides, router executes.")
    sub = ap.add_subparsers(dest="cmd", required=True)
    e = sub.add_parser("extract", help="build shadow cards with the LLM (cached per card version)")
    e.add_argument("--limit", type=int)
    e.add_argument("--refresh", action="store_true", help="re-extract even when cached (uses latest reviewed examples)")
    e.add_argument("--core-copy", action="store_true", help="no LLM: copy the fetcher's fields as a baseline")
    e.add_argument("--match", help="only cards whose market id contains this text (e.g. KXFEDFUNDSYEAR or rvcus)")
    p = sub.add_parser("propose", help="deterministic deviation check over shadow-card pairs")
    p.add_argument("--as-of", help="RFC 3339; default now")
    r = sub.add_parser("route", help="simulate routing for approved, non-stale pairs")
    r.add_argument("--side", choices=["yes", "no"], required=True)
    v = sub.add_parser("eval", help="field accuracy and proposal precision from reviews")
    v.add_argument("--rerun", action="store_true", help="re-extract reviewed cards and score against gold (calls the LLM)")
    sub.add_parser("walkthrough", help="one pair through every step, on a scratch copy of the database")
    pol = sub.add_parser("policies", help="venue policy snapshots and versions")
    pol.add_argument("action", choices=["refresh", "status", "schedule"])
    a = ap.parse_args()
    if a.cmd == "walkthrough":
        from . import walkthrough
        return walkthrough.run()

    core, con = pipeline.open_stores()
    cards = core_db.scope_cards(core)
    if getattr(a, "match", None):
        cards = [c for c in cards if a.match in c.market_id]

    if a.cmd == "policies":
        if a.action == "schedule":
            if policy.current_version(con) is None:      # a fresh volume gets its first version at once
                print(policy.refresh(con), flush=True)
            return policy.schedule(lambda: pipeline.open_stores()[1])
        if a.action == "refresh":
            r = policy.refresh(con)
            print(f"{store.now()} policies: {r['result']} -> version {r['version']}")
            for k in ("changed", "missing", "failed"):
                if r.get(k):
                    print(f"  {k}: {r[k]}")
        for row in con.execute("SELECT * FROM policy_checks ORDER BY id DESC LIMIT 5"):
            print(f"  check {row['checked_at']}  {row['result']:12} version {row['version']}")
        return

    if a.cmd == "extract" and a.core_copy:
        todo = [c for c in cards if a.refresh or store.latest_shadow(con, c) is None][:a.limit]
        for c in todo:
            extract.copy_core_card(con, c)
        print(f"copied {len(todo)} core cards as baseline shadow cards (model core-copy)")

    elif a.cmd == "extract":
        res = pipeline.run_extraction(con, cards, extract.anthropic_llm(), a.refresh, a.limit,
                                      lambda i, n, ref: print(f"[{i}/{n}] {ref}"))
        print(f"extracted {res['extracted']}, cached {res['cached']}, failed {len(res['failed'])}")
        for ref, why in res["failed"]:
            print(f"  failed {ref}: {why}")

    elif a.cmd == "propose":
        as_of = datetime.fromisoformat(a.as_of.replace("Z", "+00:00")) if a.as_of else datetime.now(timezone.utc)
        props, counts = pipeline.build_proposals(con, cards, as_of)
        for k, n in counts.items():
            print(f"{k:>34}: {n}")
        for pr in props[:40]:
            print(f"{pr.tier:9} {pr.relation or '?':8} {pr.a['ref']:45} {pr.b['ref']:50} {pr.pattern}")

    elif a.cmd == "route":
        for rt in pipeline.route_approved(con, core, a.side):
            print(f"{rt.action:20} {rt.ref_a} / {rt.ref_b}\n  {rt.reason}")

    elif a.cmd == "eval":
        m = evals.review_metrics(con)
        print("field accuracy (as reviewed):", evals.pct(m["field_accuracy"]))
        for f, pair in sorted(m["per_field"].items()):
            print(f"  {f:16} {evals.pct(pair)}")
        print("proposal precision:", evals.pct(m["proposal_precision"]))
        for pat, pair in sorted(m["per_pattern"].items()):
            print(f"  {pat:50} {evals.pct(pair)}")
        if a.rerun:
            rr = evals.rerun(con, pipeline.reviewed_examples(con, cards), extract.anthropic_llm())
            print("rerun accuracy vs gold:", evals.pct(rr["accuracy"]))
            for ref, f, got, want in rr["misses"]:
                print(f"  miss {ref} {f}: got {got!r}, gold {want!r}")


if __name__ == "__main__":
    main()
