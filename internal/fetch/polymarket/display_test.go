package polymarket

import (
	"context"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

// displayYAML matches fixtures/polymarket/categories.
const displayYAML = `
fetchers:
  polymarket:
    display_categories:
      - {category: politics, limit: 3}
      - {category: crypto, limit: 2}
      - {category: finance, limit: 2}
      - {category: climate, limit: 2}
`

func fetchDisplay(t *testing.T) (*store.Store, int64, store.FetchRun, *testkit.Transport) {
	t.Helper()
	ctx := context.Background()
	d, err := ParseDisplay([]byte(displayYAML))
	require.NoError(t, err)
	tr := testkit.Scenario(t, testkit.Fixture(t, "polymarket", "categories"))
	f := New(card.FetcherConfig{}, vocab(t), tr.Client(), testkit.FixedClock(fetchedAt))
	f.gap = 0
	f.sleep = func(context.Context, time.Duration) error { return nil }
	f.Display = d
	st := testkit.TempStore(t)
	cyc, err := st.Cycles.Begin(ctx, store.KindDiscovery, store.TriggerCLI)
	require.NoError(t, err)
	run, err := f.Fetch(ctx, st, cyc)
	require.NoError(t, err)
	return st, cyc, run, tr
}

// Display-only categories: cards saved with every payoff field empty except a
// deadline read from the rules, the category given a display name, one request
// per category and no order book calls, prices read exactly from the listing's
// best quotes.
func TestPolymarket_DisplayCategoriesSavedWithEmptyPayoff(t *testing.T) {
	st, cyc, run, tr := fetchDisplay(t)
	require.Equal(t, store.RunOK, run.Status, run.Error)

	cards := latestCards(t, st, cyc)
	perCategory := map[string]int{}
	for id, c := range cards {
		if strings.HasPrefix(id, "synthetic-") {
			continue // the synthetic Fed pair on the macro page
		}
		perCategory[c.Record.Category]++
		p := c.Payoff
		require.True(t, p.Subject.IsEmpty() && p.Decider.IsEmpty() && p.CutoffOp.IsEmpty() && p.CutoffValue.IsEmpty() &&
			p.CutoffUnit.IsEmpty() && p.UnclearResult.IsEmpty(), "%s has payoff fields: %+v", id, p.Fields())
		require.True(t, c.Record.Volume24h == "" && c.Record.URL == "", "%s: no volume or url in the payload", id)
	}
	require.Equal(t, map[string]int{"Politics": 3, "Crypto": 2, "Financials": 2, "Climate and Weather": 2}, perCategory)

	rev := cards["ipcc-2026ipos-revolut"]
	require.Equal(t, "Revolut", rev.Record.Title)
	require.Equal(t, "Which Companies Will Confirm an IPO in 2026", rev.Record.EventTitle)
	require.True(t, strings.HasPrefix(rev.Record.RulesText, "If Revolut confirms an IPO by December 31, 2026, 11:59 PM ET"))
	d, ok := rev.Payoff.Deadline.Get()
	require.True(t, ok, "deadline from the one ET moment in the rules")
	require.Equal(t, "2027-01-01T04:59:00Z", d.String())
	// Bitcoin rules put the time before the date ("12:00 AM ET on January 1, 2027"): not the parsed form, so empty.
	require.True(t, cards["cpc-btc-150k-12-31-2026"].Payoff.Deadline.IsEmpty())

	// One request per category, no book calls for display markets.
	pages, books := 0, 0
	for _, r := range tr.Requests() {
		switch {
		case strings.Contains(r, "/book") && !strings.Contains(r, "synthetic-"):
			books++
		case strings.Contains(r, "categories=") && !strings.Contains(r, "categories=macro"):
			pages++
		}
	}
	require.Equal(t, 4, pages)
	require.Equal(t, 0, books)

	prices := latestPrices(t, st, cyc)
	want := map[string]card.Micros{
		"paccc-usho-midterms-2026-11-03-dem/yes": 891_000, "paccc-usho-midterms-2026-11-03-dem/no": 110_000, // 1 - 0.8900
		"paccc-usho-midterms-2026-11-03-rep/yes": 120_000, "paccc-usho-midterms-2026-11-03-rep/no": 881_000,
		"paccc-usse-midterms-2026-11-03-rep/yes": 411_000, "paccc-usse-midterms-2026-11-03-rep/no": 590_000,
		"cpc-btc-150k-12-31-2026/yes": 50_000, "cpc-btc-150k-12-31-2026/no": 960_000,
		"ipcc-2026ipos-revolut/yes": 30_000, "ipcc-2026ipos-revolut/no": 980_000,
	}
	for k, v := range want {
		p, found := prices[k]
		require.True(t, found, "no price %s", k)
		require.Equal(t, v, mustGet(t, p.BuyPrice), k)
		require.True(t, p.Fee.IsEmpty(), "fee is empty")
		require.Equal(t, fetchedAt, p.PricedAt)
	}
	// The climate markets list no quotes (their book was empty when recorded):
	// both prices empty, never 0 or 1.
	require.True(t, prices["trc-yr-httst-rnk-glbl-2026-lte5th/yes"].BuyPrice.IsEmpty())
	require.True(t, prices["trc-yr-httst-rnk-glbl-2026-lte5th/no"].BuyPrice.IsEmpty())
	require.Equal(t, 2*9+2*2, len(prices), "two price records per display card plus the synthetic pair's book prices")

	// Synthetic Fed pair still fetched with its payoff fields.
	fed := cards["synthetic-fedfnds-2026-12-09-gt3pt75"]
	require.Equal(t, "fed_funds_upper_bound", mustStr(t, fed.Payoff.Subject))
}

// A display page that stays rate limited is one skip after the backoff and the
// extra Retry-After wait; the run stays ok and the Fed cards stay.
func TestPolymarket_DisplayPageFailureIsSkipNotFailedRun(t *testing.T) {
	ctx := context.Background()
	d, err := ParseDisplay([]byte("fetchers:\n  polymarket:\n    display_categories:\n      - {category: sports, limit: 2}\n"))
	require.NoError(t, err)
	tr := testkit.Scenario(t, testkit.Fixture(t, "polymarket", "categories")) // the sports page always answers 429
	f := New(card.FetcherConfig{}, vocab(t), tr.Client(), testkit.FixedClock(fetchedAt))
	f.gap = 0
	var waits []time.Duration
	f.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	f.Display = d
	st := testkit.TempStore(t)
	cyc, err := st.Cycles.Begin(ctx, store.KindDiscovery, store.TriggerCLI)
	require.NoError(t, err)
	run, err := f.Fetch(ctx, st, cyc)
	require.NoError(t, err)
	require.Equal(t, store.RunOK, run.Status)
	skips, err := st.Skips.ForCycle(ctx, cyc)
	require.NoError(t, err)
	found := false
	for _, k := range skips {
		found = found || strings.HasPrefix(k.Reason, "display category sports not fetched")
	}
	require.True(t, found, "skip with reason")
	require.Len(t, latestCards(t, st, cyc), 2)
	// 1 s, 2 s, 4 s, then the 10 s Retry-After wait, then 1 s, 2 s, 4 s on the last try.
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 10 * time.Second, time.Second, 2 * time.Second, 4 * time.Second}
	require.Equal(t, want, waits)
}

