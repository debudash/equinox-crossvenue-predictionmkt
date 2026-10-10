package redteam_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"equinox/internal/acceptance"
	"equinox/internal/card"
	"equinox/internal/fetch/kalshi"
	"equinox/internal/fetch/polymarket"
	"equinox/internal/match"
	"equinox/internal/route"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

const (
	kv  = kalshi.Venue
	pv  = polymarket.Venue
	k   = acceptance.KalshiT375
	gt  = acceptance.PolyGT375
	lte = acceptance.PolyLTE375
)

var fetchedAt = acceptance.FetchedAt

func kalshiSynth(t *testing.T) string { return acceptance.Fixture(t, "kalshi", "synthetic") }
func polySynth(t *testing.T) string   { return acceptance.Fixture(t, "polymarket", "synthetic") }

// fetchPair fetches a Kalshi and a Polymarket scenario into a fresh store and
// returns the env and the cycle.
func fetchPair(t *testing.T, kdir, pdir string, at time.Time) (*acceptance.Env, int64) {
	t.Helper()
	e := acceptance.NewEnv(t)
	cyc := e.Begin()
	if run := e.FetchKalshi(cyc, kdir, at); run.Status != store.RunOK {
		t.Fatalf("kalshi run = %+v", run)
	}
	if run := e.FetchPolymarket(cyc, pdir, at); run.Status != store.RunOK {
		t.Fatalf("polymarket run = %+v", run)
	}
	return e, cyc
}

func valueOf(c card.Card, name string) (string, bool) {
	for _, f := range c.Payoff.Fields() {
		if f.Name == name {
			return f.Value, f.Set
		}
	}
	return "", false
}

// R-04: "3.50" on one venue and "3.5" on the other normalize to the same
// canonical value, so the pair can match.
func TestRT_R04_TrailingZeroCutoffNormalizesAndMatches(t *testing.T) {
	cases := []struct {
		name         string
		kedit, pedit []acceptance.Edit
	}{
		{"polymarket writes 3.750%", nil, []acceptance.Edit{{File: "markets_0.json", Old: "greater than 3.75%", New: "greater than 3.750%"}}},
		{"kalshi strike is 3.750", []acceptance.Edit{{File: "events_kxfed.json", Old: `"floor_strike": 3.75,`, New: `"floor_strike": 3.750,`}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, cyc := fetchPair(t, acceptance.Variant(t, kalshiSynth(t), c.kedit...), acceptance.Variant(t, polySynth(t), c.pedit...), fetchedAt)
			kc, pc := e.Card(cyc, kv, k), e.Card(cyc, pv, gt)
			for _, cd := range []card.Card{kc, pc} {
				if v, _ := valueOf(cd, card.FieldCutoffValue); v != "3.75" {
					t.Errorf("%s cutoff_value = %q, want 3.75", cd.Ref(), v)
				}
			}
			if r := match.Compare(kc, pc); r.Result != card.Same {
				t.Fatalf("Compare = %+v, want same", r)
			}
		})
	}
	a, _ := card.ParseDecimal("3.50")
	b, _ := card.ParseDecimal("3.5")
	if a != b || a.String() != "3.5" {
		t.Fatalf("ParseDecimal: %q vs %q", a, b)
	}
}

