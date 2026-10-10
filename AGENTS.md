# AGENTS.md - Rules for every coding agent in this repo

Canonical for every harness (Agent Orchestrator workers, Claude Code). CLAUDE.md points here.

## Source of truth
docs/build-requirements.md is authoritative. Build it as written. If anything in this repo conflicts with it, it wins. Do not replace the match rule with a similarity score, a language model, or a percentage.

## What we are building
A simulation that fetches Kalshi and Polymarket markets, stores each as one market card, recognizes `same` and `opposite` bets by exact payoff-field equality, and simulates a routing choice from saved prices. It places no orders. Governing principle: precision over recall. When unsure, do not route, and record why.

## Stack
- Go 1.26+, single module.
- SQLite via `modernc.org/sqlite` (pure Go, no cgo).
- Feed: Go `html/template`, server-rendered, no frontend framework, no JavaScript required.
- Allowed third-party deps: `modernc.org/sqlite`, `gopkg.in/yaml.v3`, `github.com/spf13/cobra`, `github.com/stretchr/testify` (tests only). Anything else needs approval.
- No language model anywhere in the product.

## Commands
```
make setup      # go mod download
make test       # go test ./... -count=1 (adds -race when cgo is available)
make race       # go test -race in a Linux container (Docker), for hosts without cgo
make lint       # go vet + staticcheck
make guard      # fails if a venue name, a clock read, or a network import appears in matcher or router
make acceptance # runs the 20 required tests by name and prints pass/fail per test number, then the amendment tests W1-W11
```
CLI: `equinox fetch | match | route --side yes|no [--as-of <RFC3339>] | serve [--addr :8080] | rebuild | watch | unlock [--holder <name>]`, all with `--config` (default config/equinox.yaml). Demo database: `go run ./scripts/seed [--db data/demo.db]`.

Amendments A1 to A9 at the end of docs/build-requirements.md add the `watch` scheduler, numbered cycles, the in-memory feed cache, the new-data banner and retention.

## Directory layout
```
cmd/equinox/                 CLI entrypoint, one file per command
internal/card/               Card, PriceRecord, MatchResult, LogRecord types; vocab loader; validation. No venue names.
internal/store/              SQLite schema and repositories (raw responses, cards, prices, log, fetch runs)
internal/fetch/kalshi/       Kalshi fetcher and normalizer (the only code that knows Kalshi formats)
internal/fetch/polymarket/   Polymarket US fetcher and normalizer (the only code that knows Polymarket formats)
internal/fetch/registry/     the only place fetchers are wired
internal/match/              matcher: same | opposite | not_same. No venue names, no I/O beyond store reads.
internal/route/              router: pure decision function plus a thin runner. No venue names, no clock, no network.
internal/feed/               read-only Reddit-style feed (templates + handlers)
internal/feed/cache/         in-memory snapshot built from SQLite for the latest completed cycle; atomic swap
internal/watch/              background scheduler: discovery loop, price loop, retention; holds the run lock
internal/testkit/             fixture transport, fault scenarios, fixed as_of, temp store (tests only)
internal/acceptance/          end-to-end tests over fixtures (required tests 16, 17, 19, 20)
internal/redteam/             red-team tests named TestRT_Rxx
scripts/guard/                make guard
scripts/acceptance/           make acceptance
scripts/seed/                 SYNTHETIC demo database, no venue called
config/equinox.yaml          venue roles, price age limit, storage path
config/demo.yaml             same, pointing at data/demo.db
config/vocab.yaml            shared vocabulary: subjects, deciders, cutoff units
fixtures/                    saved raw venue responses for tests
```

## Hard rules
1. Matcher and router never call a venue, never read the clock, never read `title` or `rules_text`. `make guard` greps for `net/http`, `time.Now`, and venue names in internal/match and internal/route.
2. No venue names in internal/card, internal/match, internal/route. Venue roles come from config/equinox.yaml (`reference_venue`, `counter_venue`). The reference venue is the side-of-reference and wins exact cost ties, which is Kalshi per the build requirements.
3. Empty means unknown. Unknown never equals anything, including another empty.
4. A value outside vocab.yaml is stored as empty. Fetchers never invent a value.
5. Prices are decimal strings parsed into an exact decimal type (integer micro-dollars). Never float64 for prices or fees.
6. Every raw venue response is saved before parsing. `equinox rebuild` must reproduce identical cards from saved raw responses.
7. The decision log is append-only. No UPDATE or DELETE on log tables (a test asserts this).
8. One bad record or one failed venue never stops a run. Skip, count, log a reason, continue.
9. Every fetcher normalization choice is documented in a comment next to the code that makes it, and listed in the fetcher's NORMALIZATION.md.
10. The feed cache is built only from SQLite and only from a fully completed cycle. It never calls a venue and never writes. Feed handlers read only from the cache.
11. watch and serve are separate processes. internal/watch never imports internal/feed and internal/feed never imports internal/watch; SQLite is the only handoff.
12. One writer at a time: `watch` and every writing CLI command take the same run lock; a second writer is refused with a reason.
13. Writing style in docs and comments: never use em dashes or double dashes; use a single hyphen.

## Definition of done for any task
- Every acceptance criterion in your task file is met and tested. Required tests carry the number from docs/build-requirements.md in their name, e.g. `TestReq05_GtVsGteIsNotSame`.
- `make test lint guard` pass.
- You only edited files listed under FILE OWNERSHIP in your task file. If you need another file, stop and ask the orchestrator.
- docs/status.md: one line added (task id, what merged, anything surprising).
- docs/ai-usage-log.md: one line added (harness, model, what the agent did, what a human changed).
- TASKS.md row updated.

## When unsure
Do not guess venue API behavior. Store the field as empty, log why, add the question to docs/assumptions.md under "Open", and continue.
