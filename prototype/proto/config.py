"""Paths, model and thresholds for the proposal prototype. Nothing here is read by the core."""

import os
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]          # repo root
PROTO = ROOT / "prototype"

CORE_DB = Path(os.getenv("EQX_CORE_DB", ROOT / "data" / "equinox.db"))       # opened read-only
DEMO_DB = Path(os.getenv("EQX_DEMO_DB", ROOT / "data" / "demo.db"))         # synthetic, seeded by scripts/seed
PROPOSALS_DB = Path(os.getenv("EQX_PROPOSALS_DB", ROOT / "data" / "proposals.db"))  # the only file we write


def load_dotenv() -> None:
    """Read prototype/.env (KEY=VALUE lines) into the environment, without overriding."""
    f = PROTO / ".env"
    if not f.exists():
        return
    for line in f.read_text(encoding="utf-8").splitlines():
        if "=" in line and not line.lstrip().startswith("#"):
            k, v = line.split("=", 1)
            os.environ.setdefault(k.strip(), v.strip().strip('"'))


load_dotenv()

MODEL = os.getenv("EQX_PROTO_MODEL", "claude-sonnet-5")   # set EQX_PROTO_MODEL in prototype/.env to change
EXTRACTOR_VERSION = "v1"     # bump when the prompt or schema changes; old shadow cards stay as history

# Deviation thresholds (locked design): 0 strong, 1-2 near miss, 3+ dropped.
NEAR_MISS_MAX = 2
MIN_DAYS_TO_CLOSE = 7

# The fed funds target moves in 25 basis point steps. This is the assumption a reviewer
# approves when they accept a "grid_boundary" pair (gt 3.75 is gte 4.00 on this grid).
CUTOFF_STEP = {"fed_funds_upper_bound": "0.25"}

_core_cfg = yaml.safe_load((ROOT / "config" / "equinox.yaml").read_text(encoding="utf-8"))
_vocab = yaml.safe_load((ROOT / "config" / "vocab.yaml").read_text(encoding="utf-8"))

REFERENCE_VENUE = _core_cfg["router"]["reference_venue"]
COUNTER_VENUE = _core_cfg["router"]["counter_venue"]
_age = str(_core_cfg["router"]["price_max_age"])   # e.g. "10m"; same limit the core router uses
PRICE_MAX_AGE_SECONDS = int(_age[:-1]) * {"s": 1, "m": 60, "h": 3600}[_age[-1]]

SUBJECTS = set(_vocab["subjects"])
DECIDERS = set(_vocab["deciders"])
CUTOFF_UNITS = set(_vocab["cutoff_units"])
OPS = {"gt", "gte", "lt", "lte"}
# Core vocabulary plus two prototype values found in the venues' published terms (policies/clauses.yaml):
# resolves_no (Kalshi KXFED: no data at expiration resolves No) and venue_discretion (the venue decides).
UNCLEAR_RESULTS = {"yes_or_no_only", "half", "last_price", "cancelled", "resolves_no", "venue_discretion"}

FIELDS = ["subject", "cutoff_op", "cutoff_value", "cutoff_unit", "deadline", "decider", "unclear_result"]
