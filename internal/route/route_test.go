package route

import (
	"context"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

// Neutral venue names: this package must not contain real ones (AGENTS.md hard rule 2).
var (
	roles = card.Roles{Reference: "ref", Counter: "ctr"}
	refA  = card.MarketRef{Venue: "ref", MarketID: "K1"}
	refB  = card.MarketRef{Venue: "ctr", MarketID: "P1"}
	asOf  = testkit.AsOf
)

func m(s string) card.Micros { v, _ := card.ParseMicros(s); return v }

func price(ref card.MarketRef, side card.Side, buy, fee string, age time.Duration) *card.PriceRecord {
	p := &card.PriceRecord{Venue: ref.Venue, MarketID: ref.MarketID, Side: side, PricedAt: asOf.Add(-age)}
	if buy != "" {
		p.BuyPrice = card.Some(m(buy))
	}
	if fee != "" {
		p.Fee = card.Some(m(fee))
	}
	return p
}

func input(match card.Result, side card.Side, a, b *card.PriceRecord) Input {
	return Input{Match: match, A: refA, B: refB, Side: side, PriceA: a, PriceB: b, AsOf: asOf, MaxAge: 10 * time.Minute, Roles: roles}
}

func want(t *testing.T, rec card.LogRecord, action card.Action, reasonHas ...string) {
	t.Helper()
	if rec.Action != action {
		t.Fatalf("action %s (%s), want %s", rec.Action, rec.Reason, action)
	}
	for _, s := range reasonHas {
		if !strings.Contains(rec.Reason, s) {
			t.Errorf("reason %q does not contain %q", rec.Reason, s)
		}
	}
}

func TestReq08_NotSameIsNoRoute(t *testing.T) {
	rec := Decide(input(card.NotSame, card.Yes, price(refA, card.Yes, "0.4", "", 0), price(refB, card.Yes, "0.3", "", 0)))
	want(t, rec, card.NoRoute, "not the same")
}

func TestReq09_MissingBuyPriceIsNoRoute(t *testing.T) {
	rec := Decide(input(card.Same, card.Yes, price(refA, card.Yes, "0.4", "", 0), nil))
	want(t, rec, card.NoRoute, "ctr yes price is missing for P1")
	rec = Decide(input(card.Same, card.Yes, price(refA, card.Yes, "", "", 0), price(refB, card.Yes, "0.4", "", 0)))
	want(t, rec, card.NoRoute, "ref yes price is missing for K1")
}

func TestReq10_StalePriceIsNoRoute(t *testing.T) {
	rec := Decide(input(card.Same, card.Yes, price(refA, card.Yes, "0.4", "", 11*time.Minute), price(refB, card.Yes, "0.3", "", time.Minute)))
	want(t, rec, card.NoRoute, "price is stale", "ref yes")
	// Exactly at the limit is still fresh.
	rec = Decide(input(card.Same, card.Yes, price(refA, card.Yes, "0.4", "", 10*time.Minute), price(refB, card.Yes, "0.3", "", 0)))
	want(t, rec, card.BuyOn("ctr"))
}

func TestReq11_LowerCostWinsAndFeesCanChangeIt(t *testing.T) {
	rec := Decide(input(card.Same, card.Yes, price(refA, card.Yes, "0.42", "", 0), price(refB, card.Yes, "0.45", "", 0)))
	want(t, rec, card.BuyOn("ref"), "ref is cheaper")
	// With known fees on both venues: 0.42 + 0.02 = 0.44 > 0.40 + 0.00 = 0.40.
	rec = Decide(input(card.Same, card.Yes, price(refA, card.Yes, "0.42", "0.02", 0), price(refB, card.Yes, "0.40", "0", 0)))
	want(t, rec, card.BuyOn("ctr"))
	rec = Decide(input(card.Same, card.Yes, price(refA, card.Yes, "0.40", "0.01", 0), price(refB, card.Yes, "0.39", "0.03", 0)))
	want(t, rec, card.BuyOn("ref"))
	if !rec.FeeApplied {
		t.Error("fees known on both venues must be applied")
	}
}

func TestReq12_FeeOnOneVenueOnlyComparesBuyPrice(t *testing.T) {
	rec := Decide(input(card.Same, card.Yes, price(refA, card.Yes, "0.40", "0.05", 0), price(refB, card.Yes, "0.42", "", 0)))
	want(t, rec, card.BuyOn("ref"), "Fees were not applied", "unknown on ctr")
	if rec.FeeApplied {
		t.Error("fee_applied must be no")
	}
}

func TestReq13_OppositeComparesYesWithNo(t *testing.T) {
	rec := Decide(input(card.Opposite, card.Yes, price(refA, card.Yes, "0.60", "", 0), price(refB, card.No, "0.55", "", 0)))
	want(t, rec, card.BuyOn("ctr"))
	if rec.SideA != card.Yes || rec.SideB != card.No {
		t.Errorf("sides %s/%s, want yes/no", rec.SideA, rec.SideB)
	}
}

func TestReq14_EqualCostChoosesReferenceVenue(t *testing.T) {
	rec := Decide(input(card.Same, card.No, price(refA, card.No, "0.5", "", 0), price(refB, card.No, "0.5", "", 0)))
	want(t, rec, card.BuyOn("ref"), "exact ties")
}

func TestReq15_SameInputsSameDecision(t *testing.T) {
	ctx := context.Background()
	st := testkit.TempStore(t)
	cfg := &card.Config{}
	cfg.Venues.Allowed = []string{"ref", "ctr"}
	cfg.Router.ReferenceVenue, cfg.Router.CounterVenue, cfg.Router.PriceMaxAge = "ref", "ctr", 10*time.Minute

	c, _ := st.Cycles.Begin(ctx, store.KindFetch, store.TriggerCLI)
	st.Log.Append(ctx, c, card.LogRecord{Type: card.MatchRecord, MarketA: refA, MarketB: refB, Match: card.Opposite, Reason: "opposite"})
	st.Prices.Append(ctx, c, *price(refA, card.Yes, "0.61", "", 2*time.Minute))
	st.Prices.Append(ctx, c, *price(refB, card.No, "0.58", "", 3*time.Minute))
	st.Prices.Append(ctx, c, *price(refB, card.No, "0.10", "", -time.Minute)) // saved after as_of: ignored (D-07)
	st.Cycles.Complete(ctx, c)

	var got []card.LogRecord
	for i := 0; i < 2; i++ {
		r, _ := st.Cycles.Begin(ctx, store.KindRoute, store.TriggerCLI)
		if _, err := Run(ctx, st, r, card.Yes, asOf, cfg); err != nil {
			t.Fatal(err)
		}
		st.Cycles.Complete(ctx, r)
		routes, _ := st.Log.LatestRoutes(ctx, store.Completed)
		got = append(got, routes...)
	}
	if len(got) != 2 || got[0].Action != got[1].Action || got[0].Reason != got[1].Reason {
		t.Fatalf("runs differ: %+v", got)
	}
	want(t, got[0], card.BuyOn("ctr"), "0.58")
}

func TestRoute_DecideIsPure(t *testing.T) {
	in := input(card.Same, card.Yes, price(refA, card.Yes, "0.42", "0.01", 0), price(refB, card.Yes, "0.42", "0.01", 0))
	a, b := Decide(in), Decide(in)
	if a.Action != b.Action || a.Reason != b.Reason || a.FeeApplied != b.FeeApplied {
		t.Error("Decide is not deterministic")
	}
}
