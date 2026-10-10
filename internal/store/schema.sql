-- Equinox schema, migration 1. See docs/architecture.md "Storage schema".
-- Empty payoff fields, prices and fees are NULL. Timestamps are fixed-width
-- UTC text (2006-01-02T15:04:05.000000000Z) so text order is time order.
-- Every data row carries the cycle that wrote it; readers see only rows from
-- completed cycles (plus a writer's own open cycle), per amendment A2.

CREATE TABLE cycles (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  kind         TEXT NOT NULL,
  trigger      TEXT NOT NULL,
  started_at   TEXT NOT NULL,
  completed_at TEXT,
  status       TEXT NOT NULL CHECK (status IN ('open', 'completed', 'failed'))
);

CREATE TABLE run_lock (
  id          INTEGER PRIMARY KEY CHECK (id = 1),
  holder      TEXT NOT NULL,
  acquired_at TEXT NOT NULL
);

CREATE TABLE fetch_runs (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  cycle_id      INTEGER NOT NULL REFERENCES cycles (id),
  venue         TEXT NOT NULL,
  started_at    TEXT NOT NULL,
  finished_at   TEXT,
  status        TEXT NOT NULL CHECK (status IN ('running', 'ok', 'failed')),
  saved_count   INTEGER NOT NULL DEFAULT 0,
  skipped_count INTEGER NOT NULL DEFAULT 0,
  error         TEXT
);

CREATE TABLE raw_responses (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  cycle_id   INTEGER NOT NULL REFERENCES cycles (id),
  run_id     INTEGER REFERENCES fetch_runs (id),
  venue      TEXT NOT NULL,
  kind       TEXT NOT NULL CHECK (kind IN ('markets', 'prices')),
  endpoint   TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  sha256     TEXT NOT NULL,
  body       BLOB NOT NULL
);
CREATE INDEX raw_responses_kind ON raw_responses (kind, id);

CREATE TABLE skips (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  cycle_id  INTEGER NOT NULL REFERENCES cycles (id),
  run_id    INTEGER REFERENCES fetch_runs (id),
  stage     TEXT NOT NULL,
  venue     TEXT,
  market_id TEXT,
  reason    TEXT NOT NULL
);

-- One row per card version. A new row is written only when a card is new or changed.
CREATE TABLE cards (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  cycle_id       INTEGER NOT NULL REFERENCES cycles (id),
  venue          TEXT NOT NULL,
  market_id      TEXT NOT NULL,
  subject        TEXT,
  cutoff_op      TEXT CHECK (cutoff_op IN ('gt', 'gte', 'lt', 'lte')),
  cutoff_value   TEXT,
  cutoff_unit    TEXT,
  deadline       TEXT,
  decider        TEXT,
  unclear_result TEXT CHECK (unclear_result IN ('yes_or_no_only', 'half', 'last_price', 'cancelled')),
  title          TEXT NOT NULL,
  rules_text     TEXT NOT NULL,
  raw_ref        TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  event_title    TEXT NOT NULL,
  category       TEXT NOT NULL,
  volume_24h     TEXT NOT NULL,
  url            TEXT NOT NULL
);
CREATE INDEX cards_market ON cards (venue, market_id, id);

CREATE TABLE prices (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  cycle_id  INTEGER NOT NULL REFERENCES cycles (id),
  venue     TEXT NOT NULL,
  market_id TEXT NOT NULL,
  side      TEXT NOT NULL CHECK (side IN ('yes', 'no')),
  buy_price TEXT,
  fee       TEXT,
  priced_at TEXT NOT NULL
);
CREATE INDEX prices_lookup ON prices (venue, market_id, side, priced_at);

CREATE TABLE decision_log (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  cycle_id     INTEGER NOT NULL REFERENCES cycles (id),
  record_type  TEXT NOT NULL CHECK (record_type IN ('match', 'route')),
  decided_at   TEXT NOT NULL,
  as_of        TEXT,
  market_a     TEXT NOT NULL,
  market_b     TEXT NOT NULL,
  match_result TEXT NOT NULL CHECK (match_result IN ('same', 'opposite', 'not_same')),
  side_a       TEXT,
  side_b       TEXT,
  price_a      TEXT,
  price_b      TEXT,
  priced_at_a  TEXT,
  priced_at_b  TEXT,
  fee_applied  TEXT CHECK (fee_applied IN ('yes', 'no')),
  action       TEXT,
  reason       TEXT NOT NULL
);
CREATE INDEX decision_log_pair ON decision_log (market_a, market_b, record_type, id);

-- Card count per subject, written on every match run. Empty subject is ''.
CREATE TABLE subject_counts (
  cycle_id INTEGER NOT NULL REFERENCES cycles (id),
  subject  TEXT NOT NULL,
  venue    TEXT NOT NULL,
  count    INTEGER NOT NULL
);

-- Log tables are append-only (AGENTS.md hard rule 7).
CREATE TRIGGER decision_log_no_update BEFORE UPDATE ON decision_log BEGIN SELECT RAISE(ABORT, 'decision_log is append-only'); END;
CREATE TRIGGER decision_log_no_delete BEFORE DELETE ON decision_log BEGIN SELECT RAISE(ABORT, 'decision_log is append-only'); END;
CREATE TRIGGER skips_no_update BEFORE UPDATE ON skips BEGIN SELECT RAISE(ABORT, 'skips is append-only'); END;
CREATE TRIGGER skips_no_delete BEFORE DELETE ON skips BEGIN SELECT RAISE(ABORT, 'skips is append-only'); END;
CREATE TRIGGER subject_counts_no_update BEFORE UPDATE ON subject_counts BEGIN SELECT RAISE(ABORT, 'subject_counts is append-only'); END;
CREATE TRIGGER subject_counts_no_delete BEFORE DELETE ON subject_counts BEGIN SELECT RAISE(ABORT, 'subject_counts is append-only'); END;
