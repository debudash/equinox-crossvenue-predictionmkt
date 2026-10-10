package polymarket

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

// fetchedAt is the fixed clock for fetcher tests.
var fetchedAt = testkit.AsOf.Add(-time.Minute)

func vocab(t *testing.T) *card.Vocab {
	t.Helper()
	v, err := card.LoadVocab(filepath.Join(testkit.RepoRoot(t), "config", "vocab.yaml"))
	require.NoError(t, err)
	return v
}

func newFetcher(t *testing.T, scenario string, v *card.Vocab) *Fetcher {
	t.Helper()
	tr := testkit.Scenario(t, testkit.Fixture(t, "polymarket", scenario))
	f := New(card.FetcherConfig{Source: "polymarket_us", BaseURL: "https://gateway.polymarket.us"}, v, tr.Client(), testkit.FixedClock(fetchedAt))
	f.gap = 0
	f.sleep = func(context.Context, time.Duration) error { return nil }
	return f
}

// fetch opens a cycle on a fresh store and runs one full fetch.
func fetch(t *testing.T, scenario string, v *card.Vocab) (*store.Store, int64, store.FetchRun) {
	t.Helper()
	ctx := context.Background()
	st := testkit.TempStore(t)
	cyc, err := st.Cycles.Begin(ctx, store.KindDiscovery, store.TriggerCLI)
	require.NoError(t, err)
	run, err := newFetcher(t, scenario, v).Fetch(ctx, st, cyc)
	require.NoError(t, err)
	return st, cyc, run
}

func latestCards(t *testing.T, st *store.Store, cyc int64) map[string]card.Card {
	t.Helper()
	cs, err := st.Cards.LatestAll(context.Background(), store.Writing(cyc))
	require.NoError(t, err)
	out := map[string]card.Card{}
	for _, c := range cs {
		out[c.MarketID] = c
	}
	return out
}

func latestPrices(t *testing.T, st *store.Store, cyc int64) map[string]card.PriceRecord {
	t.Helper()
	ps, err := st.Prices.LatestAll(context.Background(), store.Writing(cyc))
	require.NoError(t, err)
	out := map[string]card.PriceRecord{}
	for _, p := range ps {
		out[p.MarketID+"/"+string(p.Side)] = p
	}
	return out
}

func fieldMap(c card.Card) map[string]string {
	m := map[string]string{}
	for _, f := range c.Payoff.Fields() {
		if f.Set {
			m[f.Name] = f.Value
		}
	}
	return m
}

func micros(t *testing.T, o card.Opt[card.Micros]) string {
	t.Helper()
	v, ok := o.Get()
	if !ok {
		return "empty"
	}
	return v.String()
}

// Test 16 input: the real Polymarket US Fed fixture normalizes to the full
// payoff fields of a threshold market, with prices from its order book.
func TestPolymarket_Req16_FedFixtureNormalizesThresholdMarket(t *testing.T) {
	st, cyc, run := fetch(t, "fed", vocab(t))
	require.Equal(t, store.RunOK, run.Status, run.Error)

	cards := latestCards(t, st, cyc)
	require.Len(t, cards, 22, "22 Federal Reserve markets in the recording")

	c := cards["rvcus-fedfnds-ondt-2027-12-31-gte3pt50"]
	require.Equal(t, Venue, c.Venue)
	require.Equal(t, map[string]string{
		"subject":      "fed_funds_upper_bound",
		"cutoff_op":    "gte",
		"cutoff_value": "3.5",
		"cutoff_unit":  "percent",
		"deadline":     "2028-01-01T04:59:00Z", // December 31, 2027 at 11:59 PM ET (EST, UTC-5)
		"decider":      "federal_reserve",
		// unclear_result is empty (D-20), so fieldMap omits it.
	}, fieldMap(c))
	require.Equal(t, "3.50% Or Above", c.Record.Title)
	require.Equal(t, "Fed Funds Rate at the End of 2027?", c.Record.EventTitle)
	require.Equal(t, "macro", c.Record.Category)
	require.True(t, strings.HasPrefix(c.Record.RulesText, "This market will settle to Yes if the upper bound"))
	require.Equal(t, fetchedAt, c.Record.FetchedAt)

	prices := latestPrices(t, st, cyc)
	// Book: best offer 0.99, best bid 0.01. YES = best offer; NO = 1 - best bid.
	yes, no := prices[c.MarketID+"/yes"], prices[c.MarketID+"/no"]
	require.Equal(t, "0.99", micros(t, yes.BuyPrice))
	require.Equal(t, "0.99", micros(t, no.BuyPrice))
	require.True(t, yes.Fee.IsEmpty() && no.Fee.IsEmpty(), "fee is not exactly derivable per contract")
	require.Equal(t, fetchedAt, yes.PricedAt)

	// A decision-bucket market with a deep book.
	nc := latestPrices(t, st, cyc)
	require.Equal(t, "0.27", micros(t, nc["rdc-usfed-fomc-2026-12-09-nochng/yes"].BuyPrice))
	require.Equal(t, "0.74", micros(t, nc["rdc-usfed-fomc-2026-12-09-nochng/no"].BuyPrice))
}

