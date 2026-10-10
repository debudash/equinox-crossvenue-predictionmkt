// Package redteam holds one test per case in docs/red-team.md whose Test
// column names a TestRT_Rxx id. Each test drives the real code paths (the
// venue normalizers over fixtures, the store, match.Run and route.Run) and
// never calls a venue. Cases mapped to a required test (R-01 to R-03, R-08,
// R-13 to R-17) or to an amendment test (W tests) are covered there.
package redteam
