# ADR-006: No model in match or route

Status: Accepted. Requirements: NFR-9, NFR-1, FR-3. Build reference: locked choice 4.

## Context
AI is optional in the PRD. A language model or similarity score in the decision path would be non-deterministic and hard to validate. Financial firms are expected to document, validate and monitor any model used in decisions [SR 11-7, model risk], which is a heavy burden for a feasibility prototype and unnecessary when exact rules suffice.

## Decision
- No language model, embedding or similarity score in the matcher or router.
- AI is used only during development (planning, documentation and coding assistance), disclosed in docs/ai-usage-log.md.

## Options considered
| Option | Why rejected |
|---|---|
| LLM to extract cutoffs from rules text | Would improve recall on free-text markets, but makes card contents non-reproducible; fetcher rules plus fixtures keep it deterministic |
| LLM tie-breaker for ambiguous pairs | There are no ambiguous pairs by design; anything unclear is `not_same` |

## Consequences
- Recall depends on hand-written fetcher rules. A future version could use a model offline to propose vocabulary entries for human approval, without entering the decision path.

## Sources
[SR 11-7, model risk]
