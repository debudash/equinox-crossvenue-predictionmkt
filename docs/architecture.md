# Architecture

High-level design shows context and the four pieces. Low-level design shows the storage schema, the core types, and the runtime sequences. Diagrams are Mermaid and render on GitHub.

## Design principles
1. Four pieces run in order, and each reads only what earlier pieces saved. Only fetchers touch the network.
2. Venue knowledge lives only in fetchers. Matcher and router get venue roles from config, never from code.
3. Empty means unknown, and unknown never matches.
4. A refusal is a first-class outcome, logged exactly like a choice.
5. Bad data is skipped, counted and logged. It never stops a run and never reaches a decision.

## High-level design

### Context
```mermaid
flowchart LR
  person([Evaluator or analyst])
  subgraph Equinox
    sys[Equinox simulation]
  end
  kalshi[(Kalshi public API)]
  poly[(Polymarket US public API)]
  person -- "CLI: fetch, match, route, rebuild, watch, unlock" --> sys
  person -- "browser: read-only feed" --> sys
  sys -- "fetch open markets and prices" --> kalshi
  sys -- "fetch open markets and prices" --> poly
```

### The four pieces
```mermaid
flowchart LR
  subgraph F[1 Fetchers]
    KF[Kalshi fetcher]
    PF[Polymarket fetcher]
  end
  V[(config/vocab.yaml)]
  subgraph DB[(SQLite)]
    RAW[raw_responses]
    CARDS[cards]
    PRICES[prices]
    RUNS[fetch_runs and skips]
    LOG[decision_log - append only]
  end
  M[2 Matcher]
  R[3 Router]
  FEED[4 Feed - read only]
  CFG[(config/equinox.yaml)]

  KF --> RAW
  PF --> RAW
  V --> KF
  V --> PF
  KF --> CARDS
  PF --> CARDS
  KF --> PRICES
  PF --> PRICES
  KF --> RUNS
  PF --> RUNS
  CARDS --> M
  M --> LOG
  LOG --> R
  PRICES --> R
  CFG --> R
  R --> LOG
  CARDS --> FEED
  PRICES --> FEED
  LOG --> FEED
  RAW -- "equinox rebuild" --> CARDS
```

| Piece | Reads | Writes | Network |
|---|---|---|---|
| Fetchers | venue APIs, vocab.yaml | raw_responses, cards, prices, fetch_runs, skips | Yes, the only piece |
| Matcher | cards | decision_log (match records) | No |
| Router | decision_log (latest match result per pair), prices, equinox.yaml, `as_of` | decision_log (route records) | No |
| Feed | in-memory snapshot of the latest completed cycle, built from cards, prices and decision_log | nothing | No |
| Watch (amendment A1) | venue APIs via fetchers, then the same match and route code | same as fetch, match and route, plus cycles and pruning | Yes, through fetchers only |

## Low-level design

### Storage schema
```mermaid
erDiagram
  FETCH_RUNS ||--o{ RAW_RESPONSES : produces
  FETCH_RUNS ||--o{ SKIPS : records
  RAW_RESPONSES ||--o{ CARDS : "raw_ref"
  CARDS ||--o{ PRICES : "venue + market_id"
  CARDS ||--o{ DECISION_LOG : "market_a or market_b"

  CYCLES ||--o{ FETCH_RUNS : groups
  CYCLES ||--o{ CARDS : writes
  CYCLES ||--o{ PRICES : writes
  CYCLES ||--o{ DECISION_LOG : writes
  CYCLES ||--o{ SKIPS : writes
  CYCLES ||--o{ SUBJECT_COUNTS : writes
  CYCLES {
    int id
    string kind
    string started_at
    string completed_at
    string status
    string trigger
  }
  RUN_LOCK {
    string holder
    string acquired_at
  }
  FETCH_RUNS {
    int id
    int cycle_id
    string venue
    string started_at
    string finished_at
    string status
    int saved_count
    int skipped_count
    string error
  }
  RAW_RESPONSES {
    int id
    int cycle_id
    int run_id
    string kind
    string venue
    string endpoint
    string fetched_at
    string sha256
    blob body
  }
  SKIPS {
    int id
    int cycle_id
    int run_id
    string stage
    string venue
    string market_id
    string reason
  }
  CARDS {
    int id
    int cycle_id
    string venue
    string market_id
    string subject
    string cutoff_op
    string cutoff_value
    string cutoff_unit
    string deadline
    string decider
    string unclear_result
    string title
    string rules_text
    string raw_ref
    string fetched_at
    string event_title
    string category
    string volume_24h
    string url
  }
  PRICES {
    int id
    int cycle_id
    string venue
    string market_id
    string side
    string buy_price
    string fee
    string priced_at
  }
  DECISION_LOG {
    int id
    int cycle_id
    string record_type
    string decided_at
    string as_of
    string market_a
    string market_b
    string match_result
    string side_a
    string side_b
    string price_a
    string price_b
    string priced_at_a
    string priced_at_b
    string fee_applied
    string action
    string reason
  }
  SUBJECT_COUNTS {
    int cycle_id
    string subject
    string venue
    int count
  }
```
Every data row carries the cycle_id that wrote it, and readers see only completed cycles. Cards are versioned: a new row only when a card changes (assumptions A-5 to A-9; internal/store/schema.sql is the source). Empty payoff fields are stored as NULL. Prices and fees are stored as decimal strings and parsed into integer micro-dollars in Go. The log tables have no UPDATE or DELETE paths.

