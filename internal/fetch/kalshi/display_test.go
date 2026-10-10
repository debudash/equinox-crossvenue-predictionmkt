package kalshi_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/fetch/kalshi"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

// displayYAML matches fixtures/kalshi/categories. KXGDP and KXNHLGAME sit past
// their category bound and must never be requested.
const displayYAML = `
fetchers:
  kalshi:
    display_categories:
      - {name: Economics, max_events: 2, events_per_series: 1, series: [KXCPIYOY, KXPAYROLLS, KXGDP]}
      - {name: Sports, max_events: 3, events_per_series: 2, series: [KXNFLGAME, KXMLBGAME, KXNHLGAME]}
      - {name: Politics, max_events: 1, series: [KXBROKEN, KXTRUMPAPPROVE]}
`

// fetchDisplay runs one Fetch of the categories scenario with the display list above.
func fetchDisplay(t *testing.T) (*store.Store, int64, store.FetchRun, *testkit.Transport) {
	t.Helper()
	ctx := context.Background()
	d, err := kalshi.ParseDisplay([]byte(displayYAML))
	if err != nil {
		t.Fatal(err)
	}
	tr := testkit.Scenario(t, testkit.Fixture(t, "kalshi", "categories"))
	st := testkit.TempStore(t)
	cycleID, err := st.Cycles.Begin(ctx, store.KindFetch, store.TriggerCLI)
	if err != nil {
		t.Fatal(err)
	}
	f := kalshi.New(card.FetcherConfig{}, vocab(t), tr.Client(), testkit.FixedClock(testkit.AsOf))
	f.Timeout = 200 * time.Millisecond
	f.DisplayGap = 0
	f.Display = d
	run, err := f.Fetch(ctx, st, cycleID)
	if err != nil {
		t.Fatalf("Fetch returned a store error: %v", err)
	}
	return st, cycleID, run, tr
}

// Display-only categories: cards saved with every payoff field empty except
// the close_time deadline, the venue category kept, prices parsed exactly,
// and the fetch bounded per series and per category.
func TestKalshi_DisplayCategoriesSavedWithEmptyPayoff(t *testing.T) {
	ctx := context.Background()
	st, cycleID, run, tr := fetchDisplay(t)
	if run.Status != store.RunOK {
		t.Fatalf("run status %s (%s); a failed display series must not fail the run", run.Status, run.Error)
	}
	for _, r := range tr.Requests() {
		if strings.Contains(r, "KXGDP") || strings.Contains(r, "KXNHLGAME") {
			t.Errorf("series past its category bound was requested: %s", r)
		}
	}

	cards, err := st.Cards.LatestAll(ctx, store.Writing(cycleID))
	if err != nil {
		t.Fatal(err)
	}
	perCategory := map[string]int{}
	display := 0
	for _, c := range cards {
		if strings.HasPrefix(c.MarketID, "KXFED-") {
			continue // the synthetic Fed pair
		}
		display++
		perCategory[c.Record.Category]++
		if !c.Payoff.Subject.IsEmpty() || !c.Payoff.Decider.IsEmpty() || !c.Payoff.CutoffOp.IsEmpty() ||
			!c.Payoff.CutoffValue.IsEmpty() || !c.Payoff.CutoffUnit.IsEmpty() || !c.Payoff.UnclearResult.IsEmpty() {
			t.Errorf("%s: display card has payoff fields: %+v", c.MarketID, c.Payoff.Fields())
		}
		if c.Record.Title == "" || c.Record.EventTitle == "" || c.Record.RulesText == "" || c.Record.URL == "" {
			t.Errorf("%s: display fields missing: %+v", c.MarketID, c.Record)
		}
	}
	// Events: CPI 1 (21 markets), payrolls 1 (13), NFL 2 (4), MLB 1 (2), Trump approval 1 (9).
	want := map[string]int{"Economics": 34, "Sports": 6, "Politics": 9}
	if display != 49 || !reflect.DeepEqual(perCategory, want) {
		t.Errorf("display cards %d per category %v, want 49 %v", display, perCategory, want)
	}

	nfl, found, err := st.Cards.Latest(ctx, card.MarketRef{Venue: kalshi.Venue, MarketID: "KXNFLGAME-26OCT11PHIJAC-JAC"}, store.Writing(cycleID))
	if err != nil || !found {
		t.Fatalf("NFL card not saved: %v", err)
	}
	wantFields(t, nfl, map[string]string{
		"subject": "", "cutoff_op": "", "cutoff_value": "", "cutoff_unit": "",
		"deadline": "2026-10-13T13:30:00Z", "decider": "", "unclear_result": "",
	})
	if nfl.Record.Title != "Jacksonville wins" || nfl.Record.EventTitle != "PHI Eagles vs JAC Jaguars" ||
		nfl.Record.Category != "Sports" || nfl.Record.Volume24h != "94707.69" || nfl.Record.URL != "https://kalshi.com/markets/kxnflgame" {
		t.Errorf("record fields: %+v", nfl.Record)
	}

	prices, err := st.Prices.LatestAll(ctx, store.Writing(cycleID))
	if err != nil {
		t.Fatal(err)
	}
	wantPrice(t, price(t, prices, "KXNFLGAME-26OCT11PHIJAC-JAC", card.Yes), "0.77")
	wantPrice(t, price(t, prices, "KXNFLGAME-26OCT11PHIJAC-JAC", card.No), "0.24")
	// A yes ask of 1.0000 means no offer: stored empty, never as a price.
	wantPrice(t, price(t, prices, "KXCPIYOY-26SEP-T3.0", card.Yes), "")
	wantPrice(t, price(t, prices, "KXCPIYOY-26SEP-T3.0", card.No), "0.01")
	wantPrice(t, price(t, prices, "KXPAYROLLS-26OCT-T-25000", card.Yes), "0.94")
	wantPrice(t, price(t, prices, "KXPAYROLLS-26OCT-T-25000", card.No), "0.08")

	// Skips: the HTTP 500 series, and one "display bound" row for each series
	// that had more open events than were requested.
	skips, err := st.Skips.ForCycle(ctx, cycleID)
	if err != nil {
		t.Fatal(err)
	}
	var failed, bound int
	for _, k := range skips {
		switch {
		case strings.Contains(k.Reason, "display series KXBROKEN (Politics) not fetched"):
			failed++
		case strings.Contains(k.Reason, "(display bound)"):
			bound++
		default:
			t.Errorf("unexpected skip: %s", k.Reason)
		}
	}
	if failed != 1 || bound != 4 || run.Skipped != 5 {
		t.Errorf("skips: %d failed, %d bound, run.Skipped %d; want 1, 4, 5", failed, bound, run.Skipped)
	}
	if run.Saved != 51 {
		t.Errorf("run.Saved = %d, want 51 cards (49 display, 2 synthetic Fed)", run.Saved)
	}
}