// An empty side of the book is stored as empty, never 0 or 1.
func TestPolymarket_EmptyBookSideIsEmptyPrice(t *testing.T) {
	st, cyc, _ := fetch(t, "fed", vocab(t))
	prices := latestPrices(t, st, cyc)
	// gte3pt25 had no offers when recorded, and one bid at 0.01.
	require.Equal(t, "empty", micros(t, prices["rvcus-fedfnds-ondt-2027-12-31-gte3pt25/yes"].BuyPrice))
	require.Equal(t, "0.99", micros(t, prices["rvcus-fedfnds-ondt-2027-12-31-gte3pt25/no"].BuyPrice))
}

// Decision buckets ("No change", "25 bps Decrease") and change markets get an
// empty cutoff, so they never match.
func TestPolymarket_DecisionBucketHasEmptyCutoff(t *testing.T) {
	st, cyc, _ := fetch(t, "fed", vocab(t))
	cards := latestCards(t, st, cyc)
	for _, id := range []string{
		"rdc-usfed-fomc-2026-12-09-nochng",
		"rdc-usfed-fomc-2026-12-09-cut25",
		"rdc-usfed-fomc-2026-10-28-hike50",
		"cbpac-fed-gt25bps-2026-12-31",
		"cbpac-usfed-2026-cut",
	} {
		c, ok := cards[id]
		require.True(t, ok, id)
		require.True(t, c.Payoff.CutoffOp.IsEmpty(), id)
		require.True(t, c.Payoff.CutoffValue.IsEmpty(), id)
		require.True(t, c.Payoff.CutoffUnit.IsEmpty(), id)
	}
	// The FOMC bucket rules name a meeting but no reading moment, so no deadline.
	require.True(t, cards["rdc-usfed-fomc-2026-12-09-nochng"].Payoff.Deadline.IsEmpty())
	// Every threshold market got a cutoff.
	n := 0
	for id, c := range cards {
		if strings.Contains(id, "fedfnds-ondt") {
			require.False(t, c.Payoff.CutoffOp.IsEmpty(), id)
			n++
		}
	}
	require.Equal(t, 9, n)
}

