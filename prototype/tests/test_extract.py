from conftest import card

from proto import extract, store
from proto.extract import Extraction, FieldValue

RULES = ("This market will settle to Yes if the upper bound of the target range for the federal funds rate is "
         "greater than or equal to 4.00% on December 31, 2027 at 11:59 PM ET. Outcome sourced from the United States "
         "Federal Reserve.")
C = card("rvcus-gte4pt00", venue="polymarket", rules=RULES, title="4.00% Or Above")


def ext(**over):
    none = FieldValue(value=None, quote=None)
    base = {f: none for f in ("subject", "cutoff_op", "cutoff_value", "cutoff_unit", "deadline", "decider", "unclear_result")}
    return Extraction(**(base | over))


def test_value_with_verbatim_quote_survives_and_is_normalized():
    f, q, p, _ = extract.check(C, ext(cutoff_op=FieldValue(value="gte", quote="greater than or equal to"),
                                    cutoff_value=FieldValue(value="4.00", quote="4.00%"),
                                    deadline=FieldValue(value="2027-12-31T23:59:00-05:00", quote="December 31, 2027 at 11:59 PM ET")))
    assert f["cutoff_op"] == "gte" and f["cutoff_value"] == "4" and f["deadline"] == "2028-01-01T04:59:00Z"
    assert p == {}


def test_paraphrased_quote_is_emptied_with_a_reason():
    f, _, p, _ = extract.check(C, ext(cutoff_op=FieldValue(value="gte", quote="at least")))
    assert f["cutoff_op"] is None and "verbatim" in p["cutoff_op"]


def test_quote_matching_ignores_whitespace_differences_only():
    f, _, _, _ = extract.check(C, ext(cutoff_op=FieldValue(value="gte", quote="greater than  or\nequal to")))
    assert f["cutoff_op"] == "gte"


def test_value_outside_vocabulary_is_emptied():
    f, _, p, _ = extract.check(C, ext(decider=FieldValue(value="fomc", quote="Federal Reserve")))
    assert f["decider"] is None and "vocabulary" in p["decider"]


def test_deadline_without_offset_is_rejected():
    f, _, p, _ = extract.check(C, ext(deadline=FieldValue(value="2027-12-31T23:59:00", quote="December 31, 2027")))
    assert f["deadline"] is None and "deadline" in p


def test_stated_unknown_stays_empty_without_a_problem():
    f, _, p, _ = extract.check(C, ext())
    assert all(v is None for v in f.values()) and p == {}


def test_shadow_card_is_cached_per_card_version(con):
    calls = []

    def llm(system, user):
        calls.append(user)
        return ext(cutoff_value=FieldValue(value="4.00", quote="4.00%"))

    a = extract.extract_card(con, C, llm)
    b = extract.extract_card(con, C, llm)
    assert len(calls) == 1 and a["id"] == b["id"] and b["fields"]["cutoff_value"] == "4"
    extract.extract_card(con, C, llm, refresh=True)
    assert len(calls) == 2


def test_reviewed_examples_go_into_the_prompt_but_never_the_card_itself():
    other = card("rvcus-gte3pt75", venue="polymarket", card_id=2, rules=RULES.replace("4.00", "3.75"))
    prompt = extract.build_prompt(C, [(other, {"cutoff_op": "gte"})])
    assert "<reviewed_example>" in prompt and '"cutoff_op": "gte"' in prompt
    from proto.learn import pick_examples
    assert pick_examples(C, [(C, {}), (other, {})]) == [(other, {})]


def test_store_round_trip(con):
    sid = store.save_shadow(con, C, "m", {"subject": None}, {}, {}, [])
    assert store.shadow_dict(store.latest_shadow(con, C))["id"] == sid


def test_reading_survives_a_new_card_version_that_only_changes_volume(con):
    # The core saves a new card version when only the 24h volume moves. Same content, same reading.
    store.save_shadow(con, C, "m", {"subject": None}, {}, {}, [])
    newer = card("rvcus-gte4pt00", venue="polymarket", card_id=99, rules=RULES, title="4.00% Or Above")
    assert newer.fingerprint == C.fingerprint and store.latest_shadow(con, newer, "m") is not None
    reworded = card("rvcus-gte4pt00", venue="polymarket", card_id=100, rules=RULES + " Amended.", title="4.00% Or Above")
    assert store.latest_shadow(con, reworded) is None
