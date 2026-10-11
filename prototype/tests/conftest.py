import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proto import store  # noqa: E402
from proto.core_db import CoreCard  # noqa: E402


@pytest.fixture
def con(tmp_path):
    return store.connect(tmp_path / "proposals.db")


def card(market_id, venue="kalshi", card_id=1, rules="", title="t", **fields):
    base = {f: None for f in ("subject", "cutoff_op", "cutoff_value", "cutoff_unit", "deadline", "decider", "unclear_result")}
    return CoreCard(card_id, venue, market_id, title, rules, "event", "macro", base | fields)


def shadow(ref, card_id=1, sid=1, **fields):
    base = {"subject": "fed_funds_upper_bound", "cutoff_op": "gt", "cutoff_value": "3.75", "cutoff_unit": "percent",
            "deadline": "2028-01-01T04:59:00Z", "decider": "federal_reserve", "unclear_result": None}
    return {"id": sid, "card_id": card_id, "ref": ref, "fingerprint": f"fp-{ref}", "fields": base | fields,
            "quotes": {}, "problems": {}}
