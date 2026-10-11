package watch

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/fetch/registry"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

// fake is a hand-built fetcher: it saves the cards and prices it is given, so
// scheduler tests control exactly what changes between passes.
type fake struct {
	venue     string
	cards     []card.Card
	prices    map[string][2]string // market id -> yes, no buy price
	fail      bool
	clock     func() time.Time
	refreshed [][]string
}

func (f *fake) Name() string { return f.venue }

func (f *fake) Fetch(ctx context.Context, st *store.Store, cycle int64) (store.FetchRun, error) {
	run, _ := st.Runs.Start(ctx, cycle, f.venue)
	if f.fail {
		st.Runs.Finish(ctx, run, store.RunFailed, 0, 0, "HTTP 500")
		return store.FetchRun{Venue: f.venue, Status: store.RunFailed, Error: "HTTP 500"}, nil
	}
	st.Raw.Save(ctx, store.Raw{CycleID: cycle, RunID: run, Venue: f.venue, Kind: store.RawMarkets, Endpoint: "/m", FetchedAt: f.clock(), Body: []byte("{}")})
	for _, c := range f.cards {
		if _, err := st.Cards.Save(ctx, cycle, c); err != nil {
			return store.FetchRun{}, err
		}
	}
	st.Runs.Finish(ctx, run, store.RunOK, len(f.cards), 0, "")
	return store.FetchRun{Venue: f.venue, Status: store.RunOK, Saved: len(f.cards)}, nil
}

func (f *fake) FetchPrices(ctx context.Context, st *store.Store, cycle int64, ids []string) (store.FetchRun, error) {
	f.refreshed = append(f.refreshed, ids)
	st.Raw.Save(ctx, store.Raw{CycleID: cycle, Venue: f.venue, Kind: store.RawPrices, Endpoint: "/p", FetchedAt: f.clock(), Body: []byte("{}")})
	for _, id := range ids {
		p := f.prices[id]
		for i, side := range []card.Side{card.Yes, card.No} {
			m, _ := card.ParseMicros(p[i])
			if err := st.Prices.Append(ctx, cycle, card.PriceRecord{Venue: f.venue, MarketID: id, Side: side, BuyPrice: card.Some(m), PricedAt: f.clock()}); err != nil {
				return store.FetchRun{}, err
			}
		}
	}
	return store.FetchRun{Venue: f.venue, Status: store.RunOK, Saved: 2 * len(ids)}, nil
}

func fedCard(venue, id string, op card.Op, value string) card.Card {
	v, _ := card.ParseDecimal(value)
	d, _ := card.ParseInstant("2026-12-09T19:00:00Z")
	return card.Card{Venue: venue, MarketID: id, Payoff: card.Payoff{
		Subject: card.Some("fed_funds_upper_bound"), CutoffOp: card.Some(op), CutoffValue: card.Some(v),
		CutoffUnit: card.Some("percent"), Deadline: card.Some(d), Decider: card.Some("federal_reserve"),
		UnclearResult: card.Some(card.YesOrNoOnly)},
		Record: card.RecordFields{Title: id, FetchedAt: testkit.AsOf}}
}

type rig struct {
	w      *Watcher
	ref    *fake
	ctr    *fake
	now    time.Time
	logbuf *bytes.Buffer
}

// newRig: ref has K1 (gt 3.75) and K2 (gt 4); ctr has P1 (gt 3.75, same as K1)
// and P2 (lte 4, opposite of K2), plus unmatched P9.
func newRig(t *testing.T) *rig {
	st := testkit.TempStore(t)
	cfg := &card.Config{}
	cfg.Venues.Allowed = []string{"ref", "ctr"}
	cfg.Router.ReferenceVenue, cfg.Router.CounterVenue, cfg.Router.PriceMaxAge = "ref", "ctr", 10*time.Minute
	cfg.Watch.DiscoveryEvery, cfg.Watch.PricesEvery, cfg.Watch.Retention = 30*time.Minute, 2*time.Minute, 24*time.Hour
	r := &rig{now: testkit.AsOf, logbuf: &bytes.Buffer{}}
	clock := func() time.Time { return r.now }
	r.ref = &fake{venue: "ref", clock: clock, cards: []card.Card{fedCard("ref", "K1", card.Gt, "3.75"), fedCard("ref", "K2", card.Gt, "4")},
		prices: map[string][2]string{"K1": {"0.40", "0.62"}, "K2": {"0.20", "0.81"}}}
	r.ctr = &fake{venue: "ctr", clock: clock, cards: []card.Card{fedCard("ctr", "P1", card.Gt, "3.75"), fedCard("ctr", "P2", card.Lte, "4"), fedCard("ctr", "P9", card.Gt, "9")},
		prices: map[string][2]string{"P1": {"0.42", "0.60"}, "P2": {"0.78", "0.25"}, "P9": {"0.01", "0.99"}}}
	r.w = &Watcher{St: st, Cfg: cfg, Fetchers: []registry.Fetcher{r.ref, r.ctr}, Clock: clock, Log: r.logbuf, Timeout: time.Minute}
	return r
}

