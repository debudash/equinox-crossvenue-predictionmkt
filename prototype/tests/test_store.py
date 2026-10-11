import sqlite3

import pytest
from conftest import shadow

from proto import compare, evals, store


def pair():
    return compare.compare(shadow("kalshi:K", card_id=1, sid=10),
                           shadow("polymarket:P", card_id=2, sid=20, cutoff_op="gte", cutoff_value="4"))


def test_reason_is_required(con):
    with pytest.raises(sqlite3.IntegrityError):
        store.record_decision(con, pair(), "same", "   ", "dd")


def test_decisions_are_append_only_and_latest_wins(con):
    store.record_decision(con, pair(), "reject", "unsure", "dd")
    store.record_decision(con, pair(), "same", "25 bp grid", "dd")
    assert store.latest_decisions(con)[("kalshi:K", "polymarket:P")]["verdict"] == "same"
    with pytest.raises(sqlite3.DatabaseError):
        con.execute("DELETE FROM decisions")


def test_decision_goes_stale_when_a_card_changes(con):
    store.record_decision(con, pair(), "same", "grid", "dd")
    d = store.latest_decisions(con)[("kalshi:K", "polymarket:P")]
    assert not store.is_stale(d, {"kalshi:K": "fp-kalshi:K", "polymarket:P": "fp-polymarket:P"})
    assert store.is_stale(d, {"kalshi:K": "fp-kalshi:K", "polymarket:P": "rules-changed"})


def test_field_reviews_become_gold_and_metrics(con):
    for ref, cid in (("kalshi:K", 1), ("polymarket:P", 2)):   # the readings the reviewed pair points at
        con.execute("INSERT INTO shadow_cards (id, card_id, ref, fingerprint, extractor_version, model, fields_json,"
                    " quotes_json, problems_json, example_refs_json, created_at) VALUES (?,?,?,?,'v1','m','{}','{}','{}','[]','')",
                    (cid * 10, cid, ref, f"fp-{ref}"))
    store.record_decision(con, pair(), "same", "grid", "dd",
                          [("a", "unclear_result", False, "yes_or_no_only"), ("b", "cutoff_op", True, None)])
    assert store.gold_by_fingerprint(con) == {"fp-kalshi:K": {"unclear_result": "yes_or_no_only"},
                                              "fp-polymarket:P": {"cutoff_op": "gte"}}
    m = evals.review_metrics(con)
    assert m["field_accuracy"] == (1, 2) and m["proposal_precision"] == (1, 1)


def test_a_route_is_logged_once_per_decision_and_side(con):
    from proto import route_sim
    did = store.record_decision(con, pair(), "same", "grid", "dd")
    r = route_sim.Route(relation="same", ref_a="kalshi:K", ref_b="polymarket:P", side_a="yes", action="no_route", reason="x")
    assert store.logged_route(con, did, "yes") is None
    store.save_route(con, did, r.__dict__)
    assert store.logged_route(con, did, "yes") is not None and store.logged_route(con, did, "no") is None