// Every raw response is saved before parsing, and raw_ref points to it.
func TestPolymarket_Req18_RawSavedBeforeParseWithRawRef(t *testing.T) {
	ctx := context.Background()
	st, cyc, _ := fetch(t, "fed", vocab(t))
	raws, err := st.Raw.List(ctx, store.RawMarkets)
	require.NoError(t, err)
	require.Len(t, raws, 3, "three markets pages")
	want, err := os.ReadFile(filepath.Join(testkit.Fixture(t, "polymarket", "fed"), "markets_100.json"))
	require.NoError(t, err)
	require.Equal(t, want, raws[1].Body, "saved byte-for-byte")
	require.Equal(t, "/v1/markets?categories=macro&active=true&closed=false&limit=100&offset=100", raws[1].Endpoint)

	c := latestCards(t, st, cyc)["rvcus-fedfnds-ondt-2027-12-31-gte3pt50"]
	require.Equal(t, strconv.FormatInt(raws[1].ID, 10), c.Record.RawRef)

	books, err := st.Raw.List(ctx, store.RawPrices)
	require.NoError(t, err)
	require.Len(t, books, 22)

	// A body that does not parse is still saved, then the run fails.
	st2, _, run := fetch(t, "malformed_json", vocab(t))
	require.Equal(t, store.RunFailed, run.Status)
	require.Contains(t, run.Error, "does not parse")
	saved, err := st2.Raw.List(ctx, store.RawMarkets)
	require.NoError(t, err)
	require.Len(t, saved, 1)
}

// Rebuild calls NormalizeMarkets on saved raws: same input, same cards.
func TestPolymarket_Req18_NormalizeMarketsIsDeterministic(t *testing.T) {
	ctx := context.Background()
	v := vocab(t)
	st, cyc, _ := fetch(t, "fed", v)
	raws, err := st.Raw.List(ctx, store.RawMarkets)
	require.NoError(t, err)
	stored := latestCards(t, st, cyc)
	n := 0
	for _, raw := range raws {
		a, pa, sa := NormalizeMarkets(raw, v)
		b, pb, sb := NormalizeMarkets(raw, v)
		require.Equal(t, a, b)
		require.Equal(t, pa, pb)
		require.Equal(t, sa, sb)
		require.Empty(t, pa, "the markets payload carries no executable ask")
		for _, c := range a {
			require.Equal(t, stored[c.MarketID], c, "rebuilt card equals the stored card")
			n++
		}
	}
	require.Equal(t, 22, n)
}

// Test 20 part: a malformed record is skipped, counted and logged, and the run
// completes with the good records saved.
func TestPolymarket_Req20_MalformedRecordSkippedCountedLogged(t *testing.T) {
	ctx := context.Background()
	st, cyc, run := fetch(t, "malformed", vocab(t))
	require.Equal(t, store.RunOK, run.Status, run.Error)

	cards := latestCards(t, st, cyc)
	require.Contains(t, cards, "rvcus-fedfnds-ondt-2027-12-31-gte3pt50")
	require.Contains(t, cards, "rvcus-fedfnds-oov-2027-12-31-gte3pt75")
	require.Len(t, cards, 2)

	skips, err := st.Skips.ForCycle(ctx, cyc)
	require.NoError(t, err)
	var broken, scope bool
	for _, k := range skips {
		require.Equal(t, Venue, k.Venue)
		require.Equal(t, run.ID, k.RunID)
		if k.MarketID == "rvcus-fedfnds-broken-2027-12-31-gte4pt00" && strings.Contains(k.Reason, "malformed market record") {
			broken = true
		}
		if strings.Contains(k.Reason, "1 markets outside V1 scope") {
			scope = true
		}
	}
	require.True(t, broken, "broken record logged with its slug: %+v", skips)
	require.True(t, scope, "non-Fed market counted: %+v", skips)
	require.Equal(t, 2, run.Skipped, "broken record plus one out-of-scope market")
	require.Equal(t, 2+4, run.Saved, "two cards plus two price records each")
}

