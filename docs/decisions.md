# Decision log

Every decision that shapes the build, who made it, and where it is recorded. Status: Locked (in the build requirements), Approved (amendment or explicit approval), Assumed (applied, awaiting explicit confirmation), Open.

| ID | Date | Decision | By | Status | Recorded in |
|---|---|---|---|---|---|
| D-01 | 2026-10-07 | Venues are Kalshi and Polymarket | Debu Dash | Locked | build-requirements, locked choice 1 |
| D-02 | 2026-10-07 | Standards approach: RFC-style draft extending FINOS CDM; no patent | Debu Dash | Approved | ADR-007, spec section 9 |
| D-03 | 2026-10-07 | Citations only for business subject matter; technology references kept unmapped | Debu Dash | Approved | citations.md |
| D-04 | 2026-10-09 | Equivalence is exact payoff-field equality: same, opposite, not_same; no score, no model | Debu Dash | Locked | build-requirements "Equivalence", ADR-001 |
| D-05 | 2026-10-09 | Router runs on saved data only; Kalshi wins exact ties; 10-minute price age limit | Debu Dash | Locked | build-requirements "Routing", ADR-003 |
| D-06 | 2026-10-09 | Kalshi's router role is set in config (reference_venue) so router code has no venue names | Debu Dash | Approved | AGENTS.md rule 2, ADR-003 |
| D-07 | 2026-10-09 | Router ignores prices saved after as_of, so reruns at the same as_of are repeatable | Debu Dash | Approved | assumptions A-1, ADR-002 |
| D-08 | 2026-10-09 | README notes residual settlement differences (rule wording and fallback settlement) as a known limitation; the oracle-vote gap no longer applies with Polymarket US | Debu Dash | Approved | README, spec section 8 |
| D-09 | 2026-10-09 | Feed scrolling pages through saved data only; never fetches from a venue | Debu Dash | Approved | amendment A3, ADR-008 |
| D-10 | 2026-10-09 | Background `watch`: discovery loop and price loop; match on card change, route on price change | Debu Dash | Approved | amendment A1, ADR-008 |
| D-11 | 2026-10-09 | Feed served from an in-memory snapshot of the latest completed cycle, hydrated from SQLite | Debu Dash | Approved | amendments A2, A3, ADR-008 |
| D-12 | 2026-10-09 | watch routes both yes and no on each price change | Debu Dash | Approved | amendment A6 |
| D-13 | 2026-10-09 | watch and serve are separate processes; SQLite is the only handoff | Debu Dash | Approved | amendment A7 |
| D-14 | 2026-10-09 | Defaults: discovery 30 min, prices 2 min, retention 24 h; watch refuses a price interval at or above the age limit | Debu Dash | Approved | amendment A8, config/equinox.yaml |
| D-15 | 2026-10-09 | Runs page with buttons dropped in favor of watch | Debu Dash | Approved | this log |
| D-17 | 2026-10-09 | Wireframes: workflow, command line (including watch), feed, post with both-side bot replies; old tier screens superseded | Debu Dash | Approved | wireframe canvas, README |
| D-16 | 2026-10-09 | Polymarket venue is Polymarket US (CFTC-regulated, public market data API, USD settlement); International only as a fallback if no matchable Fed pair exists | Debu Dash | Approved | amendment A9, T-11 |
| D-18 | 2026-10-09 | Wave 1 gate: before Wave 2, confirm at least one real Fed market on each venue fits the cutoff format (gt, gte, lt, lte) | Debu Dash | Approved | START_HERE, T-10, T-11 |
