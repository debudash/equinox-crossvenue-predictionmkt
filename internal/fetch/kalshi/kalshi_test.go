package kalshi_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/fetch/kalshi"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

func vocab(t *testing.T) *card.Vocab {
	t.Helper()
	v, err := card.LoadVocab(filepath.Join(testkit.RepoRoot(t), "config", "vocab.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// fetch runs one Fetch of a fixture scenario in a fresh store and cycle.
func fetch(t *testing.T, scenario string) (*store.Store, int64, store.FetchRun, *testkit.Transport) {
	t.Helper()
	ctx := context.Background()
	tr := testkit.Scenario(t, testkit.Fixture(t, "kalshi", scenario))
	st := testkit.TempStore(t)
	cycleID, err := st.Cycles.Begin(ctx, store.KindFetch, store.TriggerCLI)
	if err != nil {
		t.Fatal(err)
	}
	f := kalshi.New(card.FetcherConfig{}, vocab(t), tr.Client(), testkit.FixedClock(testkit.AsOf))
	f.Timeout = 200 * time.Millisecond
	run, err := f.Fetch(ctx, st, cycleID)
	if err != nil {
		t.Fatalf("Fetch returned a store error: %v", err)
	}
	return st, cycleID, run, tr
}

func rawOf(t *testing.T, scenario, file string, id int64) store.Raw {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testkit.Fixture(t, "kalshi", scenario), file))
	if err != nil {
		t.Fatal(err)
	}
	return store.Raw{ID: id, CycleID: 1, RunID: 1, Venue: kalshi.Venue, Kind: store.RawMarkets, FetchedAt: testkit.AsOf, Body: b}
}

func findCard(t *testing.T, cards []card.Card, id string) card.Card {
	t.Helper()
	for _, c := range cards {
		if c.MarketID == id {
			return c
		}
	}
	t.Fatalf("no card %s", id)
	return card.Card{}
}

func price(t *testing.T, ps []card.PriceRecord, id string, side card.Side) card.PriceRecord {
	t.Helper()
	for _, p := range ps {
		if p.MarketID == id && p.Side == side {
			return p
		}
	}
	t.Fatalf("no %s price for %s", side, id)
	return card.PriceRecord{}
}

func wantFields(t *testing.T, c card.Card, want map[string]string) {
	t.Helper()
	for _, f := range c.Payoff.Fields() {
		w, listed := want[f.Name]
		if !listed {
			t.Fatalf("test does not state %s", f.Name)
		}
		if f.Value != w || f.Set != (w != "") {
			t.Errorf("%s %s = %q (set %v), want %q", c.MarketID, f.Name, f.Value, f.Set, w)
		}
	}
}

func wantPrice(t *testing.T, p card.PriceRecord, want string) {
	t.Helper()
	v, ok := p.BuyPrice.Get()
	got := ""
	if ok {
		got = v.String()
	}
	if got != want {
		t.Errorf("%s %s buy_price = %q, want %q", p.MarketID, p.Side, got, want)
	}
	if !p.Fee.IsEmpty() {
		t.Errorf("%s %s fee is set, want empty", p.MarketID, p.Side)
	}
	if !p.PricedAt.Equal(testkit.AsOf) {
		t.Errorf("%s priced_at = %v, want the raw response time", p.MarketID, p.PricedAt)
	}
}

// Test 16 (Kalshi side): the recorded Fed fixture normalizes to full payoff
// fields and executable ask prices.
func TestKalshi_Req16_NormalizesFedFixture(t *testing.T) {
	raw := rawOf(t, "fed", "events_kxfed.json", 7)
	cards, prices, skips := kalshi.NormalizeMarkets(raw, vocab(t))
	if len(cards) != 247 || len(prices) != 2*247 || len(skips) != 0 {
		t.Fatalf("got %d cards, %d prices, %d skips; want 247, 494, 0: %v", len(cards), len(prices), len(skips), skips)
	}
	c := findCard(t, cards, "KXFED-26DEC-T3.75")
	wantFields(t, c, map[string]string{
		"subject": "fed_funds_upper_bound", "cutoff_op": "gt", "cutoff_value": "3.75", "cutoff_unit": "percent",
		"deadline": "2026-12-09T19:00:00Z", "decider": "federal_reserve", "unclear_result": "",
	})
	if c.Venue != "kalshi" || c.Record.RawRef != "7" || !c.Record.FetchedAt.Equal(testkit.AsOf) {
		t.Errorf("record fields: venue %q raw_ref %q fetched_at %v", c.Venue, c.Record.RawRef, c.Record.FetchedAt)
	}
	if c.Record.EventTitle != "Fed funds rate after Dec 2026 meeting?" || c.Record.Category != "Economics" || c.Record.Volume24h != "5054.09" {
		t.Errorf("display fields: %+v", c.Record)
	}
	if !strings.HasPrefix(c.Record.RulesText, "If the upper bound of the target federal funds rate") ||
		!strings.Contains(c.Record.RulesText, "\n\nThis market will expire") {
		t.Errorf("rules_text is not rules_primary then rules_secondary verbatim: %q", c.Record.RulesText)
	}
	wantPrice(t, price(t, prices, "KXFED-26DEC-T3.75", card.Yes), "0.99")
	wantPrice(t, price(t, prices, "KXFED-26DEC-T3.75", card.No), "0.04")

	year := rawOf(t, "fed", "events_kxfedfundsyear.json", 8)
	cards, prices, skips = kalshi.NormalizeMarkets(year, vocab(t))
	if len(cards) != 210 || len(skips) != 0 {
		t.Fatalf("year series: %d cards, %d skips", len(cards), len(skips))
	}
	wantFields(t, findCard(t, cards, "KXFEDFUNDSYEAR-28JAN01-T4.00"), map[string]string{
		"subject": "fed_funds_upper_bound", "cutoff_op": "gt", "cutoff_value": "4", "cutoff_unit": "percent",
		"deadline": "2028-01-01T04:59:00Z", "decider": "federal_reserve", "unclear_result": "",
	})
	wantPrice(t, price(t, prices, "KXFEDFUNDSYEAR-28JAN01-T4.00", card.Yes), "0.81")
	wantPrice(t, price(t, prices, "KXFEDFUNDSYEAR-28JAN01-T4.00", card.No), "0.2")
}

// An empty book side is stored as empty, never as 0 or 1.
func TestKalshi_EmptyAskSideStoredEmpty(t *testing.T) {
	_, prices, _ := kalshi.NormalizeMarkets(rawOf(t, "fed", "events_kxfed.json", 1), vocab(t))
	// KXFED-26OCT-T5.25 has no yes bids, so Kalshi reports its no ask as 1.0000.
	wantPrice(t, price(t, prices, "KXFED-26OCT-T5.25", card.No), "")
	wantPrice(t, price(t, prices, "KXFED-26OCT-T5.25", card.Yes), "0.01")
}

func TestKalshi_NormalizeMarketsIsDeterministic(t *testing.T) {
	raw := rawOf(t, "fed", "events_kxfed.json", 3)
	c1, p1, s1 := kalshi.NormalizeMarkets(raw, vocab(t))
	c2, p2, s2 := kalshi.NormalizeMarkets(raw, vocab(t))
	if !reflect.DeepEqual(c1, c2) || !reflect.DeepEqual(p1, p2) || !reflect.DeepEqual(s1, s2) {
		t.Fatal("two calls on the same raw response differ")
	}
}

// Every raw response is saved before parsing, byte for byte, and each card's
// raw_ref names the response it came from.
func TestKalshi_Req18_RawSavedBeforeParseAndRawRefSet(t *testing.T) {
	ctx := context.Background()
	st, cycleID, run, _ := fetch(t, "fed")
	if run.Status != store.RunOK || run.Saved != 457 || run.Skipped != 0 || run.Error != "" {
		t.Fatalf("run = %+v", run)
	}
	raws, err := st.Raw.List(ctx, store.RawMarkets)
	if err != nil {
		t.Fatal(err)
	}
	if len(raws) != 2 {
		t.Fatalf("%d raw responses saved, want 2", len(raws))
	}
	byID := map[string]store.Raw{}
	for i, file := range []string{"events_kxfed.json", "events_kxfedfundsyear.json"} {
		want := rawOf(t, "fed", file, 0).Body
		if string(raws[i].Body) != string(want) {
			t.Errorf("raw %d is not byte-equal to %s", i, file)
		}
		if raws[i].RunID != run.ID || raws[i].Kind != store.RawMarkets || !raws[i].FetchedAt.Equal(testkit.AsOf) {
			t.Errorf("raw %d = %+v", i, raws[i])
		}
		byID[strconv.FormatInt(raws[i].ID, 10)] = raws[i]
	}
	cards, err := st.Cards.LatestAll(ctx, store.Writing(cycleID))
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 457 {
		t.Fatalf("%d cards saved", len(cards))
	}
	for _, c := range cards {
		raw, ok := byID[c.Record.RawRef]
		if !ok || !strings.Contains(string(raw.Body), `"ticker":"`+c.MarketID+`"`) {
			t.Fatalf("card %s raw_ref %q does not point at its response", c.MarketID, c.Record.RawRef)
		}
	}
	p, ok, err := st.Prices.LatestAt(ctx, card.MarketRef{Venue: "kalshi", MarketID: "KXFED-26DEC-T4.00"}, card.Yes, testkit.AsOf, store.Writing(cycleID))
	if err != nil || !ok {
		t.Fatalf("price not saved: %v", err)
	}
	wantPrice(t, p, "0.86")
}

// A body that does not parse is still saved first, and the run is recorded as
// failed without an error return.
func TestKalshi_RawSavedEvenWhenBodyDoesNotParse(t *testing.T) {
	st, _, run, _ := fetch(t, "malformed_json")
	if run.Status != store.RunFailed || !strings.Contains(run.Error, "does not parse") {
		t.Fatalf("run = %+v", run)
	}
	raws, err := st.Raw.List(context.Background(), store.RawMarkets)
	if err != nil {
		t.Fatal(err)
	}
	if len(raws) != 2 || string(raws[0].Body) != "{" {
		t.Fatalf("want the unparsable body saved first, got %d raws", len(raws))
	}
}

func skipsOf(t *testing.T, st *store.Store, cycleID int64) map[string]string {
	t.Helper()
	ks, err := st.Skips.ForCycle(context.Background(), cycleID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, k := range ks {
		if k.Venue != "kalshi" || k.Stage != store.StageFetch || k.RunID == 0 {
			t.Errorf("skip %+v", k)
		}
		out[k.MarketID] = k.Reason
	}
	return out
}

// Test 20 (fetch side): a malformed record is skipped, counted and logged,
// and the run completes with the good records saved.
func TestKalshi_Req20_MalformedRecordSkippedCountedLoggedRunCompletes(t *testing.T) {
	st, cycleID, run, _ := fetch(t, "malformed")
	if run.Status != store.RunOK || run.Saved != 3 || run.Skipped != 2 {
		t.Fatalf("run = %+v; want ok, 3 saved, 2 skipped", run)
	}
	reason := skipsOf(t, st, cycleID)["KXFED-26DEC-T3.50"]
	if !strings.Contains(reason, "yes_ask_dollars") || !strings.Contains(reason, "outside 0 to 1") {
		t.Errorf("skip reason = %q", reason)
	}
	if _, found, _ := st.Cards.Latest(context.Background(), card.MarketRef{Venue: "kalshi", MarketID: "KXFED-26DEC-T3.50"}, store.Writing(cycleID)); found {
		t.Error("the malformed record was saved as a card")
	}
}

func TestKalshi_ComboMarketSkippedAndCounted(t *testing.T) {
	st, cycleID, run, _ := fetch(t, "malformed")
	found := false
	for id, reason := range skipsOf(t, st, cycleID) {
		if strings.HasPrefix(id, "KXMVE") && strings.Contains(reason, "multivariate combo") {
			found = true
		}
	}
	if !found || run.Skipped != 2 {
		t.Fatalf("combo market not skipped and counted (skipped %d)", run.Skipped)
	}
}

// Test 17 (fetch side): a venue value outside vocab.yaml is stored as empty.
func TestKalshi_Req17_OutOfVocabStoredEmpty(t *testing.T) {
	st, cycleID, _, _ := fetch(t, "malformed")
	c, found, err := st.Cards.Latest(context.Background(), card.MarketRef{Venue: "kalshi", MarketID: "KXFED-27JAN-T3.75"}, store.Writing(cycleID))
	if err != nil || !found {
		t.Fatalf("card not saved: %v", err)
	}
	// The settlement source "Bank of England" maps to no decider.
	wantFields(t, c, map[string]string{
		"subject": "fed_funds_upper_bound", "cutoff_op": "gt", "cutoff_value": "3.75", "cutoff_unit": "percent",
		"deadline": "2027-01-27T19:00:00Z", "decider": "", "unclear_result": "",
	})

	// A vocabulary without the subject and decider leaves both empty.
	small, err := card.ParseVocab([]byte("subjects: [cpi]\ndeciders: [bls]\ncutoff_units: [percent]\n"))
	if err != nil {
		t.Fatal(err)
	}
	cards, _, _ := kalshi.NormalizeMarkets(rawOf(t, "fed", "events_kxfed.json", 1), small)
	got := findCard(t, cards, "KXFED-26DEC-T3.75")
	if !got.Payoff.Subject.IsEmpty() || !got.Payoff.Decider.IsEmpty() {
		t.Errorf("out of vocabulary values were stored: %+v", got.Payoff.Fields())
	}
}

func TestKalshi_HTTP500RecordedAsFailedRun(t *testing.T) {
	_, _, run, tr := fetch(t, "http_500")
	if run.Status != store.RunFailed || !strings.Contains(run.Error, "HTTP 500") {
		t.Fatalf("run = %+v", run)
	}
	// The other series is still fetched.
	if n := len(tr.Requests()); n != 2 {
		t.Errorf("%d requests, want 2", n)
	}
}

func TestKalshi_TimeoutRecordedAsFailedRun(t *testing.T) {
	_, _, run, _ := fetch(t, "timeout")
	if run.Status != store.RunFailed || !strings.Contains(run.Error, "timed out") {
		t.Fatalf("run = %+v", run)
	}
}

func TestKalshi_FetchPricesRefreshesOnlyNamedMarkets(t *testing.T) {
	ctx := context.Background()
	tr := testkit.Scenario(t, testkit.Fixture(t, "kalshi", "fed"))
	st := testkit.TempStore(t)
	cycleID, err := st.Cycles.Begin(ctx, store.KindPrices, store.TriggerWatch)
	if err != nil {
		t.Fatal(err)
	}
	f := kalshi.New(card.FetcherConfig{}, vocab(t), tr.Client(), testkit.FixedClock(testkit.AsOf))
	run, err := f.FetchPrices(ctx, st, cycleID, []string{"KXFEDFUNDSYEAR-28JAN01-T4.00", "KXFED-26DEC-T3.75", "KXFED-26DEC-T3.75"})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != store.RunOK || run.Saved != 4 || run.Skipped != 0 {
		t.Fatalf("run = %+v", run)
	}
	raws, err := st.Raw.List(ctx, store.RawPrices)
	if err != nil || len(raws) != 1 {
		t.Fatalf("want one prices raw response, got %d (%v)", len(raws), err)
	}
	ps, _ := kalshi.NormalizePrices(raws[0])
	wantPrice(t, price(t, ps, "KXFED-26DEC-T3.75", card.No), "0.04")
	wantPrice(t, price(t, ps, "KXFEDFUNDSYEAR-28JAN01-T4.00", card.Yes), "0.81")
}