// Rebuild reproduces display cards from the saved raw responses alone: the
// event bound is in the saved endpoint, not in config.
func TestKalshi_DisplayCardsRebuildFromRaw(t *testing.T) {
	ctx := context.Background()
	st, cycleID, _, _ := fetchDisplay(t)
	raws, err := st.Raw.List(ctx, store.RawMarkets)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, raw := range raws {
		cards, _, _ := kalshi.NormalizeMarkets(raw, vocab(t))
		for _, c := range cards {
			saved, found, err := st.Cards.Latest(ctx, c.Ref(), store.Writing(cycleID))
			if err != nil || !found {
				t.Fatalf("%s not saved: %v", c.MarketID, err)
			}
			sr, cr := saved.Record, c.Record
			sameTime := sr.FetchedAt.Equal(cr.FetchedAt)
			sr.FetchedAt, cr.FetchedAt = time.Time{}, time.Time{}
			if !reflect.DeepEqual(saved.Payoff.Fields(), c.Payoff.Fields()) || sr != cr || !sameTime {
				t.Errorf("%s: rebuild differs from fetch", c.MarketID)
			}
			n++
		}
	}
	if n != 51 {
		t.Errorf("rebuilt %d cards, want 51", n)
	}
}

func TestKalshi_ParseDisplayDefaultsAndRefusals(t *testing.T) {
	d, err := kalshi.ParseDisplay([]byte("fetchers:\n  kalshi:\n    display_categories:\n      - {name: X, series: [KXCPI]}\n"))
	if err != nil || len(d) != 1 || d[0].MaxEvents != 10 || d[0].EventsPerSeries != 10 {
		t.Fatalf("defaults: %+v %v", d, err)
	}
	if d, err := kalshi.ParseDisplay([]byte("fetchers:\n  polymarket: {}\n")); err != nil || len(d) != 0 {
		t.Fatalf("no section must give no display list: %+v %v", d, err)
	}
	for _, bad := range []string{
		"fetchers:\n  kalshi:\n    display_categories:\n      - {name: Fed, series: [KXFED]}\n",
		"fetchers:\n  kalshi:\n    display_categories:\n      - {name: Empty}\n",
		"fetchers:\n  kalshi:\n    display_categories:\n      - {name: Big, events_per_series: 500, series: [KXCPI]}\n",
	} {
		if _, err := kalshi.ParseDisplay([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
