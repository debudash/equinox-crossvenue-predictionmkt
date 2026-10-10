# Start here

This folder is the complete planning pack for the Equinox buildout. Drop it into an empty repo and hand it to Claude Code (or Agent Orchestrator).

## 1. Set up the repo
```
mkdir equinox && cd equinox
git init
# copy the contents of this folder here, so AGENTS.md sits at the repo root
git add . && git commit -m "Planning pack"
```

## 2. Start the build
Open Claude Code in the repo root and paste the contents of kickoff.md as the first message. It reads AGENTS.md, docs/build-requirements.md, docs/architecture.md, docs/plan.md and TASKS.md, then runs Wave 0.

## 3. Your checkpoints
The agent stops at each wave boundary and reports merged tasks, failing checks, decisions and which required tests pass. Approve before it continues.

| Wave | Tasks | What to check before approving |
|---|---|---|
| 0 | T-00 to T-02 | `make guard` exists and fails on a venue name in match or route; schema matches docs/architecture.md |
| 1 | T-10 to T-13 | Tests 1 to 15 pass; both NORMALIZATION.md files exist; the Fed-pair gate (D-18) is reported |
| 2 | T-20 to T-22 | Tests 16 to 20 pass; feed matches the wireframes |
| 3 | T-23 | W1 to W11 pass; watch and serve run as separate processes |
| 4 | T-30 | README verdict uses measured numbers only; citations marked Verify are checked |

## 4. Still open
- No decisions are pending. Every entry in docs/decisions.md is Locked or Approved.
- Wave 1 gate (D-18): the agent must show at least one real Fed market pair on Kalshi and Polymarket US that fits the cutoff format before Wave 2. If none exists, you decide whether to fall back to Polymarket International (amendment A9).

## What is in the pack
| Path | Purpose |
|---|---|
| AGENTS.md, CLAUDE.md | Rules every coding agent follows |
| kickoff.md | First message for the orchestrator |
| docs/build-requirements.md | Authoritative requirements plus approved amendments A1 to A9 |
| docs/decisions.md | Decision log |
| docs/architecture.md | HLD and LLD diagrams |
| docs/adr/ | ADR-001 to ADR-008 |
| docs/tasks/, TASKS.md, docs/plan.md | Task specs, board, wave plan |
| docs/traceability.md | Every required and amendment test mapped to a task |
| config/ | vocab.yaml (Fed markets) and equinox.yaml (venue roles, intervals) |
| docs/spec.md, docs/citations.md | Card model as an RFC draft; sources |
| docs/metrics.md, docs/red-team.md, evals/ | Evidence plan |
| Demo_Script.md, README.md, docs/deployment.md | Demo and submission material |
| Wireframes | https://claude.ai/artifact/TghfanvCGFAfjUs5bsRk34 |
