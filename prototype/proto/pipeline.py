"""Glue for the four steps, shared by the CLI and the Streamlit screen."""

from datetime import datetime, timezone

from . import compare, config, core_db, extract, policy, route_sim, store, values


def open_stores(core=None, con=None):
    """Connect both databases and migrate older proposal rows to content fingerprints."""
    core, con = core or core_db.connect(), con or store.connect()
    store.backfill_fingerprints(con, lambda cid: core_db.fingerprint_of(core, cid))
    policy.ensure(con)
    return core, con


def current_fingerprints(cards) -> dict[str, str]:
    return {c.ref: c.fingerprint for c in cards}


def reviewed_examples(con, cards) -> list[tuple[core_db.CoreCard, dict]]:
    """Every card a human has reviewed, with its gold values, ready to be a few-shot example."""
    by_fp = {c.fingerprint: c for c in cards}
    return [(by_fp[fp], gold) for fp, gold in store.gold_by_fingerprint(con).items() if fp in by_fp]


def has_llm_shadow(con, card) -> bool:
    """Read by the configured model on the current policy version. A core-copy baseline, another model's
    reading, or a reading grounded on an older policy version does not count."""
    return store.latest_shadow(con, card, config.MODEL, policy.current_version(con)) is not None


def run_extraction(con, cards, llm, refresh=False, limit=None, on_progress=None) -> dict:
    """Extract every in-scope card that has no shadow card yet (or all, with refresh)."""
    reviewed = reviewed_examples(con, cards)
    pending = [c for c in cards if refresh or not has_llm_shadow(con, c)]
    todo = pending[:limit]
    done, failed = 0, []
    for i, card in enumerate(todo, 1):
        try:
            extract.extract_card(con, card, llm, reviewed, refresh=True)
            done += 1
        except Exception as e:  # one bad card never stops the run, same rule as the core (hard rule 8)
            failed.append((card.ref, str(e)))
        if on_progress:
            on_progress(i, len(todo), card.ref)
    return {"extracted": done, "failed": failed, "cached": len(cards) - len(pending), "left": len(pending) - len(todo)}


def merge(core_fields: dict, llm_fields: dict) -> tuple[dict, dict, list[str]]:
    """Fetcher wins, the LLM fills gaps. Returns (fields, source per field, fields where both set and disagree).
    The raw LLM output stays stored unchanged; this merge only shapes what the deviation check sees."""
    fields, source, disagree = {}, {}, []
    for f in config.FIELDS:
        core_v, llm_v = core_fields[f], llm_fields.get(f)
        fields[f] = core_v if core_v is not None else llm_v
        source[f] = "fetcher" if core_v is not None else ("llm" if llm_v is not None else None)
        if core_v is not None and llm_v is not None and core_v != llm_v:
            disagree.append(f)
    return fields, source, disagree


def shadows_for(con, cards) -> list[dict]:
    out = []
    for c in cards:
        # The configured model's reading when there is one, else the latest reading of any kind (shown by name).
        pv = policy.current_version(con)
        if row := (store.latest_shadow(con, c, config.MODEL, pv) or store.latest_shadow(con, c, config.MODEL)
                   or store.latest_shadow(con, c)):
            s = store.shadow_dict(row)
            fields, source, disagree = merge(c.core_fields, s["fields"])
            out.append(s | {"llm_fields": s["fields"], "fields": fields, "source": source, "disagree": disagree,
                            "title": c.title, "rules_text": c.rules_text, "core_fields": c.core_fields,
                            "event_title": c.event_title})
    return out


def build_proposals(con, cards, as_of: datetime | None = None):
    as_of = as_of or datetime.now(timezone.utc)
    deadlines = {c.ref: c.core_fields["deadline"] for c in cards}
    return compare.propose(shadows_for(con, cards), deadlines, as_of)


def approved_pairs(con, core) -> list:
    """Latest decision per pair that approves same/opposite and still covers the current card content."""
    current = current_fingerprints(core_db.scope_cards(core))
    return [d for d in store.latest_decisions(con).values()
            if d["verdict"] in ("same", "opposite") and not store.is_stale(d, current)]


def preview_route(core, d, side: str) -> tuple[route_sim.Route, object, object]:
    """The routing decision for one approved pair and side, with the two prices it used. Nothing is saved."""
    side_b = side if d["verdict"] == "same" else ("no" if side == "yes" else "yes")
    pa = core_db.latest_price(core, d["ref_a"], side)
    pb = core_db.latest_price(core, d["ref_b"], side_b)
    as_of = route_sim.as_of_from_prices(pa, pb) or values.core_text(datetime.now(timezone.utc))
    return route_sim.decide(d["verdict"], d["ref_a"], d["ref_b"], side, pa, pb, as_of), pa, pb


def log_route(con, core, d, side: str):
    """Log one approved decision's route for one side, once. Returns the logged row (existing or new)."""
    if existing := store.logged_route(con, d["id"], side):
        return existing
    r, pa, pb = preview_route(core, d, side)
    _save(con, d, r, pa, pb)
    return store.logged_route(con, d["id"], side)


def _save(con, d, r, pa, pb) -> None:
    # Log both saved prices even when the route is refused, so the record shows every input it judged.
    inputs = {"price_a": pa.buy_price if pa else None, "fee_a": pa.fee if pa else None,
              "priced_at_a": pa.priced_at if pa else None, "price_b": pb.buy_price if pb else None,
              "fee_b": pb.fee if pb else None, "priced_at_b": pb.priced_at if pb else None}
    store.save_route(con, d["id"], r.__dict__ | inputs | {"policy_version": policy.current_version(con),
                                                          "decision_policy_version": d["policy_version"]})


def route_approved(con, core, side: str) -> list[route_sim.Route]:
    """Log the route for every approved pair not yet logged for this side. Already logged pairs are skipped."""
    routes = []
    for d in approved_pairs(con, core):
        if store.logged_route(con, d["id"], side):
            continue
        r, pa, pb = preview_route(core, d, side)
        _save(con, d, r, pa, pb)
        routes.append(r)
    return routes