// Rebuild takes the display path from the saved endpoint alone, and a Fed
// sentence inside a display page never gives a subject or decider.
func TestPolymarket_DisplayPathChosenByEndpoint(t *testing.T) {
	body := []byte(`{"markets":[{"slug":"x","question":"Q","title":"T","category":"finance",
	  "description":"If the upper bound of the target federal funds rate is greater than 3.75% on December 9, 2026 at 2:00 PM ET. Outcome sourced from the United States Federal Reserve.",
	  "marketSides":[{"description":"Yes","long":true},{"description":"No","long":false}]}]}`)
	raw := store.Raw{ID: 3, Venue: Venue, Kind: store.RawMarkets, FetchedAt: fetchedAt, Body: body,
		Endpoint: "/v1/markets?" + displayQuery("finance", 2)}
	cards, _, skips := NormalizeMarkets(raw, vocab(t))
	require.Empty(t, skips)
	require.Len(t, cards, 1)
	p := cards[0].Payoff
	require.True(t, p.Subject.IsEmpty() && p.Decider.IsEmpty() && p.CutoffOp.IsEmpty(), "display page: %+v", p.Fields())
	require.Equal(t, "Financials", cards[0].Record.Category)

	// The same body on the macro page (or with no endpoint) is the Fed path.
	for _, ep := range []string{"", "/v1/markets?" + marketsQuery(0)} {
		raw.Endpoint = ep
		cards, _, _ = NormalizeMarkets(raw, vocab(t))
		require.Len(t, cards, 1)
		require.Equal(t, "fed_funds_upper_bound", mustStr(t, cards[0].Payoff.Subject), ep)
	}
}