func (r *rig) records(t *testing.T, typ card.RecordType) []card.LogRecord {
	t.Helper()
	all, err := r.w.St.Log.List(context.Background(), store.Completed)
	if err != nil {
		t.Fatal(err)
	}
	var out []card.LogRecord
	for _, rec := range all {
		if rec.Type == typ {
			out = append(out, rec)
		}
	}
	return out
}

func TestAmd_W1_MatchRunsOnlyAfterCardsChanged(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	if err := r.w.Discovery(ctx); err != nil {
		t.Fatal(err)
	}
	first := len(r.records(t, card.MatchRecord))
	if first == 0 {
		t.Fatal("new cards must trigger match")
	}
	r.now = r.now.Add(30 * time.Minute)
	r.w.Discovery(ctx)
	if n := len(r.records(t, card.MatchRecord)); n != first {
		t.Errorf("unchanged cards must not re-run match: %d then %d records", first, n)
	}
	// A new version that changes only display data (title here; volume and url alike) cannot change a match
	// result, because the matcher reads only payoff fields, so it must not re-run match (decision D-21).
	r.ctr.cards[0].Record.Title = "renamed"
	r.now = r.now.Add(30 * time.Minute)
	r.w.Discovery(ctx)
	if n := len(r.records(t, card.MatchRecord)); n != first {
		t.Errorf("a display-only change must not re-run match: %d then %d records", first, n)
	}
	v, _ := card.ParseDecimal("4")
	r.ctr.cards[0].Payoff.CutoffValue = card.Some(v)
	r.now = r.now.Add(30 * time.Minute)
	r.w.Discovery(ctx)
	if n := len(r.records(t, card.MatchRecord)); n != 2*first {
		t.Errorf("a changed payoff field must re-run match: %d then %d records", first, n)
	}
}

func TestAmd_W2_UnchangedPriceStillSavedEachPoll(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.w.Discovery(ctx)
	for i := 0; i < 8; i++ { // 16 minutes of steady prices
		r.now = r.now.Add(2 * time.Minute)
		if err := r.w.Prices(ctx); err != nil {
			t.Fatal(err)
		}
	}
	p, ok, _ := r.w.St.Prices.LatestAt(ctx, card.MarketRef{Venue: "ref", MarketID: "K1"}, card.Yes, r.now, store.Completed)
	if !ok || !p.PricedAt.Equal(r.now) {
		t.Fatalf("latest price must be from the last poll: %+v", p)
	}
	routes := r.records(t, card.RouteRecord)
	if len(routes) != 4 { // first poll only: 2 pairs x 2 sides
		t.Errorf("steady prices must not re-route: %d route records", len(routes))
	}
	for _, rec := range routes {
		if rec.Action == card.NoRoute {
			t.Errorf("fresh steady prices must route: %s", rec.Reason)
		}
	}
	for _, ids := range r.ctr.refreshed {
		for _, id := range ids {
			if id == "P9" {
				t.Error("unmatched markets must not be refreshed")
			}
		}
	}
}

func TestAmd_W3_ChangedPriceReroutesThatPairOnly(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.w.Discovery(ctx)
	r.w.Prices(ctx)
	before := len(r.records(t, card.RouteRecord))
	r.ctr.prices["P1"] = [2]string{"0.38", "0.60"}
	r.now = r.now.Add(2 * time.Minute)
	r.w.Prices(ctx)
	routes := r.records(t, card.RouteRecord)[before:]
	if len(routes) != 2 {
		t.Fatalf("want 2 new route records for the changed pair, got %d", len(routes))
	}
	for _, rec := range routes {
		if rec.MarketA.MarketID != "K1" || rec.MarketB.MarketID != "P1" || !rec.AsOf.Equal(r.now) {
			t.Errorf("route must be for K1/P1 at the refresh time: %+v", rec)
		}
	}
}

func TestAmd_W9_PriceChangeRoutesBothSides(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.w.Discovery(ctx)
	r.w.Prices(ctx)
	before := len(r.records(t, card.RouteRecord))
	r.ref.prices["K2"] = [2]string{"0.25", "0.70"}
	r.now = r.now.Add(2 * time.Minute)
	r.w.Prices(ctx)
	routes := r.records(t, card.RouteRecord)[before:]
	if len(routes) != 2 || routes[0].SideA == routes[1].SideA || !routes[0].AsOf.Equal(routes[1].AsOf) {
		t.Fatalf("want one yes and one no route record with the same as_of: %+v", routes)
	}
	if routes[0].Match != card.Opposite || routes[0].SideB == routes[0].SideA {
		t.Errorf("opposite pair must compare the other side on the counter venue: %+v", routes[0])
	}
}