### Core types and boundaries
```mermaid
classDiagram
  class Fetcher {
    <<interface>>
    +Name() string
    +Fetch(ctx) RunResult
  }
  class Card {
    +Venue string
    +MarketID string
    +Payoff Payoff
    +Record RecordFields
  }
  class Payoff {
    +Subject *string
    +CutoffOp *Op
    +CutoffValue *Decimal
    +CutoffUnit *string
    +Deadline *time
    +Decider *string
    +UnclearResult *string
  }
  class PriceRecord {
    +Venue string
    +MarketID string
    +Side yes or no
    +BuyPrice *Micros
    +Fee *Micros
    +PricedAt time
  }
  class MatchResult {
    +Result same or opposite or not_same
    +Reason string
  }
  class Matcher {
    <<function>>
    +Compare(a Card, b Card) MatchResult
  }
  class Router {
    <<function>>
    +Decide(match, side, prices, asOf, roles) Decision
  }
  class Decision {
    +Action string
    +FeeApplied bool
    +Reason string
  }
  Fetcher ..> Card : writes
  Fetcher ..> PriceRecord : writes
  Card *-- Payoff
  Matcher ..> MatchResult
  Router ..> Decision
```
`Compare` and `Decide` are pure functions. Thin runners load from SQLite, call them, and append to the log.

### Sequence: fetch
```mermaid
sequenceDiagram
  participant CLI as equinox fetch
  participant F as Fetcher (each venue, independently)
  participant API as Venue API
  participant DB as SQLite
  CLI->>F: Fetch(ctx with timeout)
  F->>API: GET open markets and prices
  alt response received
    API-->>F: payload
    F->>DB: save raw response first
    F->>F: normalize into card and price records via vocab.yaml
    F->>DB: save valid cards and prices
    F->>DB: save skips with reasons
  else request fails
    F->>DB: save error in fetch_runs
  end
  F->>DB: save counts (saved, skipped)
  Note over CLI,F: a failure in one fetcher never stops the other
```

### Sequence: match
```mermaid
sequenceDiagram
  participant CLI as equinox match
  participant DB as SQLite
  participant M as Compare
  CLI->>DB: load cards, group by non-empty subject
  CLI->>DB: log card count per subject
  loop each reference-venue card x counter-venue card with the same subject
    CLI->>M: Compare(a, b)
    M-->>CLI: same, opposite or not_same plus reason
    CLI->>DB: append match record
  end
```

### Sequence: route
```mermaid
sequenceDiagram
  participant CLI as equinox route --side --as-of
  participant DB as SQLite
  participant R as Decide
  CLI->>DB: latest match record per pair where result is same or opposite
  loop each pair
    CLI->>DB: latest price per needed card and side with priced_at not after as_of
    CLI->>R: Decide(match, side, prices, as_of, roles)
    Note over R: no network, no clock, no titles
    R-->>CLI: action, fee_applied, reason
    CLI->>DB: append route record
  end
```

### Sequence: rebuild
```mermaid
sequenceDiagram
  participant CLI as equinox rebuild
  participant DB as SQLite
  participant N as Normalizer (each venue)
  CLI->>DB: load saved raw responses
  CLI->>N: same normalization code path as fetch
  N-->>CLI: cards
  CLI->>DB: replace cards
  Note over CLI: no venue call; test 18 asserts identical cards
```

## Background refresh and feed cache (amendments A1 to A5)

### Watch loops
```mermaid
flowchart TB
  subgraph W[equinox watch - one process, holds the run lock]
    D[Discovery loop - every 30 min]
    P[Price loop - every 2 min]
    PR[Retention - prune prices older than 24 h]
  end
  D --> FD[Fetch market lists via fetchers]
  FD --> CH{Cards added or changed?}
  CH -- yes --> MA[Run match]
  CH -- no --> SKIP[Skip match]
  P --> FP[Refresh prices for matched markets only]
  FP --> SAVE[Save a price record on every poll]
  SAVE --> PC{Needed price changed?}
  PC -- yes --> RT[Run route for that pair, as_of = refresh time]
  PC -- no --> NOOP[No new route record]
  MA --> CY[(Mark cycle completed)]
  SKIP --> CY
  RT --> CY
  NOOP --> CY
```

### Feed cache swap
```mermaid
sequenceDiagram
  participant W as watch
  participant DB as SQLite
  participant S as serve: cache
  participant B as Browser
  W->>DB: write cycle N (fetch, match, route)
  W->>DB: mark cycle N completed
  loop every 5 s
    S->>DB: latest completed cycle id
    alt newer than cached
      S->>DB: read cards, latest prices, latest log for cycle N
      S->>S: build snapshot N, swap pointer atomically, keep N-1
    end
  end
  B->>S: GET feed (rendered from snapshot N)
  B->>S: GET more?cycle=N&cursor=...
  S-->>B: next page from snapshot N
  Note over B,S: if snapshot N+1 exists, page shows the new-data banner
```

Rules: the cache is built only from SQLite and only from completed cycles; it never calls a venue and never writes. Pages carry their cycle number so "Load more" finishes on the cycle the reader started with. The previous snapshot is kept for that reason.

## Key tradeoffs
| Choice | Gained | Given up |
|---|---|---|
| Exact payoff equality, no score | Zero tolerance for wrong-bet routing, fully explainable | Recall limited by vocabulary and fetcher rules |
| Shared vocabulary, one category in V1 | Equality is meaningful across venues | Narrow coverage until entries are added |
| Saved data only for match and route | Determinism, no blocking, auditability | Decisions use prices up to 10 minutes old |
| SQLite | Single file, queryable, zero setup | Single writer |
| One contract at the buy price | Simple, explainable | No size, depth or slippage |
| Venue roles in config | Router has no venue names | One more config file to keep correct |
| Feed from an in-memory snapshot | Fast scrolling, consistent point-in-time view | Up to one cache check interval behind SQLite; memory grows with market count |
| Background watch | Feed stays current without a person | Venue rate limits to respect; one more process to run |