// Test 17 part: a value outside vocab.yaml is stored as empty.
func TestPolymarket_Req17_OutOfVocabStoredEmpty(t *testing.T) {
	st, cyc, _ := fetch(t, "malformed", vocab(t))
	c := latestCards(t, st, cyc)["rvcus-fedfnds-oov-2027-12-31-gte3pt75"]
	// Settled by the Federal Reserve Bank of New York: not a vocab decider.
	require.True(t, c.Payoff.Decider.IsEmpty())
	require.Equal(t, "fed_funds_upper_bound", fieldMap(c)["subject"])

	// A vocabulary without the subject or unit stores them as empty.
	v, err := card.ParseVocab([]byte("subjects: [cpi_yoy]\ndeciders: [federal_reserve]\ncutoff_units: [usd]\n"))
	require.NoError(t, err)
	st2, cyc2, _ := fetch(t, "malformed", v)
	g := latestCards(t, st2, cyc2)["rvcus-fedfnds-ondt-2027-12-31-gte3pt50"]
	require.True(t, g.Payoff.Subject.IsEmpty())
	require.True(t, g.Payoff.CutoffUnit.IsEmpty())
	require.Equal(t, "federal_reserve", fieldMap(g)["decider"])
}

// Test 19 part: HTTP 500 is saved as a failed run and not returned as an error.
func TestPolymarket_Req19_HTTP500IsFailedRunNotError(t *testing.T) {
	ctx := context.Background()
	st, cyc, run := fetch(t, "http_500", vocab(t))
	require.Equal(t, store.RunFailed, run.Status)
	require.Contains(t, run.Error, "HTTP 500")
	require.Equal(t, Venue, run.Venue)
	require.Empty(t, latestCards(t, st, cyc))
	runs, err := st.Runs.ForCycle(ctx, cyc)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, store.RunFailed, runs[0].Status)
}

// Prices convert exactly to micro-dollars; 1 - bid is integer math; malformed
// or out-of-range prices make that side empty and are logged.
func TestPolymarket_IntegerPriceConversionExact(t *testing.T) {
	raw := func(body string) store.Raw {
		return store.Raw{ID: 7, CycleID: 1, RunID: 2, Venue: Venue, Kind: store.RawPrices, FetchedAt: fetchedAt, Body: []byte(body)}
	}
	book := `{"marketData":{"marketSlug":"m","state":"MARKET_STATE_OPEN",
	  "bids":[{"px":{"value":"0.1234","currency":"USD"},"qty":"5"},{"px":{"value":"0.3301","currency":"USD"},"qty":"1.5"},{"px":{"value":"0.9","currency":"USD"},"qty":"0.0000"}],
	  "offers":[{"px":{"value":"0.6700","currency":"USD"},"qty":"2"},{"px":{"value":"0.335","currency":"USD"},"qty":"3"}]}}`
	ps, skips := NormalizePrices(raw(book))
	require.Empty(t, skips)
	require.Len(t, ps, 2)
	require.Equal(t, card.Yes, ps[0].Side)
	require.Equal(t, card.Micros(335_000), mustGet(t, ps[0].BuyPrice))
	require.Equal(t, card.No, ps[1].Side)
	require.Equal(t, card.Micros(669_900), mustGet(t, ps[1].BuyPrice), "1 - 0.3301; the zero-quantity 0.9 bid is ignored")
	require.Equal(t, "m", ps[0].MarketID)
	require.Equal(t, fetchedAt, ps[0].PricedAt)

	bad := `{"marketData":{"marketSlug":"m","state":"MARKET_STATE_OPEN",
	  "bids":[{"px":{"value":"1.5","currency":"USD"},"qty":"1"}],
	  "offers":[{"px":{"value":"0.1234567","currency":"USD"},"qty":"1"}]}}`
	ps, skips = NormalizePrices(raw(bad))
	require.Len(t, ps, 2)
	require.True(t, ps[0].BuyPrice.IsEmpty())
	require.True(t, ps[1].BuyPrice.IsEmpty())
	require.Len(t, skips, 2)

	halted := `{"marketData":{"marketSlug":"m","state":"MARKET_STATE_HALTED","bids":[{"px":{"value":"0.4","currency":"USD"},"qty":"1"}],"offers":[{"px":{"value":"0.6","currency":"USD"},"qty":"1"}]}}`
	ps, _ = NormalizePrices(raw(halted))
	require.True(t, ps[0].BuyPrice.IsEmpty() && ps[1].BuyPrice.IsEmpty())

	_, skips = NormalizePrices(raw(`{"marketData":`))
	require.Len(t, skips, 1)
}

