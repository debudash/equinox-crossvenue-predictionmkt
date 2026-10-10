package acceptance

import (
	"reflect"
	"regexp"
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

const (
	kv = kalshi.Venue
	pv = polymarket.Venue
)

// Test 16: the Kalshi and Polymarket fixtures for the same Fed market
// normalize to byte-equal payoff fields. Per D-19 no real pair exists, so the
// fixtures are the clearly labelled SYNTHETIC pair; the real refusal is
// TestRealFixtures_ClosestPairIsNotSameNamingCutoffOp.
func TestReq16_SyntheticFedPairNormalizesToByteEqualPayoff(t *testing.T) {
	e := NewEnv(t)
	cyc := e.Begin()
	e.FetchSynthetic(cyc, FetchedAt)

	k := e.Card(cyc, kv, KalshiT375)
	p := e.Card(cyc, pv, PolyGT375)
	if !reflect.DeepEqual(k.Payoff.Fields(), p.Payoff.Fields()) {
		t.Fatalf("payoff fields differ:\nkalshi:\n%spolymarket:\n%s", FieldsText(k), FieldsText(p))
	}
	want := "subject=fed_funds_upper_bound\ncutoff_op=gt\ncutoff_value=3.75\ncutoff_unit=percent\n" +
		"deadline=2026-12-09T19:00:00Z\ndecider=federal_reserve\nunclear_result=yes_or_no_only\n"
	if got := FieldsText(k); got != want {
		t.Fatalf("kalshi payoff:\n%s\nwant:\n%s", got, want)
	}
	if got := FieldsText(p); got != want {
		t.Fatalf("polymarket payoff:\n%s\nwant:\n%s", got, want)
	}
	// Titles differ; only payoff fields decide the bet.
	if k.Record.Title == p.Record.Title {
		t.Errorf("titles are equal (%q); the fixture should show differing titles", k.Record.Title)
	}
	if r := match.Compare(k, p); r.Result != card.Same {
		t.Fatalf("Compare = %+v, want same", r)
	}
}

// D-19: the real recordings of 2026-10-10 hold no exact pair. The closest real
// pair (Kalshi "above 3.25%" = gt, Polymarket US "greater than or equal to
// 3.25%" = gte, both read at year end 2027) is refused, naming cutoff_op, and
// the real data produces no same or opposite pair at all.
func TestRealFixtures_ClosestPairIsNotSameNamingCutoffOp(t *testing.T) {
	e := NewEnv(t)
	cyc := e.Begin()
	if run := e.FetchKalshi(cyc, Fixture(t, "kalshi", "fed"), FetchedAt); run.Status != store.RunOK {
		t.Fatalf("kalshi run = %+v", run)
	}
	if run := e.FetchPolymarket(cyc, Fixture(t, "polymarket", "fed"), FetchedAt); run.Status != store.RunOK {
		t.Fatalf("polymarket run = %+v", run)
	}
	k := e.Card(cyc, kv, "KXFEDFUNDSYEAR-28JAN01-T3.25")
	p := e.Card(cyc, pv, "rvcus-fedfnds-ondt-2027-12-31-gte3pt25")
	for _, f := range []struct{ name, k, p string }{
		{card.FieldSubject, "fed_funds_upper_bound", "fed_funds_upper_bound"},
		{card.FieldCutoffOp, "gt", "gte"},
		{card.FieldCutoffValue, "3.25", "3.25"},
		{card.FieldDeadline, "2028-01-01T04:59:00Z", "2028-01-01T04:59:00Z"},
	} {
		if got := field(k, f.name); got != f.k {
			t.Errorf("kalshi %s = %q, want %q", f.name, got, f.k)
		}
		if got := field(p, f.name); got != f.p {
			t.Errorf("polymarket %s = %q, want %q", f.name, got, f.p)
		}
	}

	sum := e.Match(cyc)
	if sum.Same != 0 || sum.Opposite != 0 || sum.Compared == 0 {
		t.Fatalf("real fixtures: %+v; want pairs compared and none same or opposite", sum)
	}
	rec := e.Pair(cyc, card.MatchRecord, k.MarketID, p.MarketID, "")
	if rec.Match != card.NotSame || !strings.Contains(rec.Reason, "cutoff_op differs") {
		t.Fatalf("closest real pair: %s %q; want not_same naming cutoff_op", rec.Match, rec.Reason)
	}
	if s := e.Route(cyc, card.Yes, testkit.AsOf); s.Routed != 0 || s.NoRoute != 0 {
		t.Fatalf("route over real fixtures = %+v; want nothing routed", s)
	}
}

// Test 17 end to end: a settlement source outside vocab.yaml (Kalshi
// KXFED-27JAN-T3.75 names the Bank of England) is stored as an empty decider,
// and the card matches nothing, even against a market equal in every other
// payoff field.
func TestReq17_KalshiOutOfVocabDeciderStoredEmptyMatchesNothing(t *testing.T) {
	e := NewEnv(t)
	cyc := e.Begin()
	e.FetchKalshi(cyc, Fixture(t, "kalshi", "malformed"), FetchedAt)
	e.FetchPolymarket(cyc, Fixture(t, "faults", "polymarket", "counterpart_27jan"), FetchedAt)

	k := e.Card(cyc, kv, "KXFED-27JAN-T3.75")
	if !k.Payoff.Decider.IsEmpty() {
		t.Fatalf("out of vocabulary decider was stored: %s", FieldsText(k))
	}
	p := e.Card(cyc, pv, "synthetic-fedfnds-2027-01-27-gt3pt75")

	// Control: with a known decider the two cards would be the same bet, so
	// the empty decider is the only thing standing between them.
	fixed := k
	fixed.Payoff.Decider = card.Some("federal_reserve")
	if r := match.Compare(fixed, p); r.Result != card.Same {
		t.Fatalf("control pair = %+v; the counterpart fixture should differ only in decider", r)
	}

	e.Match(cyc)
	recs := recordsFor(e.Log(cyc, card.MatchRecord), k.Ref())
	if len(recs) == 0 {
		t.Fatal("the out of vocabulary card was never compared")
	}
	for _, r := range recs {
		if r.Match != card.NotSame {
			t.Errorf("%s vs %s = %s, want not_same", r.MarketA, r.MarketB, r.Match)
		}
	}
	rec := e.Pair(cyc, card.MatchRecord, k.MarketID, p.MarketID, "")
	if want := "decider is empty on kalshi:KXFED-27JAN-T3.75"; !strings.Contains(rec.Reason, want) {
		t.Fatalf("reason %q does not contain %q", rec.Reason, want)
	}
	if s := e.Route(cyc, card.Yes, testkit.AsOf); s.Routed != 0 {
		t.Fatalf("route = %+v; nothing may be routed", s)
	}
}

// Test 17 end to end, Polymarket side: "Outcome sourced from the Federal
// Reserve Bank of New York" is not in vocab.yaml, so decider is empty and
// neither synthetic market matches Kalshi KXFED-26DEC-T3.75.
func TestReq17_PolymarketOutOfVocabDeciderStoredEmptyMatchesNothing(t *testing.T) {
	e := NewEnv(t)
	cyc := e.Begin()
	e.FetchKalshi(cyc, Fixture(t, "kalshi", "synthetic"), FetchedAt)
	e.FetchPolymarket(cyc, Fixture(t, "faults", "polymarket", "oov_decider"), FetchedAt)

	for _, id := range []string{PolyGT375, PolyLTE375} {
		c := e.Card(cyc, pv, id)
		if !c.Payoff.Decider.IsEmpty() {
			t.Fatalf("%s: out of vocabulary decider was stored", id)
		}
	}
	sum := e.Match(cyc)
	if sum.Same != 0 || sum.Opposite != 0 {
		t.Fatalf("match = %+v; want no same or opposite", sum)
	}
	for _, id := range []string{PolyGT375, PolyLTE375} {
		rec := e.Pair(cyc, card.MatchRecord, KalshiT375, id, "")
		if want := "decider is empty on polymarket:" + id; !strings.Contains(rec.Reason, want) {
			t.Errorf("reason %q does not contain %q", rec.Reason, want)
		}
	}
}

// Test 19: one venue failing to load does not drop the other venue's saved
// cards, and pairs that needed the failed venue are no_route.
//
// Cycle 1: both venues up; the synthetic pair matches and routes. Cycle 2:
// Polymarket answers HTTP 500. Kalshi saves fresh cards and prices, the
// Polymarket cards saved in cycle 1 remain, and every pair needing Polymarket
// is no_route because its only Polymarket price is the old one. Nothing
// crashes and the cycle completes.
func TestReq19_PolymarketDownKeepsKalshiAndDependentPairsAreNoRoute(t *testing.T) {
	e := NewEnv(t)
	first := testkit.AsOf.Add(-30 * time.Minute)
	c1 := e.Begin()
	e.FetchSynthetic(c1, first)
	e.Match(c1)
	if s := e.Route(c1, card.Yes, first.Add(time.Minute)); s.Routed != 2 {
		t.Fatalf("cycle 1 with both venues up: %+v, want 2 routed", s)
	}
	e.Complete(c1)

	c2 := e.Begin()
	kr := e.FetchKalshi(c2, Fixture(t, "kalshi", "synthetic"), FetchedAt)
	pr := e.FetchPolymarket(c2, Fixture(t, "polymarket", "http_500"), FetchedAt)
	if kr.Status != store.RunOK || pr.Status != store.RunFailed || !strings.Contains(pr.Error, "HTTP 500") {
		t.Fatalf("runs: kalshi %+v, polymarket %+v", kr, pr)
	}
	for _, id := range []string{PolyGT375, PolyLTE375} {
		e.Card(c2, pv, id) // the failed venue's earlier cards are kept
	}
	e.Card(c2, kv, KalshiT375)

	if sum := e.Match(c2); sum.Same != 1 || sum.Opposite != 1 {
		t.Fatalf("cycle 2 match = %+v", sum)
	}
	for _, side := range []card.Side{card.Yes, card.No} {
		s := e.Route(c2, side, testkit.AsOf)
		if s.Routed != 0 || s.NoRoute != 2 {
			t.Fatalf("cycle 2 route %s = %+v; want 2 no_route", side, s)
		}
		for _, id := range []string{PolyGT375, PolyLTE375} {
			r := e.Pair(c2, card.RouteRecord, KalshiT375, id, side)
			if r.Action != card.NoRoute || !strings.Contains(r.Reason, "The polymarket") || !strings.Contains(r.Reason, "stale") {
				t.Errorf("%s %s: %s %q; want no_route naming the stale polymarket price", id, side, r.Action, r.Reason)
			}
			if strings.Contains(r.Reason, "The kalshi") {
				t.Errorf("%s %s: reason blames the kalshi price, which is fresh: %q", id, side, r.Reason)
			}
		}
	}
	e.Complete(c2)
}

// Test 19, other direction and first run: Kalshi answers HTTP 500 on a store
// that never had Kalshi data. The Polymarket cards and prices are saved; with
// no Kalshi card no pair needing Kalshi exists, so nothing is routed.
func TestReq19_KalshiDownFromStartKeepsPolymarketAndRoutesNothing(t *testing.T) {
	e := NewEnv(t)
	cyc := e.Begin()
	kr := e.FetchKalshi(cyc, Fixture(t, "kalshi", "http_500"), FetchedAt)
	pr := e.FetchPolymarket(cyc, Fixture(t, "polymarket", "synthetic"), FetchedAt)
	if kr.Status != store.RunFailed || !strings.Contains(kr.Error, "HTTP 500") || pr.Status != store.RunOK {
		t.Fatalf("runs: kalshi %+v, polymarket %+v", kr, pr)
	}
	if n := len(e.Cards(cyc)); n != 2 {
		t.Fatalf("%d cards saved, want the 2 Polymarket cards", n)
	}
	if sum := e.Match(cyc); sum.Compared != 0 {
		t.Fatalf("match = %+v; no pair can exist without a Kalshi card", sum)
	}
	if s := e.Route(cyc, card.Yes, testkit.AsOf); s.Routed+s.NoRoute != 0 {
		t.Fatalf("route = %+v", s)
	}
	e.Complete(cyc)
}

// Test 20 end to end: malformed records on both venues are skipped, counted on
// the fetch run, logged in the skips table with a reason, and the fetch,
// match and route run completes.
func TestReq20_MalformedRecordsSkippedCountedLoggedRunCompletes(t *testing.T) {
	e := NewEnv(t)
	cyc := e.Begin()
	kr := e.FetchKalshi(cyc, Fixture(t, "kalshi", "malformed"), FetchedAt)
	pr := e.FetchPolymarket(cyc, Fixture(t, "polymarket", "malformed"), FetchedAt)
	if kr.Status != store.RunOK || kr.Skipped != 2 || kr.Saved != 3 {
		t.Errorf("kalshi run = %+v; want ok, 3 saved, 2 skipped", kr)
	}
	if pr.Status != store.RunOK || pr.Skipped < 2 {
		t.Errorf("polymarket run = %+v; want ok with the broken and out of scope records skipped", pr)
	}

	skips := e.Skips(cyc)
	if got := len(skips); got != kr.Skipped+pr.Skipped {
		// Polymarket counts out-of-scope records one by one but logs one
		// summary row per page; this fixture has exactly one such record.
		t.Errorf("%d skip rows, runs count %d", got, kr.Skipped+pr.Skipped)
	}
	wantReason := map[string]*regexp.Regexp{
		"KXFED-26DEC-T3.50":                        regexp.MustCompile(`yes_ask_dollars.*outside 0 to 1`),
		"rvcus-fedfnds-broken-2027-12-31-gte4pt00": regexp.MustCompile(`malformed market record`),
	}
	for id, re := range wantReason {
		found := false
		for _, k := range skips {
			if k.MarketID == id && k.Stage == store.StageFetch && re.MatchString(k.Reason) {
				found = true
			}
		}
		if !found {
			t.Errorf("no skip for %s matching %q in %+v", id, re, skips)
		}
		if _, ok := e.FindCard(cyc, kv, id); ok {
			t.Errorf("malformed %s was saved as a card", id)
		}
		if _, ok := e.FindCard(cyc, pv, id); ok {
			t.Errorf("malformed %s was saved as a card", id)
		}
	}
	for _, k := range skips {
		if k.Reason == "" {
			t.Errorf("skip without a reason: %+v", k)
		}
	}

	// The good records were saved and the rest of the run completes.
	e.Card(cyc, kv, "KXFED-26DEC-T3.75")
	e.Card(cyc, pv, "rvcus-fedfnds-ondt-2027-12-31-gte3pt50")
	e.Match(cyc)
	e.Route(cyc, card.Yes, testkit.AsOf)
	e.Complete(cyc)
	if c, err := e.St.Cycles.Get(e.Ctx, cyc); err != nil || c.Status != store.StatusCompleted {
		t.Fatalf("cycle = %+v, %v", c, err)
	}
}

// Happy path over the synthetic pair: fetch both venues, match, and route yes
// and no at as_of. One same and one opposite pair; each routed to a venue with
// a plain-sentence reason.
func TestE2E_SyntheticHappyPath(t *testing.T) {
	e := NewEnv(t)
	cyc := e.Begin()
	e.FetchSynthetic(cyc, FetchedAt)

	sum := e.Match(cyc)
	if sum.Compared != 4 || sum.Same != 1 || sum.Opposite != 1 || sum.NotSame != 2 {
		t.Fatalf("match = %+v; want 4 compared, 1 same, 1 opposite, 2 not_same", sum)
	}
	if r := e.Pair(cyc, card.MatchRecord, KalshiT375, PolyGT375, ""); r.Match != card.Same {
		t.Fatalf("T3.75 vs gt3.75 = %s %q", r.Match, r.Reason)
	}
	if r := e.Pair(cyc, card.MatchRecord, KalshiT375, PolyLTE375, ""); r.Match != card.Opposite {
		t.Fatalf("T3.75 vs lte3.75 = %s %q", r.Match, r.Reason)
	}
	if r := e.Pair(cyc, card.MatchRecord, KalshiT400, PolyGT375, ""); r.Match != card.NotSame || !strings.Contains(r.Reason, "cutoff_value") {
		t.Fatalf("T4.00 vs gt3.75 = %s %q", r.Match, r.Reason)
	}

	for _, side := range []card.Side{card.Yes, card.No} {
		if s := e.Route(cyc, side, testkit.AsOf); s.Routed != 2 || s.NoRoute != 0 {
			t.Fatalf("route %s = %+v; want 2 routed", side, s)
		}
	}
	// Kalshi T3.75: yes 0.96, no 0.05. Polymarket gt3.75: yes 0.95, no 0.07.
	// Polymarket lte3.75: yes 0.06, no 0.97.
	cases := []struct {
		poly         string
		side, sideB  card.Side
		action       card.Action
		priceA, priB string
	}{
		{PolyGT375, card.Yes, card.Yes, card.BuyOn(pv), "0.96", "0.95"},
		{PolyGT375, card.No, card.No, card.BuyOn(kv), "0.05", "0.07"},
		{PolyLTE375, card.Yes, card.No, card.BuyOn(kv), "0.96", "0.97"},
		{PolyLTE375, card.No, card.Yes, card.BuyOn(kv), "0.05", "0.06"},
	}
	for _, c := range cases {
		r := e.Pair(cyc, card.RouteRecord, KalshiT375, c.poly, c.side)
		if r.Action != c.action || r.SideB != c.sideB {
			t.Errorf("%s %s: %s side_b %s, want %s side_b %s (%q)", c.poly, c.side, r.Action, r.SideB, c.action, c.sideB, r.Reason)
		}
		if got := price(r.PriceA) + "/" + price(r.PriceB); got != c.priceA+"/"+c.priB {
			t.Errorf("%s %s prices %s, want %s/%s", c.poly, c.side, got, c.priceA, c.priB)
		}
		if !r.PricedAtA.Equal(FetchedAt) || !r.PricedAtB.Equal(FetchedAt) || !r.AsOf.Equal(testkit.AsOf) {
			t.Errorf("%s %s times: a %v b %v as_of %v", c.poly, c.side, r.PricedAtA, r.PricedAtB, r.AsOf)
		}
		if r.FeeApplied || !strings.Contains(r.Reason, "Fees were not applied") {
			t.Errorf("%s %s: fees should not apply (both empty): %q", c.poly, c.side, r.Reason)
		}
		if !strings.HasPrefix(r.Reason, "Compared "+string(c.side)+" on kalshi") || !strings.HasSuffix(r.Reason, ".") {
			t.Errorf("%s %s: reason is not plain sentences: %q", c.poly, c.side, r.Reason)
		}
	}
}

func field(c card.Card, name string) string {
	for _, f := range c.Payoff.Fields() {
		if f.Name == name {
			return f.Value
		}
	}
	return ""
}

func price(o card.Opt[card.Micros]) string {
	if v, ok := o.Get(); ok {
		return v.String()
	}
	return ""
}

func recordsFor(recs []card.LogRecord, ref card.MarketRef) []card.LogRecord {
	var out []card.LogRecord
	for _, r := range recs {
		if r.MarketA == ref || r.MarketB == ref {
			out = append(out, r)
		}
	}
	return out
}
