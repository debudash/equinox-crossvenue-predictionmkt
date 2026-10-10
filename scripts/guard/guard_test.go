package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestGuard(t *testing.T) {
	cases := []struct {
		name string
		file string
		body string
		want string // substring of the expected violation; empty means clean
	}{
		{"clean match", "internal/match/m.go", "package match\n\nfunc Compare() {}\n", ""},
		{"venue in match", "internal/match/m.go", "package match\n\nconst v = \"kalshi\"\n", `venue name "kalshi"`},
		{"venue in route, mixed case", "internal/route/r.go", "package route\n\n// Polymarket wins\n", `venue name "polymarket"`},
		{"venue in match test file", "internal/match/m_test.go", "package match\n\nvar x = \"KALSHI\"\n", `venue name "kalshi"`},
		{"net/http in route", "internal/route/r.go", "package route\n\nimport _ \"net/http\"\n", `network import "net/http"`},
		{"net in match", "internal/match/m.go", "package match\n\nimport _ \"net\"\n", `network import "net"`},
		{"time.Now in route", "internal/route/r.go", "package route\n\nimport \"time\"\n\nvar t = time.Now()\n", "clock read time.Now"},
		{"aliased time.Since in match", "internal/match/m.go", "package match\n\nimport tm \"time\"\n\nvar d = tm.Since(tm.Time{})\n", "clock read time.Since"},
		{"dot import of time", "internal/route/r.go", "package route\n\nimport . \"time\"\n\nvar d Duration\n", "dot import"},
		{"time type use is fine", "internal/route/r.go", "package route\n\nimport \"time\"\n\nfunc Decide(asOf time.Time) time.Duration { return asOf.Sub(asOf) }\n", ""},
		{"venue in card", "internal/card/c.go", "package card\n\nconst v = \"polymarket\"\n", `venue name "polymarket"`},
		{"clock read in card is not guarded", "internal/card/c.go", "package card\n\nimport \"time\"\n\nvar t = time.Now()\n", ""},
		{"venue in fetcher is allowed", "internal/fetch/kalshi/k.go", "package kalshi\n\nconst v = \"kalshi\"\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{tc.file: tc.body})
			vs, err := Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(vs) != 0 {
					t.Fatalf("want clean, got %v", vs)
				}
				return
			}
			for _, v := range vs {
				if strings.Contains(v.What, tc.want) {
					return
				}
			}
			t.Fatalf("want violation containing %q, got %v", tc.want, vs)
		})
	}
}

func TestGuardPassesOnRepo(t *testing.T) {
	vs, err := Check(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("repository violates the guard: %v", vs)
	}
}
