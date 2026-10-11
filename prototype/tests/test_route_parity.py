"""Parity with the core router (internal/route.Decide).

1. The core's own required tests 08-14, ported vector for vector.
2. Replay of every route record the core wrote to data/demo.db: same inputs must give the same
   action, sides, fee flag and reason sentence, character for character.
"""

import sqlite3
from datetime import timedelta

import pytest

from proto import config, route_sim, values
from proto.core_db import Price

ROLES = ("ref", "ctr")
A, B = "ref:K1", "ctr:P1"
AS_OF = values.parse_utc("2026-10-10T12:00:00Z")
AS_OF_TEXT = values.core_text(AS_OF)


def price(ref, side, buy, fee=None, age=timedelta(0)):
    v, m = ref.split(":")
    return Price(v, m, side, buy or None, fee, values.core_text(AS_OF - age))


def decide(rel, side, pa, pb):
    return route_sim.decide(rel, A, B, side, pa, pb, AS_OF_TEXT, max_age_s=600, roles=ROLES)


def test_req08_not_same_is_no_route():
    r = decide("not_same", "yes", price(A, "yes", "0.4"), price(B, "yes", "0.3"))
    assert r.action == "no_route" and "not the same" in r.reason


def test_req09_missing_buy_price_is_no_route():
    assert "ctr yes price is missing for P1" in decide("same", "yes", price(A, "yes", "0.4"), None).reason
    assert "ref yes price is missing for K1" in decide("same", "yes", price(A, "yes", ""), price(B, "yes", "0.4")).reason


def test_req10_stale_price_is_no_route_and_the_limit_itself_is_fresh():
    r = decide("same", "yes", price(A, "yes", "0.4", age=timedelta(minutes=11)), price(B, "yes", "0.3", age=timedelta(minutes=1)))
    assert r.action == "no_route" and "ref yes price is stale for K1: saved 11m0s before as_of, over the 10m0s limit." in r.reason
    r = decide("same", "yes", price(A, "yes", "0.4", age=timedelta(minutes=10)), price(B, "yes", "0.3"))
    assert r.action == "buy_on_ctr"


def test_req11_lower_cost_wins_and_fees_can_change_it():
    assert decide("same", "yes", price(A, "yes", "0.42"), price(B, "yes", "0.45")).action == "buy_on_ref"
    assert decide("same", "yes", price(A, "yes", "0.42", "0.02"), price(B, "yes", "0.40", "0")).action == "buy_on_ctr"
    r = decide("same", "yes", price(A, "yes", "0.40", "0.01"), price(B, "yes", "0.39", "0.03"))
    assert r.action == "buy_on_ref" and r.fee_applied == "yes"


def test_req12_fee_on_one_venue_only_compares_buy_price():
    r = decide("same", "yes", price(A, "yes", "0.40", "0.05"), price(B, "yes", "0.42"))
    assert r.action == "buy_on_ref" and r.fee_applied == "no" and "unknown on ctr" in r.reason


def test_req13_opposite_compares_yes_with_no():
    r = decide("opposite", "yes", price(A, "yes", "0.60"), price(B, "no", "0.55"))
    assert (r.action, r.side_a, r.side_b) == ("buy_on_ctr", "yes", "no")


def test_req14_equal_cost_chooses_reference_venue():
    r = decide("same", "no", price(A, "no", "0.5"), price(B, "no", "0.5"))
    assert r.action == "buy_on_ref" and "exact ties" in r.reason


def test_price_outside_zero_to_one_is_not_used():
    assert "outside 0 to 1" in decide("same", "yes", price(A, "yes", "1.25"), price(B, "yes", "0.95")).reason


def test_go_duration_matches_go_formatting():
    ns = 1_000_000_000
    cases = {0: "0s", 1: "1ns", 1500: "1.5µs", 2_500_000: "2.5ms", 11 * 60 * ns: "11m0s",
             3600 * ns: "1h0m0s", 90 * ns + 500_000_000: "1m30.5s", 3 * ns: "3s"}
    for v, want in cases.items():
        assert route_sim.go_duration(v) == want


DEMO = config.ROOT / "data" / "demo.db"


@pytest.mark.skipif(not DEMO.exists(), reason="data/demo.db not present (go run ./scripts/seed)")
def test_replay_core_route_records_from_demo_db():
    c = sqlite3.connect(f"file:{DEMO.as_posix()}?mode=ro", uri=True)
    c.row_factory = sqlite3.Row
    routes = c.execute("SELECT * FROM decision_log WHERE record_type='route'").fetchall()
    assert routes, "demo.db has no route records to replay"

    def saved(ref, side, priced_at):
        v, m = ref.split(":", 1)
        r = c.execute("SELECT venue, market_id, side, buy_price, fee, priced_at FROM prices WHERE venue=? AND market_id=?"
                      " AND side=? AND priced_at<=? ORDER BY priced_at DESC, id DESC LIMIT 1", (v, m, side, priced_at)).fetchone()
        return Price(*r) if r else None

    for rec in routes:
        pa = saved(rec["market_a"], rec["side_a"], rec["as_of"])
        pb = saved(rec["market_b"], rec["side_b"], rec["as_of"])
        got = route_sim.decide(rec["match_result"], rec["market_a"], rec["market_b"], rec["side_a"], pa, pb, rec["as_of"])
        assert (got.action, got.side_b, got.fee_applied, got.reason) == \
               (rec["action"], rec["side_b"], rec["fee_applied"], rec["reason"]), rec["id"]
