package registry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/fetch/kalshi"
	"equinox/internal/fetch/polymarket"
	"equinox/internal/match"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

// displayYAML matches fixtures/kalshi/categories and fixtures/polymarket/categories.
const displayYAML = `
fetchers:
  kalshi:
    display_categories:
      - {name: Economics, max_events: 2, events_per_series: 1, series: [KXCPIYOY, KXPAYROLLS]}
      - {name: Sports, max_events: 3, events_per_series: 2, series: [KXNFLGAME, KXMLBGAME]}
      - {name: Politics, max_events: 1, series: [KXBROKEN, KXTRUMPAPPROVE]}
  polymarket:
    display_categories:
      - {category: politics, limit: 3}
      - {category: crypto, limit: 2}
      - {category: finance, limit: 2}
      - {category: climate, limit: 2}
`

// Display-only cards never match. Both venues' display categories are fetched
// next to the synthetic Fed pair (one same, one opposite); the matcher finds
// exactly that pair and no match record names a display card.
func TestDisplayCategoriesNeverMatch(t *testing.T) {
	ctx := context.Background()
	cfg, v := loadConfig(t)
	path := filepath.Join(t.TempDir(), "equinox.yaml")
	if err := os.WriteFile(path, []byte(displayYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	clock := testkit.FixedClock(testkit.AsOf.Add(-2 * time.Minute))
	kf := kalshi.New(card.FetcherConfig{}, v, testkit.Scenario(t, testkit.Fixture(t, "kalshi", "categories")).Client(), clock)
	kf.DisplayGap = 0
	pf := polymarket.New(card.FetcherConfig{}, v, testkit.Scenario(t, testkit.Fixture(t, "polymarket", "categories")).Client(), clock)
	fs := []Fetcher{kf, pf}
	if err := LoadDisplay(path, fs); err != nil {
		t.Fatal(err)
	}
	if len(kf.Display) != 3 || len(pf.Display) != 4 {
		t.Fatalf("LoadDisplay: kalshi %d, polymarket %d categories", len(kf.Display), len(pf.Display))
	}

	st := testkit.TempStore(t)
	cyc, err := st.Cycles.Begin(ctx, store.KindFetch, store.TriggerCLI)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range FetchAll(ctx, st, cyc, fs, time.Minute) {
		if r.Err != nil || r.Run.Status != store.RunOK {
			t.Fatalf("%s: %v %s %s", r.Venue, r.Err, r.Run.Status, r.Run.Error)
		}
	}
	cards, err := st.Cards.LatestAll(ctx, store.Writing(cyc))
	if err != nil {
		t.Fatal(err)
	}
	fed := map[card.MarketRef]bool{}
	display := 0
	for _, c := range cards {
		if c.Payoff.Subject.IsEmpty() {
			display++
			if !c.Payoff.Decider.IsEmpty() {
				t.Errorf("%s: display card has a decider", c.MarketID)
			}
			continue
		}
		fed[c.Ref()] = true
	}
	if display != 49+9 || len(fed) != 4 {
		t.Fatalf("%d display cards and %d Fed cards, want 58 and 4", display, len(fed))
	}

	sum, err := match.Run(ctx, st, cyc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Same != 1 || sum.Opposite != 1 {
		t.Errorf("same %d opposite %d, want the synthetic pair only (1 and 1)", sum.Same, sum.Opposite)
	}
	recs, err := st.Log.List(ctx, store.Writing(cyc))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if !fed[r.MarketA] || !fed[r.MarketB] {
			t.Errorf("match record names a display card: %s / %s (%s)", r.MarketA.MarketID, r.MarketB.MarketID, r.Match)
		}
	}
	if len(recs) != sum.Compared || sum.Compared != 4 {
		t.Errorf("%d records, %d compared; want 4 Fed pairs only", len(recs), sum.Compared)
	}
}

// The shipped config parses: every display category is valid and no Fed
// series or macro page is listed as display-only.
func TestShippedConfigDisplayCategoriesParse(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(testkit.RepoRoot(t), "config", "equinox.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	k, err := kalshi.ParseDisplay(b)
	if err != nil || len(k) == 0 {
		t.Fatalf("kalshi: %d categories, %v", len(k), err)
	}
	p, err := polymarket.ParseDisplay(b)
	if err != nil || len(p) == 0 {
		t.Fatalf("polymarket: %d categories, %v", len(p), err)
	}
	for _, d := range k {
		for _, s := range d.Series {
			if strings.HasPrefix(s, "KXFED") {
				t.Errorf("Fed-like series %s listed as display-only", s)
			}
		}
	}
}