func TestAmd_W4_VenueFailureThenNormalCycle(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.ctr.fail = true
	if err := r.w.Discovery(ctx); err != nil {
		t.Fatalf("a venue failure must not fail the cycle: %v", err)
	}
	c1, ok, _ := r.w.St.Cycles.LatestCompleted(ctx)
	runs, _ := r.w.St.Runs.ForCycle(ctx, c1.ID)
	if !ok || len(runs) != 2 || runs[0].Status != store.RunFailed && runs[1].Status != store.RunFailed {
		t.Fatalf("failure must be saved in the completed cycle: %+v", runs)
	}
	r.ctr.fail = false
	r.now = r.now.Add(30 * time.Minute)
	r.w.Discovery(ctx)
	c2, _, _ := r.w.St.Cycles.LatestCompleted(ctx)
	runs, _ = r.w.St.Runs.ForCycle(ctx, c2.ID)
	for _, run := range runs {
		if run.Status != store.RunOK {
			t.Errorf("next cycle must run normally: %+v", run)
		}
	}
	if len(r.records(t, card.MatchRecord)) == 0 {
		t.Error("the recovered venue's cards must be matched")
	}
}

func TestAmd_W8_PruneOnlyOldPriceData(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.w.Discovery(ctx)
	r.w.Prices(ctx)
	matches, routes := len(r.records(t, card.MatchRecord)), len(r.records(t, card.RouteRecord))
	r.now = r.now.Add(25 * time.Hour)
	r.w.Prices(ctx) // fresh prices, kept
	if err := r.w.Retention(ctx); err != nil {
		t.Fatal(err)
	}
	all, _ := r.w.St.Prices.LatestAll(ctx, store.Completed)
	for _, p := range all {
		if !p.PricedAt.Equal(r.now) {
			t.Errorf("old price kept: %+v", p)
		}
	}
	if len(all) == 0 {
		t.Error("fresh prices must be kept")
	}
	if m, _ := r.w.St.Raw.List(ctx, store.RawMarkets); len(m) != 2 {
		t.Errorf("market raw responses must be kept: %d", len(m))
	}
	if p, _ := r.w.St.Raw.List(ctx, store.RawPrices); len(p) != 2 {
		t.Errorf("only the fresh price raw responses must remain: %d", len(p))
	}
	if len(r.records(t, card.MatchRecord)) != matches || len(r.records(t, card.RouteRecord)) < routes {
		t.Error("log records must never be pruned")
	}
	if cards, _ := r.w.St.Cards.LatestAll(ctx, store.Completed); len(cards) != 5 {
		t.Errorf("cards must be kept: %d", len(cards))
	}
}

func TestAmd_W11_RefusesPriceIntervalAtOrAboveAgeLimit(t *testing.T) {
	r := newRig(t)
	for _, every := range []time.Duration{10 * time.Minute, 15 * time.Minute} {
		r.w.Cfg.Watch.PricesEvery = every
		err := Validate(r.w.Cfg)
		if err == nil || !strings.Contains(err.Error(), every.String()) || !strings.Contains(err.Error(), "10m0s") {
			t.Errorf("prices_every %s: want refusal naming both values, got %v", every, err)
		}
		if err := r.w.Run(context.Background()); err == nil {
			t.Error("Run must refuse to start")
		}
	}
	r.w.Cfg.Watch.PricesEvery = 9 * time.Minute
	if err := Validate(r.w.Cfg); err != nil {
		t.Errorf("9m is below the limit: %v", err)
	}
}

func TestWatch_RunHoldsLockAndReleasesOnShutdown(t *testing.T) {
	r := newRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- r.w.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, _, held, _ := r.w.St.Lock.Holder(context.Background()); held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watch never took the run lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	err := r.w.St.Lock.Acquire(context.Background(), "equinox fetch")
	if !errors.Is(err, store.ErrLocked) {
		t.Errorf("a CLI writer must be refused while watch runs: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, _, held, _ := r.w.St.Lock.Holder(context.Background()); held {
		t.Error("shutdown must release the run lock")
	}
}

// Amendment A7: watch and serve are separate processes; SQLite is the only handoff.
func TestWatch_NeverImportsFeed(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		if strings.Contains(string(b), `"equinox/internal/feed`) {
			t.Errorf("%s imports internal/feed", f)
		}
	}
}