// R-05: a deadline given in a local time zone is converted to UTC before
// comparing. Polymarket US writes Eastern time (with daylight saving for the
// date); a Kalshi strike_date with an offset converts the same way.
func TestRT_R05_LocalTimeDeadlineConvertedToUTC(t *testing.T) {
	cases := []struct {
		name, want   string
		kedit, pedit []acceptance.Edit
	}{
		{"winter ET vs offset", "2026-12-09T19:00:00Z",
			[]acceptance.Edit{{File: "events_kxfed.json", Old: `"strike_date": "2026-12-09T19:00:00Z"`, New: `"strike_date": "2026-12-09T14:00:00-05:00"`}},
			nil},
		{"summer ET (daylight saving)", "2026-07-29T18:00:00Z",
			[]acceptance.Edit{{File: "events_kxfed.json", Old: `"strike_date": "2026-12-09T19:00:00Z"`, New: `"strike_date": "2026-07-29T18:00:00Z"`}},
			[]acceptance.Edit{{File: "markets_0.json", Old: "December 9, 2026 at 2:00 PM ET", New: "July 29, 2026 at 2:00 PM ET"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, cyc := fetchPair(t, acceptance.Variant(t, kalshiSynth(t), c.kedit...), acceptance.Variant(t, polySynth(t), c.pedit...), fetchedAt)
			kc, pc := e.Card(cyc, kv, k), e.Card(cyc, pv, gt)
			for _, cd := range []card.Card{kc, pc} {
				if v, _ := valueOf(cd, card.FieldDeadline); v != c.want {
					t.Errorf("%s deadline = %q, want %q", cd.Ref(), v, c.want)
				}
			}
			if r := match.Compare(kc, pc); r.Result != card.Same {
				t.Fatalf("Compare = %+v, want same", r)
			}
		})
	}
}

// R-06: a different unclear_result is not_same, and the reason names it. Both
// a different value and a missing fallback sentence are refused.
func TestRT_R06_UnclearResultDiffersIsNotSame(t *testing.T) {
	e, cyc := fetchPair(t, kalshiSynth(t), polySynth(t), fetchedAt)
	kc, pc := e.Card(cyc, kv, k), e.Card(cyc, pv, gt)
	pc.Payoff.UnclearResult = card.Some(card.LastPrice)
	r := match.Compare(kc, pc)
	if r.Result != card.NotSame || !strings.Contains(r.Reason, "unclear_result differs") ||
		!strings.Contains(r.Reason, "yes_or_no_only") || !strings.Contains(r.Reason, "last_price") {
		t.Fatalf("last_price vs yes_or_no_only = %+v", r)
	}

	// The same market without the fallback sentence has no unclear_result (D-20).
	pdir := acceptance.Variant(t, polySynth(t), acceptance.Edit{File: "markets_0.json",
		Old: " If the outcome cannot be determined, this market resolves to No.", New: ""})
	e2, cyc2 := fetchPair(t, kalshiSynth(t), pdir, fetchedAt)
	e2.Match(cyc2)
	rec := e2.Pair(cyc2, card.MatchRecord, k, gt, "")
	if rec.Match != card.NotSame || !strings.Contains(rec.Reason, "unclear_result is empty on polymarket:"+gt) {
		t.Fatalf("no fallback sentence: %s %q", rec.Match, rec.Reason)
	}
}

// R-07: when both cards have the same field empty the pair is not_same; empty
// never equals empty.
func TestRT_R07_BothEmptyIsNotSame(t *testing.T) {
	if card.None[string]().Equal(card.None[string]()) {
		t.Fatal("empty equals empty")
	}
	kdir := acceptance.Variant(t, kalshiSynth(t), acceptance.Edit{File: "events_kxfed.json",
		Old: `"strike_date": "2026-12-09T19:00:00Z"`, New: `"strike_date": ""`})
	pdir := acceptance.Variant(t, polySynth(t), acceptance.Edit{File: "markets_0.json",
		Old: "December 9, 2026 at 2:00 PM ET", New: "December 9, 2026"})
	e, cyc := fetchPair(t, kdir, pdir, fetchedAt)
	kc, pc := e.Card(cyc, kv, k), e.Card(cyc, pv, gt)
	if !kc.Payoff.Deadline.IsEmpty() || !pc.Payoff.Deadline.IsEmpty() {
		t.Fatalf("deadlines should both be empty: %v / %v", kc.Payoff.Fields(), pc.Payoff.Fields())
	}
	e.Match(cyc)
	for _, id := range []string{gt, lte} {
		rec := e.Pair(cyc, card.MatchRecord, k, id, "")
		if rec.Match != card.NotSame || !strings.Contains(rec.Reason, "deadline is empty on both cards") {
			t.Errorf("%s: %s %q", id, rec.Match, rec.Reason)
		}
	}
	if s := e.Route(cyc, card.Yes, testkit.AsOf); s.Routed+s.NoRoute != 0 {
		t.Fatalf("route = %+v; a not_same pair is never routed", s)
	}
}

// R-09: an empty side of the book (Kalshi reports it as 1.0000, Polymarket as
// no bids) is stored as an empty buy_price, never 0 or 1, and no_route names it.
func TestRT_R09_EmptyBookSideIsEmptyPriceAndNoRoute(t *testing.T) {
	kdir := acceptance.Variant(t, kalshiSynth(t), acceptance.Edit{File: "events_kxfed.json",
		Old: `"yes_ask_dollars": "0.9600"`, New: `"yes_ask_dollars": "1.0000"`})
	pdir := acceptance.Variant(t, polySynth(t), acceptance.Edit{File: "books/" + gt + ".json",
		Old: `"bids":[{"px":{"value":"0.9300","currency":"USD"},"qty":"200.0000"},{"px":{"value":"0.9100","currency":"USD"},"qty":"400.0000"}]`,
		New: `"bids":[]`})
	e, cyc := fetchPair(t, kdir, pdir, fetchedAt)
	for _, c := range []struct {
		venue, id string
		side      card.Side
	}{{kv, k, card.Yes}, {pv, gt, card.No}} {
		p, ok, err := e.St.Prices.LatestAt(e.Ctx, card.MarketRef{Venue: c.venue, MarketID: c.id}, c.side, testkit.AsOf, store.Writing(cyc))
		if err != nil || !ok {
			t.Fatalf("no %s %s price record: %v", c.id, c.side, err)
		}
		if !p.BuyPrice.IsEmpty() {
			v, _ := p.BuyPrice.Get()
			t.Errorf("%s %s buy_price = %s, want empty", c.id, c.side, v)
		}
	}
	e.Match(cyc)
	e.Route(cyc, card.Yes, testkit.AsOf)
	e.Route(cyc, card.No, testkit.AsOf)
	if r := e.Pair(cyc, card.RouteRecord, k, gt, card.Yes); r.Action != card.NoRoute || r.Reason != "No route: The kalshi yes price is missing for "+k+"." {
		t.Errorf("yes: %s %q", r.Action, r.Reason)
	}
	if r := e.Pair(cyc, card.RouteRecord, k, gt, card.No); r.Action != card.NoRoute || r.Reason != "No route: The polymarket no price is missing for "+gt+"." {
		t.Errorf("no: %s %q", r.Action, r.Reason)
	}
}

// editSides rewrites the marketSides of every market in a Polymarket markets
// file.
func editSides(t *testing.T, path string, fn func(sides []any) []any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var page map[string]any
	if err := dec.Decode(&page); err != nil {
		t.Fatal(err)
	}
	for _, m := range page["markets"].([]any) {
		mm := m.(map[string]any)
		mm["marketSides"] = fn(mm["marketSides"].([]any))
	}
	out, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// R-10: outcome tokens listed in reverse order: YES is chosen by its label and
// long flag, never by position. Sides whose labels do not say which is YES are
// skipped, not guessed.
func TestRT_R10_ReversedOutcomeOrderChoosesYesByLabel(t *testing.T) {
	pdir := acceptance.Variant(t, polySynth(t))
	editSides(t, filepath.Join(pdir, "markets_0.json"), func(s []any) []any { return []any{s[1], s[0]} })
	e, cyc := fetchPair(t, kalshiSynth(t), pdir, fetchedAt)
	if r := match.Compare(e.Card(cyc, kv, k), e.Card(cyc, pv, gt)); r.Result != card.Same {
		t.Fatalf("reversed sides: Compare = %+v", r)
	}
	e.Match(cyc)
	e.Route(cyc, card.Yes, testkit.AsOf)
	// YES on Polymarket is still the best offer 0.95, not the NO side.
	if r := e.Pair(cyc, card.RouteRecord, k, gt, card.Yes); r.Action != card.BuyOn(pv) || !strings.Contains(r.Reason, "yes on polymarket at 0.95") {
		t.Fatalf("reversed sides route: %s %q", r.Action, r.Reason)
	}

	// Labels swapped against the long flag: the market is skipped with a reason.
	bad := acceptance.Variant(t, polySynth(t))
	editSides(t, filepath.Join(bad, "markets_0.json"), func(s []any) []any {
		a, b := s[0].(map[string]any), s[1].(map[string]any)
		a["description"], b["description"] = b["description"], a["description"]
		return s
	})
	e2 := acceptance.NewEnv(t)
	cyc2 := e2.Begin()
	run := e2.FetchPolymarket(cyc2, bad, fetchedAt)
	if run.Status != store.RunOK || len(e2.Cards(cyc2)) != 0 {
		t.Fatalf("mislabelled sides: run %+v, %d cards", run, len(e2.Cards(cyc2)))
	}
	n := 0
	for _, s := range e2.Skips(cyc2) {
		if strings.Contains(s.Reason, "not labeled Yes (long) and No (short)") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("%d mislabelled markets skipped with a reason, want 2", n)
	}
}

// R-11: a price exactly 10 minutes old is current; 10 minutes and 1 second is
// stale.
func TestRT_R11_PriceAgeBoundary(t *testing.T) {
	for _, c := range []struct {
		age   time.Duration
		fresh bool
	}{{10 * time.Minute, true}, {10*time.Minute + time.Second, false}} {
		e, cyc := fetchPair(t, kalshiSynth(t), polySynth(t), testkit.AsOf.Add(-c.age))
		e.Match(cyc)
		s := e.Route(cyc, card.Yes, testkit.AsOf)
		r := e.Pair(cyc, card.RouteRecord, k, gt, card.Yes)
		if c.fresh {
			if s.Routed != 2 || r.Action == card.NoRoute {
				t.Errorf("age %s: %+v %q; want routed", c.age, s, r.Reason)
			}
			continue
		}
		if s.NoRoute != 2 || r.Action != card.NoRoute || !strings.Contains(r.Reason, "stale") ||
			!strings.Contains(r.Reason, "10m1s before as_of, over the 10m0s limit") {
			t.Errorf("age %s: %+v %q; want no_route, stale", c.age, s, r.Reason)
		}
	}
}

// R-12: a price saved after as_of is ignored for that as_of: the router uses
// the latest price saved at or before as_of, and never one from the future.
func TestRT_R12_PriceSavedAfterAsOfIgnored(t *testing.T) {
	e := acceptance.NewEnv(t)
	c1 := e.Begin()
	e.FetchSynthetic(c1, fetchedAt)
	e.Match(c1)
	e.Complete(c1)

	// A later fetch, one minute after as_of, where Kalshi yes is cheaper.
	later := testkit.AsOf.Add(time.Minute)
	c2 := e.Begin()
	e.FetchKalshi(c2, acceptance.Variant(t, kalshiSynth(t), acceptance.Edit{File: "events_kxfed.json",
		Old: `"yes_ask_dollars": "0.9600"`, New: `"yes_ask_dollars": "0.9000"`}), later)
	e.FetchPolymarket(c2, polySynth(t), later)
	e.Route(c2, card.Yes, testkit.AsOf)
	r := e.Pair(c2, card.RouteRecord, k, gt, card.Yes)
	if r.Action != card.BuyOn(pv) || !r.PricedAtA.Equal(fetchedAt) || !strings.Contains(r.Reason, "kalshi at 0.96") {
		t.Fatalf("at as_of: %s priced_at_a %v %q; want the 0.96 price saved before as_of", r.Action, r.PricedAtA, r.Reason)
	}
	e.Complete(c2)

	c3 := e.Begin()
	e.Route(c3, card.Yes, later)
	if r := e.Pair(c3, card.RouteRecord, k, gt, card.Yes); r.Action != card.BuyOn(kv) || !r.PricedAtA.Equal(later) {
		t.Fatalf("at the later as_of: %s %v %q; want the 0.90 price", r.Action, r.PricedAtA, r.Reason)
	}

	// The pure decision refuses a price stamped after as_of if handed one.
	p := card.PriceRecord{Venue: kv, MarketID: k, Side: card.Yes, BuyPrice: card.Some(card.Micros(900_000)), PricedAt: later}
	q := card.PriceRecord{Venue: pv, MarketID: gt, Side: card.Yes, BuyPrice: card.Some(card.Micros(950_000)), PricedAt: fetchedAt}
	d := route.Decide(route.Input{Match: card.Same, A: card.MarketRef{Venue: kv, MarketID: k}, B: card.MarketRef{Venue: pv, MarketID: gt}, Side: card.Yes, PriceA: &p, PriceB: &q,
		AsOf: testkit.AsOf, MaxAge: e.Cfg.Router.PriceMaxAge, Roles: e.Cfg.Roles()})
	if d.Action != card.NoRoute || !strings.Contains(d.Reason, "saved after as_of") {
		t.Fatalf("Decide = %s %q", d.Action, d.Reason)
	}
}

// R-18: a card with an unknown venue value in saved data is skipped as
// inconsistent, logged, and never compared.
func TestRT_R18_UnknownVenueSkippedAsInconsistent(t *testing.T) {
	e := acceptance.NewEnv(t)
	cyc := e.Begin()
	e.FetchSynthetic(cyc, fetchedAt)
	odd := e.Card(cyc, pv, gt)
	odd.Venue = "predictit"
	if _, err := e.St.Cards.Save(e.Ctx, cyc, odd); err != nil {
		t.Fatal(err)
	}
	sum := e.Match(cyc)
	if sum.Skipped != 1 || sum.Same != 1 || sum.Opposite != 1 {
		t.Fatalf("match = %+v; want the odd card skipped and the real pairs intact", sum)
	}
	found := false
	for _, s := range e.Skips(cyc) {
		if s.Stage == store.StageMatch && s.Venue == "predictit" && strings.Contains(s.Reason, `unknown venue "predictit"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no match-stage skip naming the unknown venue: %+v", e.Skips(cyc))
	}
	for _, r := range e.Log(cyc, card.MatchRecord) {
		if r.MarketA.Venue == "predictit" || r.MarketB.Venue == "predictit" {
			t.Fatalf("unknown venue card was compared: %+v", r)
		}
	}
	p := card.PriceRecord{Venue: "predictit", MarketID: gt, Side: card.Yes, PricedAt: fetchedAt}
	if err := card.CheckPriceRecord(p, e.Cfg.Venues.Allowed); !errors.Is(err, card.ErrInconsistent) {
		t.Fatalf("CheckPriceRecord = %v, want inconsistent", err)
	}
}

// R-19: a price outside 0 to 1 is skipped as inconsistent, on either venue,
// and never reaches a decision.
func TestRT_R19_PriceOutsideZeroToOneSkipped(t *testing.T) {
	// Kalshi: the recorded malformed fixture has a yes ask of 1.25; the whole
	// market is skipped with a reason.
	e := acceptance.NewEnv(t)
	cyc := e.Begin()
	e.FetchKalshi(cyc, acceptance.Fixture(t, "kalshi", "malformed"), fetchedAt)
	if _, ok := e.FindCard(cyc, kv, "KXFED-26DEC-T3.50"); ok {
		t.Error("market with a 1.25 ask was saved")
	}
	if _, ok, _ := e.St.Prices.LatestAt(e.Ctx, card.MarketRef{Venue: kv, MarketID: "KXFED-26DEC-T3.50"}, card.Yes, testkit.AsOf, store.Writing(cyc)); ok {
		t.Error("price outside 0 to 1 was saved")
	}
	if !hasSkip(e.Skips(cyc), "KXFED-26DEC-T3.50", "outside 0 to 1") {
		t.Errorf("no skip naming the price: %+v", e.Skips(cyc))
	}

	// Polymarket: a book offer of 1.25 makes that side empty, logged as a skip,
	// so the pair is no_route for yes.
	pdir := acceptance.Variant(t, polySynth(t), acceptance.Edit{File: "books/" + gt + ".json",
		Old: `"offers":[{"px":{"value":"0.9500"`, New: `"offers":[{"px":{"value":"1.2500"`})
	e2, cyc2 := fetchPair(t, kalshiSynth(t), pdir, fetchedAt)
	if !hasSkip(e2.Skips(cyc2), gt, "outside 0 to 1") {
		t.Errorf("no skip naming the book price: %+v", e2.Skips(cyc2))
	}
	e2.Match(cyc2)
	e2.Route(cyc2, card.Yes, testkit.AsOf)
	if r := e2.Pair(cyc2, card.RouteRecord, k, gt, card.Yes); r.Action != card.NoRoute || !strings.Contains(r.Reason, "polymarket yes price is missing") {
		t.Errorf("route: %s %q", r.Action, r.Reason)
	}
	if err := card.CheckPrice(card.Micros(1_250_000)); !errors.Is(err, card.ErrInconsistent) {
		t.Errorf("CheckPrice(1.25) = %v", err)
	}
}

// R-19, saved data: a price record outside 0 to 1 already in the store (for
// example written by hand or by an older build) must be treated as
// inconsistent by the router, not used as a price.
func TestRT_R19_SavedPriceOutsideZeroToOneNotUsedByRouter(t *testing.T) {
	e := acceptance.NewEnv(t)
	cyc := e.Begin()
	e.FetchSynthetic(cyc, fetchedAt)
	e.Match(cyc)
	bad := card.PriceRecord{Venue: kv, MarketID: k, Side: card.Yes, BuyPrice: card.Some(card.Micros(1_250_000)), PricedAt: fetchedAt.Add(time.Second)}
	if err := e.St.Prices.Append(e.Ctx, cyc, bad); err != nil {
		t.Fatal(err)
	}
	e.Route(cyc, card.Yes, testkit.AsOf)
	r := e.Pair(cyc, card.RouteRecord, k, gt, card.Yes)
	if r.Action != card.NoRoute {
		t.Errorf("route = %s %q; want no_route for an inconsistent saved price", r.Action, r.Reason)
	}
}

func hasSkip(skips []store.Skip, id, substr string) bool {
	for _, s := range skips {
		if s.MarketID == id && strings.Contains(s.Reason, substr) {
			return true
		}
	}
	return false
}

// R-20: a Kalshi multivariate combo market is skipped and counted, never stored.
func TestRT_R20_KalshiComboMarketSkippedAndCounted(t *testing.T) {
	e := acceptance.NewEnv(t)
	cyc := e.Begin()
	run := e.FetchKalshi(cyc, acceptance.Fixture(t, "kalshi", "malformed"), fetchedAt)
	if run.Status != store.RunOK {
		t.Fatalf("run = %+v", run)
	}
	combos := 0
	for _, s := range e.Skips(cyc) {
		if strings.HasPrefix(s.MarketID, "KXMVE") && strings.Contains(s.Reason, "multivariate combo") {
			combos++
		}
	}
	if combos != 1 || run.Skipped < combos {
		t.Fatalf("%d combo skips, run skipped %d", combos, run.Skipped)
	}
	for _, c := range e.Cards(cyc) {
		if strings.HasPrefix(c.MarketID, "KXMVE") {
			t.Fatalf("combo market stored as a card: %s", c.MarketID)
		}
	}
}

// R-22: no code path updates or deletes a log record. The log repositories
// expose no such method, and SQLite triggers abort a direct UPDATE or DELETE.
func TestRT_R22_LogRecordsCannotBeUpdatedOrDeleted(t *testing.T) {
	for _, repo := range []any{&store.DecisionLog{}, &store.Skips{}, &store.SubjectCounts{}} {
		ty := reflect.TypeOf(repo)
		for i := 0; i < ty.NumMethod(); i++ {
			name := ty.Method(i).Name
			for _, bad := range []string{"Update", "Delete", "Edit", "Remove", "Set", "Clear", "Prune", "Replace"} {
				if strings.Contains(name, bad) {
					t.Errorf("%s has method %s", ty, name)
				}
			}
		}
	}

	path := filepath.Join(t.TempDir(), "equinox.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.Clock = testkit.FixedClock(testkit.AsOf)
	e := &acceptance.Env{T: t, Ctx: t.Context(), St: st}
	cyc := e.Begin()
	ref := card.MarketRef{Venue: kv, MarketID: k}
	other := card.MarketRef{Venue: pv, MarketID: gt}
	if _, err := st.Log.Append(e.Ctx, cyc, card.LogRecord{Type: card.MatchRecord, MarketA: ref, MarketB: other, Match: card.Same, Reason: "All seven payoff fields are equal."}); err != nil {
		t.Fatal(err)
	}
	if err := st.Skips.Append(e.Ctx, store.Skip{CycleID: cyc, Stage: store.StageFetch, Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Subjects.Append(e.Ctx, cyc, []store.SubjectCount{{Subject: "fed_funds_upper_bound", Venue: kv, Count: 1}}); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`UPDATE decision_log SET match_result = 'not_same', reason = 'edited'`,
		`DELETE FROM decision_log`,
		`UPDATE skips SET reason = 'edited'`,
		`DELETE FROM skips`,
		`UPDATE subject_counts SET count = 99`,
		`DELETE FROM subject_counts`,
	} {
		if _, err := db.Exec(stmt); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: err = %v, want an append-only abort", stmt, err)
		}
	}
	recs, err := st.Log.List(e.Ctx, store.Writing(cyc))
	if err != nil || len(recs) != 1 || recs[0].Match != card.Same || recs[0].Reason != "All seven payoff fields are equal." {
		t.Fatalf("log after attempts = %+v, %v", recs, err)
	}
}

// R-27: a venue returning rate-limit responses is backed off (1s, 2s, 4s, as
// documented) and the failure is recorded on its run; the other venue
// continues and its cards are saved.
func TestRT_R27_RateLimitBacksOffAndOtherVenueContinues(t *testing.T) {
	t.Parallel()
	e := acceptance.NewEnv(t)
	cyc := e.Begin()
	tr := testkit.Scenario(t, acceptance.Fixture(t, "polymarket", "rate_limited"))
	f := polymarket.New(e.Cfg.Fetchers[pv], e.Vocab, tr.Client(), testkit.FixedClock(fetchedAt))
	start := time.Now()
	pr, err := f.Fetch(e.Ctx, e.St, cyc)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Fetch returned an error instead of a failed run: %v", err)
	}
	if pr.Status != store.RunFailed || !strings.Contains(pr.Error, "HTTP 429") {
		t.Fatalf("polymarket run = %+v; want failed, naming HTTP 429", pr)
	}
	if n := len(tr.Requests()); n != 4 {
		t.Fatalf("%d requests, want 1 plus 3 retries", n)
	}
	if elapsed < 7*time.Second {
		t.Fatalf("finished in %s; the 1s, 2s, 4s backoff was not applied", elapsed)
	}

	kr := e.FetchKalshi(cyc, kalshiSynth(t), fetchedAt)
	if kr.Status != store.RunOK || kr.Saved != 2 {
		t.Fatalf("kalshi run = %+v; want ok with 2 cards", kr)
	}
	e.Card(cyc, kv, k)
	if sum := e.Match(cyc); sum.Compared != 0 {
		t.Fatalf("match = %+v; no Polymarket card exists", sum)
	}
	e.Complete(cyc)
}
