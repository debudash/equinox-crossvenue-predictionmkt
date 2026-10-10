# ADR-005: Normalize into one shared vocabulary

Status: Accepted. Requirements: FR-2, FR-4, NFR-5, NFR-8. Build reference: locked choices 7 and 8, "Market card and price record".

## Context
Equality between cards is only meaningful if both fetchers use the same words and the same structure. Free-text cutoffs and source names from two venues rarely match as strings.

## Decision
- config/vocab.yaml lists every allowed subject, decider and cutoff unit. V1 seeds Federal Reserve rate markets only.
- Fetchers map venue data into the vocabulary and the structured cutoff. A value outside the vocabulary is stored as empty and never matches.
- cutoff_value is a decimal string with no trailing zeros; deadlines are ISO 8601 in UTC.
- Only fetchers know venue formats. Each documents every normalization choice next to the code and in its NORMALIZATION.md (deadline source, unclear_result mapping, skipped record types such as Kalshi multivariate combos).
- Buy prices are taken from executable quotes. An empty book side is stored as an empty price, never as 0 or 1.
- Coverage grows only by adding vocabulary entries plus fetcher rules, each with a fixture test.

## Options considered
| Option | Why rejected |
|---|---|
| Fuzzy string normalization in the matcher | Moves venue knowledge into the matcher and adds non-determinism |
| Per-venue vocabularies with a mapping table | Two sources of truth for one concept |

## Consequences
- Test 16 proves both fetchers produce byte-equal payoff fields for the same Fed market.
- Vocabulary size is the main lever for recall, and its coverage is stated in the README.