func TestPolymarket_ParseDisplayDefaultsAndRefusals(t *testing.T) {
	d, err := ParseDisplay([]byte("fetchers:\n  polymarket:\n    display_categories:\n      - {category: politics}\n"))
	require.NoError(t, err)
	require.Equal(t, 10, d[0].Limit)
	d, err = ParseDisplay([]byte("fetchers:\n  kalshi: {}\n"))
	require.NoError(t, err)
	require.Len(t, d, 0)
	for _, bad := range []string{
		"fetchers:\n  polymarket:\n    display_categories:\n      - {category: macro}\n",
		"fetchers:\n  polymarket:\n    display_categories:\n      - {limit: 5}\n",
		"fetchers:\n  polymarket:\n    display_categories:\n      - {category: politics, limit: 500}\n",
	} {
		_, err := ParseDisplay([]byte(bad))
		require.True(t, err != nil, "accepted %q", bad)
	}
	require.Equal(t, "Climate and Weather", displayCategory("climate"))
	require.Equal(t, "Sports", displayCategory("sports"))
}

func mustStr(t *testing.T, o card.Opt[string]) string {
	t.Helper()
	v, ok := o.Get()
	require.True(t, ok)
	return v
}

// Listing quotes are read exactly; a bad quote empties that side and is
// logged; a market that is not open has no prices.
func TestPolymarket_DisplayPricesFromListingQuotes(t *testing.T) {
	raw := store.Raw{ID: 9, Venue: Venue, FetchedAt: fetchedAt}
	ps, ks := displayPrices([]byte(`{"status":"MARKET_STATUS_OPEN","bestAskQuote":{"value":"0.3350","currency":"USD"},"bestBidQuote":{"value":"0.3301","currency":"USD"}}`), "m", raw)
	require.Empty(t, ks)
	require.Equal(t, card.Micros(335_000), mustGet(t, ps[0].BuyPrice))
	require.Equal(t, card.Micros(669_900), mustGet(t, ps[1].BuyPrice), "1 - 0.3301 in integer micro-dollars")

	ps, ks = displayPrices([]byte(`{"status":"MARKET_STATUS_OPEN","bestAskQuote":{"value":"1.5","currency":"USD"},"bestBidQuote":{"value":"0.2","currency":"EUR"}}`), "m", raw)
	require.Len(t, ks, 2)
	require.True(t, ps[0].BuyPrice.IsEmpty() && ps[1].BuyPrice.IsEmpty())

	ps, _ = displayPrices([]byte(`{"status":"MARKET_STATUS_HALTED","bestAskQuote":{"value":"0.6","currency":"USD"},"bestBidQuote":{"value":"0.4","currency":"USD"}}`), "m", raw)
	require.True(t, ps[0].BuyPrice.IsEmpty() && ps[1].BuyPrice.IsEmpty())
}
