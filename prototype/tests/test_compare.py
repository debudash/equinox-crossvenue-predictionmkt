from datetime import datetime, timezone

from conftest import shadow

from proto import compare

K, P = "kalshi:KXFEDFUNDSYEAR-28JAN01-T3.75", "polymarket:rvcus-gte4pt00"


def test_gt_375_and_gte_400_is_one_grid_deviation_proposed_as_same():
    p = compare.compare(shadow(K, unclear_result="yes_or_no_only"),
                        shadow(P, cutoff_op="gte", cutoff_value="4", unclear_result="yes_or_no_only"))
    assert p.relation == "same" and p.soft == ["grid_boundary"] and p.tier == "near_miss"


def test_gt_400_and_gte_400_is_a_different_bet_not_the_closest_pair():
    # Literal field counting ranks this pair closest (one field differs). On the 25 bp grid it is a different bet.
    p = compare.compare(shadow(K, cutoff_value="4"), shadow(P, cutoff_op="gte", cutoff_value="4"))
    assert p.hard == ["cutoff_differs"] and p.tier == "dropped"


def test_exact_complement_is_opposite_and_grid_complement_too():
    assert compare.compare(shadow(K), shadow(P, cutoff_op="lte")).relation == "opposite"
    p = compare.compare(shadow(K), shadow(P, cutoff_op="lt", cutoff_value="4"))   # gt 3.75 vs lt 4.00
    assert p.relation == "opposite" and "grid_boundary" in p.soft


def test_all_equal_and_stated_is_strong():
    p = compare.compare(shadow(K, unclear_result="half"), shadow(P, unclear_result="half"))
    assert p.tier == "strong" and p.relation == "same" and p.pattern == "none"


def test_unknowns_are_soft_and_three_drop_the_pair():
    p = compare.compare(shadow(K), shadow(P, decider=None, deadline=None))
    assert sorted(p.soft) == ["deadline_unknown", "decider_unknown", "unclear_result_unknown"]
    assert p.tier == "dropped"


def test_deadline_hours_apart_is_soft_days_apart_is_hard():
    near = compare.compare(shadow(K, unclear_result="half"), shadow(P, unclear_result="half", deadline="2028-01-01T05:00:00Z"))
    far = compare.compare(shadow(K), shadow(P, deadline="2027-06-01T04:59:00Z"))
    assert near.soft == ["deadline_close"] and "deadline_differs" in far.hard


def test_meeting_decision_market_without_cutoff_is_cutoff_unknown():
    p = compare.compare(shadow(K, unclear_result="half"),
                        shadow(P, cutoff_op=None, cutoff_value=None, cutoff_unit=None, unclear_result="half"))
    assert p.relation is None and p.soft == ["cutoff_unknown"]


def test_propose_keeps_only_pairs_with_seven_days_and_counts_the_funnel():
    shadows = [shadow(K, unclear_result="half"), shadow(P, unclear_result="half"),
               shadow("kalshi:SOON", deadline="2026-10-12T00:00:00Z", unclear_result="half")]
    props, counts = compare.propose(shadows, {}, datetime(2026, 10, 10, tzinfo=timezone.utc))
    assert [p.key for p in props] == [(K, P)]
    assert counts["pairs checked"] == 2 and counts["closes within 7 days or unknown"] == 1 and counts["strong"] == 1


def test_fetcher_wins_llm_fills_gaps_and_disagreement_is_flagged():
    from proto import pipeline
    core = {"subject": "fed_funds_upper_bound", "cutoff_op": "gt", "cutoff_value": "4.5", "cutoff_unit": "percent",
            "deadline": "2026-12-09T19:00:00Z", "decider": "federal_reserve", "unclear_result": None}
    llm = core | {"deadline": None, "unclear_result": "yes_or_no_only", "cutoff_op": "gte"}
    fields, source, disagree = pipeline.merge(core, llm)
    assert fields["deadline"] == "2026-12-09T19:00:00Z" and source["deadline"] == "fetcher"
    assert fields["unclear_result"] == "yes_or_no_only" and source["unclear_result"] == "llm"
    assert fields["cutoff_op"] == "gt" and disagree == ["cutoff_op"]
    p = compare.compare(shadow(K, unclear_result="half") | {"disagree": ["cutoff_op"]}, shadow(P, unclear_result="half"))
    assert "fetcher_llm_disagree" in p.soft