func mustGet(t *testing.T, o card.Opt[card.Micros]) card.Micros {
	t.Helper()
	v, ok := o.Get()
	require.True(t, ok)
	return v
}

// The price loop refreshes only the requested markets.
func TestPolymarket_FetchPricesRefreshesOnlyGivenMarkets(t *testing.T) {
	ctx := context.Background()
	st := testkit.TempStore(t)
	cyc, err := st.Cycles.Begin(ctx, store.KindPrices, store.TriggerCLI)
	require.NoError(t, err)
	f := newFetcher(t, "fed", vocab(t))
	run, err := f.FetchPrices(ctx, st, cyc, []string{"rvcus-fedfnds-ondt-2027-12-31-gte3pt50", "no-such-market"})
	require.NoError(t, err)
	require.Equal(t, store.RunOK, run.Status)
	require.Equal(t, 2, run.Saved)
	require.Equal(t, 1, run.Skipped, "unknown market skipped with a reason")
	require.Len(t, latestPrices(t, st, cyc), 2)
}

// Rules text parsing for operators and the ET deadline across daylight saving.
func TestPolymarket_CutoffAndDeadlineParsing(t *testing.T) {
	v := vocab(t)
	cases := map[string]string{
		"is greater than 4.25%":          "gt 4.25",
		"is greater than or equal to 4%": "gte 4",
		"is at or above 3.75%":           "gte 3.75",
		"is less than 3.50%":             "lt 3.5",
		"is at or below 2%":              "lte 2",
	}
	for phrase, want := range cases {
		op, val, unit := cutoffOf("the upper bound of the target range for the federal funds rate "+phrase+" on June 30, 2027 at 11:59 PM ET.", v)
		o, _ := op.Get()
		d, _ := val.Get()
		u, _ := unit.Get()
		require.Equal(t, want, string(o)+" "+d.String(), phrase)
		require.Equal(t, "percent", u)
	}
	d, ok := deadlineOf("on June 30, 2027 at 11:59 PM ET.").Get()
	require.True(t, ok)
	require.Equal(t, "2027-07-01T03:59:00Z", d.String(), "EDT is UTC-4")
	require.True(t, deadlineOf("on June 30, 2027 at 11:59 PM ET or July 1, 2027 at 1:00 PM ET").IsEmpty(), "two moments are ambiguous")
	require.True(t, unclearResultOf("If the game is canceled, this market will resolve 50-50.").IsEmpty())
	require.True(t, unclearResultOf("This market will settle to Yes if the upper bound ... Outcome sourced from the United States Federal Reserve.").IsEmpty(), "D-20: silent rules stay empty")
	require.Equal(t, card.Some(card.YesOrNoOnly).Equal(unclearResultOf("If the outcome cannot be determined by the deadline, this market resolves to No.")), true, "D-20: an explicit fallback sentence in the rules counts")
}

// A 429 is retried with backoff (1s, 2s, 4s), then the run fails without an error return.
func TestPolymarket_RateLimitBacksOffThenFailsRun(t *testing.T) {
	ctx := context.Background()
	st := testkit.TempStore(t)
	cyc, err := st.Cycles.Begin(ctx, store.KindDiscovery, store.TriggerCLI)
	require.NoError(t, err)
	tr := testkit.Scenario(t, testkit.Fixture(t, "polymarket", "rate_limited"))
	f := New(card.FetcherConfig{}, vocab(t), tr.Client(), testkit.FixedClock(fetchedAt))
	f.gap = 0
	var waits []time.Duration
	f.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	run, err := f.Fetch(ctx, st, cyc)
	require.NoError(t, err)
	require.Equal(t, store.RunFailed, run.Status)
	require.Contains(t, run.Error, "HTTP 429")
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, waits)
	require.Len(t, tr.Requests(), 4)
}
