"""Policy snapshots, versions, clause checks and grounding. Documents are faked; no network."""

from conftest import card, shadow

from proto import compare, extract, policy, store
from proto.extract import Extraction, FieldValue

ECON = ("Contingencies: Before Settlement, Kalshi may, at its sole discretion, initiate the Market Outcome Review "
        "Process pursuant to Rule 6.3(d) of the Rulebook. If an Expiration Value cannot be determined on the "
        "Expiration Date, Kalshi has the right to determine payouts pursuant to Rule 6.3(b) in the Rulebook.")


def docs(**override):
    """Every source returns its codified clauses joined (so none is missing), unless overridden."""
    by_source = {}
    for c in policy.clauses():
        by_source.setdefault(c["source"], []).append(c["text"])
    texts = {s["url"]: override.get(s["id"], " ".join(by_source.get(s["id"], []))) for s in policy.sources()}
    return lambda url: texts[url]


def test_first_refresh_issues_a_version_and_an_unchanged_day_does_not(con):
    first = policy.refresh(con, docs())
    assert first["result"] == "new_version" and first["version"].endswith(".1") and first["missing"] == []
    again = policy.refresh(con, docs())
    assert again == {"result": "unchanged", "version": first["version"], "changed": [], "missing": [], "failed": {}}
    assert [r["result"] for r in con.execute("SELECT result FROM policy_checks ORDER BY id")] == ["new_version", "unchanged"]


def test_pdf_spacing_noise_is_not_a_change_but_new_wording_is(con):
    v1 = policy.refresh(con, docs())["version"]
    base = docs()
    respaced = lambda url: "  ".join(base(url).split(" "))          # extraction spacing only
    assert policy.refresh(con, respaced)["result"] == "unchanged"
    reworded = docs(**{"kalshi-econstat-terms": ECON.replace("Kalshi has the right", "Kalshi will")})
    r = policy.refresh(con, reworded)
    assert r["result"] == "new_version" and r["version"] != v1
    assert "kalshi-econstat-terms" in r["changed"] and "KECON-unclear" in r["missing"]
    assert "Kalshi will" in policy.diff(con, r["version"], "kalshi-econstat-terms")


def test_failed_fetch_keeps_last_snapshot_and_never_creates_a_version(con):
    v1 = policy.refresh(con, docs())["version"]

    def down(url):
        raise ConnectionError("timeout")

    r = policy.refresh(con, down)
    assert r["result"] == "unchanged" and r["version"] == v1 and set(r["failed"]) == {s["id"] for s in policy.sources()}


def test_clauses_apply_by_venue_and_series():
    assert {c["id"] for c in policy.applicable("kalshi:KXFEDFUNDSYEAR-28JAN01-T3.75")} >= {"KECON-unclear"}
    assert not any(c["source"] == "kalshi-econstat-terms" for c in policy.applicable("kalshi:KXFED-26DEC-T4.50"))
    assert any(c["id"] == "PRB-unclear" for c in policy.applicable("polymarket:anything"))
    assert "<venue_policy>" in policy.prompt_block("kalshi:KXFED-26DEC-T4.50")


def test_value_quoted_from_venue_policy_is_kept_with_its_provenance():
    c = card("KXFEDFUNDSYEAR-28JAN01-T3.75", rules="If the upper bound ... is above 3.75%, then the market resolves to Yes.")
    none = FieldValue(value=None, quote=None)
    raw = Extraction(**{f: none for f in ("subject", "cutoff_op", "cutoff_value", "cutoff_unit", "deadline", "decider")},
                     unclear_result=FieldValue(value="venue_discretion",
                                               quote="Kalshi has the right to determine payouts pursuant to Rule 6.3(b)"))
    f, _, p, prov = extract.check(c, raw)
    assert f["unclear_result"] == "venue_discretion" and prov["unclear_result"] == "policy:KECON-unclear" and p == {}


def test_both_venues_keeping_discretion_is_flagged_not_treated_as_equal():
    p = compare.compare(shadow("kalshi:K", unclear_result="venue_discretion"),
                        shadow("polymarket:P", unclear_result="venue_discretion"))
    assert p.soft == ["both_venue_discretion"] and p.tier == "near_miss"


def test_decision_records_the_policy_version(con):
    p = compare.compare(shadow("kalshi:K"), shadow("polymarket:P"))
    store.record_decision(con, p, "same", "grid", "dd", policy_version="2026-10-11.1")
    assert store.latest_decisions(con)[("kalshi:K", "polymarket:P")]["policy_version"] == "2026-10-11.1"
