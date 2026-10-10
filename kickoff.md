# Kickoff message for Claude Code (or the AO project orchestrator)

Paste everything below the line as your first message.

---

You are building Equinox. Read in this order before writing code: AGENTS.md, docs/build-requirements.md (authoritative), docs/architecture.md, docs/plan.md, TASKS.md.

Operating rules:
1. Execute one wave at a time per docs/plan.md. A wave starts only when the previous wave is merged and `make test lint guard` passes.
2. One worker per task file in docs/tasks/. Give each worker its task file, AGENTS.md, and docs/build-requirements.md. Nothing else unless the task lists it.
3. Enforce FILE OWNERSHIP. If a worker needs a file it does not own, pause and ask me.
4. Wave 0 sets the contracts (types, schema, vocab, config). Any later change to internal/card or the SQLite schema is a contract change: stop and ask me.
5. Reject any proposal that adds a similarity score, a confidence value, a language model, a venue name in matcher or router, a clock read in the router, or float math for prices. Cite AGENTS.md.
6. Tests never call a live venue. Use fixtures/.
7. Keep TASKS.md and docs/status.md current after every merge.
8. At each wave boundary, stop and post: merged tasks, open PRs, failing checks, decisions made, assumptions added, and which of the 20 required tests now pass. Wait for my approval.

Wave 1 has a gate (D-18): before starting Wave 2, report whether at least one real Fed market pair fits the cutoff format on Kalshi and Polymarket US. If not, stop and wait for my decision.

Start with Wave 0: T-00, then T-01, then T-02.
