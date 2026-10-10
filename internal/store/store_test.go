package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
)

var (
	ctx = context.Background()
	t0  = time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	s.Clock = func() time.Time { return t0 }
	t.Cleanup(func() { s.Close() })
	return s
}

func begin(t *testing.T, s *Store, kind string) int64 {
	t.Helper()
	id, err := s.Cycles.Begin(ctx, kind, TriggerCLI)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func complete(t *testing.T, s *Store, id int64) {
	t.Helper()
	if err := s.Cycles.Complete(ctx, id); err != nil {
		t.Fatal(err)
	}
}

func fedCard(venue, id string) card.Card {
	v, _ := card.ParseDecimal("3.50")
	d, _ := card.ParseInstant("2026-12-10T19:00:00Z")
	return card.Card{
		Venue: venue, MarketID: id,
		Payoff: card.Payoff{
			Subject: card.Some("fed_funds_upper_bound"), CutoffOp: card.Some(card.Gt), CutoffValue: card.Some(v),
			CutoffUnit: card.Some("percent"), Deadline: card.Some(d), Decider: card.Some("federal_reserve"),
			UnclearResult: card.Some(card.YesOrNoOnly),
		},
		Record: card.RecordFields{Title: "Fed above 3.5%", RulesText: "rules", RawRef: "1", FetchedAt: t0,
			EventTitle: "Fed December", Category: "Economics", Volume24h: "1200", URL: "https://example.test/m"},
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e.db")
	for i := 0; i < 2; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		var v int
		if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v != len(migrations) {
			t.Fatalf("user_version = %d, %v", v, err)
		}
		s.Close()
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaHasArchitectureTables(t *testing.T) {
	s := newStore(t)
	for _, table := range []string{"cycles", "run_lock", "fetch_runs", "raw_responses", "skips", "cards", "prices", "decision_log", "subject_counts"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s missing", table)
		}
	}
}

func TestCardRoundTripAndEmptyIsNull(t *testing.T) {
	s := newStore(t)
	c := begin(t, s, KindFetch)
	in := fedCard("a", "M1")
	in.Payoff.Deadline = card.None[card.Instant]()
	if _, err := s.Cards.Save(ctx, c, in); err != nil {
		t.Fatal(err)
	}
	complete(t, s, c)
	var deadlineNull bool
	if err := s.db.QueryRow(`SELECT deadline IS NULL FROM cards`).Scan(&deadlineNull); err != nil || !deadlineNull {
		t.Fatalf("empty deadline must be stored as NULL: %v", err)
	}
	out, ok, err := s.Cards.Latest(ctx, in.Ref(), Completed)
	if err != nil || !ok {
		t.Fatalf("card not found: %v", err)
	}
	if !reflect.DeepEqual(out.Payoff.Fields(), in.Payoff.Fields()) || !reflect.DeepEqual(out.Record, in.Record) {
		t.Errorf("round trip differs:\n in %+v\nout %+v", in, out)
	}
	if !out.Payoff.Deadline.IsEmpty() {
		t.Error("deadline must read back empty")
	}
}

func TestCardVersioning(t *testing.T) {
	s := newStore(t)
	c1 := begin(t, s, KindFetch)
	c := fedCard("a", "M1")
	if r, _ := s.Cards.Save(ctx, c1, c); !r.New || !r.Changed || !r.PayoffChanged {
		t.Errorf("first save: %+v", r)
	}
	again := c
	again.Record.RawRef, again.Record.FetchedAt = "2", t0.Add(time.Minute)
	if r, _ := s.Cards.Save(ctx, c1, again); r.Changed {
		t.Errorf("only raw_ref and fetched_at changed, want no new version: %+v", r)
	}
	vol := c
	vol.Record.Volume24h = "9999"
	if r, _ := s.Cards.Save(ctx, c1, vol); !r.Changed || r.New || r.PayoffChanged {
		t.Errorf("display change: %+v", r)
	}
	pay := vol
	pay.Payoff.CutoffOp = card.Some(card.Gte)
	if r, _ := s.Cards.Save(ctx, c1, pay); !r.Changed || !r.PayoffChanged {
		t.Errorf("payoff change: %+v", r)
	}
	vs, _ := s.Cards.Versions(ctx)
	if len(vs) != 3 {
		t.Errorf("want 3 versions, got %d", len(vs))
	}
}

func TestOnlyCompletedCyclesAreVisible(t *testing.T) {
	s := newStore(t)
	c1 := begin(t, s, KindFetch)
	s.Cards.Save(ctx, c1, fedCard("a", "M1"))
	if cs, _ := s.Cards.LatestAll(ctx, Completed); len(cs) != 0 {
		t.Fatal("open cycle must be invisible to readers")
	}
	if cs, _ := s.Cards.LatestAll(ctx, Writing(c1)); len(cs) != 1 {
		t.Fatal("open cycle must be visible to its own writer")
	}
	complete(t, s, c1)

	c2 := begin(t, s, KindFetch)
	changed := fedCard("a", "M1")
	changed.Record.Title = "changed in a failed cycle"
	s.Cards.Save(ctx, c2, changed)
	s.Cards.Save(ctx, c2, fedCard("a", "M2"))
	if err := s.Cycles.Fail(ctx, c2); err != nil {
		t.Fatal(err)
	}
	cs, _ := s.Cards.LatestAll(ctx, Completed)
	if len(cs) != 1 || cs[0].Record.Title != "Fed above 3.5%" {
		t.Fatalf("failed cycle leaked: %+v", cs)
	}

	// A later cycle with the same change must write it again, since the failed version is invisible.
	c3 := begin(t, s, KindFetch)
	if r, _ := s.Cards.Save(ctx, c3, changed); !r.Changed {
		t.Error("change from a failed cycle must be re-saved")
	}
	complete(t, s, c3)
	if c, _, _ := s.Cards.Latest(ctx, changed.Ref(), Scope{UpTo: c1}); c.Record.Title != "Fed above 3.5%" {
		t.Errorf("UpTo must pin a reader to an older cycle, got %q", c.Record.Title)
	}
	last, ok, _ := s.Cycles.LatestCompleted(ctx)
	if !ok || last.ID != c3 || last.Status != StatusCompleted || !last.CompletedAt.Equal(t0) {
		t.Errorf("latest completed: %+v", last)
	}
}

func TestClearAndResaveReproducesRows(t *testing.T) {
	s := newStore(t)
	c := begin(t, s, KindFetch)
	cards := []card.Card{fedCard("a", "M1"), fedCard("b", "P1")}
	for _, x := range cards {
		s.Cards.Save(ctx, c, x)
	}
	complete(t, s, c)
	before, _ := s.Cards.Versions(ctx)
	err := s.Tx(ctx, func(tx *Store) error {
		if err := tx.Cards.Clear(ctx); err != nil {
			return err
		}
		for _, x := range cards {
			if _, err := tx.Cards.Save(ctx, c, x); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := s.Cards.Versions(ctx)
	if !reflect.DeepEqual(versionText(before), versionText(after)) {
		t.Errorf("rows differ after clear and resave:\n%v\n%v", versionText(before), versionText(after))
	}
}

func versionText(vs []Version) []string {
	var out []string
	for _, v := range vs {
		var b strings.Builder
		for _, f := range v.Card.Payoff.Fields() {
			b.WriteString(f.Value + "|")
		}
		out = append(out, fmt.Sprintf("%s%s|%s|%d|%d", b.String(), v.Card.Ref(), v.Card.Record.Title, v.ID, v.CycleID))
	}
	return out
}

func TestPricesLatestAtAndPrune(t *testing.T) {
	s := newStore(t)
	c := begin(t, s, KindPrices)
	ref := card.MarketRef{Venue: "a", MarketID: "M1"}
	put := func(at time.Time, price string) {
		m, _ := card.ParseMicros(price)
		if err := s.Prices.Append(ctx, c, card.PriceRecord{Venue: "a", MarketID: "M1", Side: card.Yes, BuyPrice: card.Some(m), PricedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	put(t0, "0.40")
	put(t0.Add(5*time.Minute), "0.45")
	put(t0.Add(20*time.Minute), "0.50")
	s.Prices.Append(ctx, c, card.PriceRecord{Venue: "a", MarketID: "M1", Side: card.No, PricedAt: t0})
	complete(t, s, c)

	p, ok, err := s.Prices.LatestAt(ctx, ref, card.Yes, t0.Add(10*time.Minute), Completed)
	if err != nil || !ok || !p.BuyPrice.Equal(card.Some(card.Micros(450_000))) || !p.PricedAt.Equal(t0.Add(5*time.Minute)) {
		t.Fatalf("LatestAt must ignore prices after as_of: %+v %v", p, err)
	}
	if _, ok, _ := s.Prices.LatestAt(ctx, ref, card.Yes, t0.Add(-time.Second), Completed); ok {
		t.Error("no price before the first one")
	}
	no, _, _ := s.Prices.LatestAt(ctx, ref, card.No, t0, Completed)
	if !no.BuyPrice.IsEmpty() || !no.Fee.IsEmpty() {
		t.Error("empty buy price and fee must read back empty")
	}
	all, _ := s.Prices.LatestAll(ctx, Completed)
	if len(all) != 2 || !all[1].BuyPrice.Equal(card.Some(card.Micros(500_000))) {
		t.Errorf("LatestAll: %+v", all)
	}

	s.Raw.Save(ctx, Raw{CycleID: c, Venue: "a", Kind: RawPrices, Endpoint: "/p", FetchedAt: t0, Body: []byte("{}")})
	s.Raw.Save(ctx, Raw{CycleID: c, Venue: "a", Kind: RawMarkets, Endpoint: "/m", FetchedAt: t0, Body: []byte("{}")})
	np, nr, err := s.Prices.Prune(ctx, t0.Add(time.Minute))
	if err != nil || np != 2 || nr != 1 {
		t.Fatalf("prune removed %d prices, %d raws: %v", np, nr, err)
	}
	if m, _ := s.Raw.List(ctx, RawMarkets); len(m) != 1 {
		t.Error("market raw responses must be kept")
	}
}

func TestDecisionLogRoundTrip(t *testing.T) {
	s := newStore(t)
	c := begin(t, s, KindRoute)
	a, b := card.MarketRef{Venue: "a", MarketID: "K1"}, card.MarketRef{Venue: "b", MarketID: "P1"}
	match := card.LogRecord{Type: card.MatchRecord, MarketA: a, MarketB: b, Match: card.Same, Reason: "All payoff fields are equal."}
	route := card.LogRecord{Type: card.RouteRecord, AsOf: t0, MarketA: a, MarketB: b, Match: card.Same,
		SideA: card.Yes, SideB: card.Yes, PriceA: card.Some(card.Micros(400_000)), PricedAtA: t0.Add(-time.Minute),
		FeeApplied: false, Action: card.NoRoute, Reason: "The b yes price is missing."}
	if _, err := s.Log.Append(ctx, c, match); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Log.Append(ctx, c, route); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Log.Append(ctx, c, card.LogRecord{Type: card.MatchRecord, MarketA: a, MarketB: b, Match: card.Same}); err == nil {
		t.Error("a record without a reason must be rejected")
	}
	complete(t, s, c)

	recs, err := s.Log.List(ctx, Completed)
	if err != nil || len(recs) != 2 {
		t.Fatalf("list: %v %v", recs, err)
	}
	got := recs[1]
	if got.CycleID != c || !got.DecidedAt.Equal(t0) || !got.AsOf.Equal(t0) || got.MarketB != b ||
		!got.PriceA.Equal(route.PriceA) || !got.PriceB.IsEmpty() || !got.PricedAtB.IsZero() ||
		got.Action != card.NoRoute || got.Reason != route.Reason || got.FeeApplied {
		t.Errorf("route record round trip: %+v", got)
	}
	var fee *string
	s.db.QueryRow(`SELECT fee_applied FROM decision_log WHERE record_type = 'match'`).Scan(&fee)
	if fee != nil {
		t.Error("fee_applied must be empty on match records")
	}
	if m, _ := s.Log.LatestMatches(ctx, Completed); len(m) != 1 || m[0].Match != card.Same {
		t.Errorf("LatestMatches: %+v", m)
	}
	if r, _ := s.Log.LatestRoutes(ctx, Completed); len(r) != 1 || r[0].SideA != card.Yes {
		t.Errorf("LatestRoutes: %+v", r)
	}
	if f, _ := s.Log.ForMarket(ctx, b, Completed); len(f) != 2 {
		t.Errorf("ForMarket: %d", len(f))
	}
}

// AGENTS.md hard rule 7: the decision log has no update or delete path.
func TestDecisionLogIsAppendOnly(t *testing.T) {
	typ := reflect.TypeOf(&DecisionLog{})
	allowed := map[string]bool{"Append": true, "List": true, "LatestMatches": true, "LatestRoutes": true, "ForMarket": true}
	for i := 0; i < typ.NumMethod(); i++ {
		if name := typ.Method(i).Name; !allowed[name] {
			t.Errorf("DecisionLog exposes %s; only Append and read methods are allowed", name)
		}
	}
	src, err := os.ReadFile("log.go")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?i)\b(UPDATE|DELETE)\b\s+(FROM\s+)?decision_log`).Match(src) {
		t.Error("log.go contains an UPDATE or DELETE on decision_log")
	}

	s := newStore(t)
	c := begin(t, s, KindMatch)
	a, b := card.MarketRef{Venue: "a", MarketID: "K1"}, card.MarketRef{Venue: "b", MarketID: "P1"}
	s.Log.Append(ctx, c, card.LogRecord{Type: card.MatchRecord, MarketA: a, MarketB: b, Match: card.NotSame, Reason: "decider differs"})
	s.Skips.Append(ctx, Skip{CycleID: c, Stage: StageMatch, Reason: "x"})
	for _, q := range []string{
		`UPDATE decision_log SET reason = 'edited'`, `DELETE FROM decision_log`,
		`UPDATE skips SET reason = 'edited'`, `DELETE FROM skips`,
	} {
		if _, err := s.db.Exec(q); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s must be rejected, got %v", q, err)
		}
	}
}

func TestRunLock(t *testing.T) {
	s := newStore(t)
	if err := s.Lock.Acquire(ctx, "watch pid 41"); err != nil {
		t.Fatal(err)
	}
	err := s.Lock.Acquire(ctx, "cli fetch")
	var le *LockedError
	if !errors.As(err, &le) || !errors.Is(err, ErrLocked) || le.Holder != "watch pid 41" || !strings.Contains(err.Error(), "watch pid 41") {
		t.Fatalf("second writer must be refused naming the holder, got %v", err)
	}
	if err := s.Lock.Release(ctx, "cli fetch"); err == nil {
		t.Error("a non-holder must not release the lock")
	}
	if err := s.Lock.Release(ctx, "watch pid 41"); err != nil {
		t.Fatal(err)
	}
	if _, _, held, _ := s.Lock.Holder(ctx); held {
		t.Error("lock must be free after release")
	}
	if err := s.Lock.Acquire(ctx, "cli fetch"); err != nil {
		t.Errorf("free lock must be acquirable: %v", err)
	}
}

func TestRunsSkipsRawAndCounts(t *testing.T) {
	s := newStore(t)
	c := begin(t, s, KindFetch)
	run, err := s.Runs.Start(ctx, c, "a")
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Raw.Save(ctx, Raw{CycleID: c, RunID: run, Venue: "a", Kind: RawMarkets, Endpoint: "/markets?status=open", FetchedAt: t0, Body: []byte(`{"markets":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Skips.Append(ctx, Skip{CycleID: c, RunID: run, Stage: StageFetch, Venue: "a", MarketID: "COMBO-1", Reason: "multivariate combo market"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Runs.Finish(ctx, run, RunFailed, 3, 1, "HTTP 500"); err != nil {
		t.Fatal(err)
	}
	if err := s.Runs.Finish(ctx, run, RunOK, 0, 0, ""); err == nil {
		t.Error("a run finishes once")
	}
	if err := s.Subjects.Append(ctx, c, []SubjectCount{{Subject: "fed_funds_upper_bound", Venue: "a", Count: 3}, {Subject: "", Venue: "a", Count: 1}}); err != nil {
		t.Fatal(err)
	}

	runs, _ := s.Runs.ForCycle(ctx, c)
	if len(runs) != 1 || runs[0].Status != RunFailed || runs[0].Saved != 3 || runs[0].Skipped != 1 || runs[0].Error != "HTTP 500" || !runs[0].FinishedAt.Equal(t0) {
		t.Errorf("runs: %+v", runs)
	}
	skips, _ := s.Skips.ForCycle(ctx, c)
	if len(skips) != 1 || skips[0].Reason != "multivariate combo market" || skips[0].RunID != run {
		t.Errorf("skips: %+v", skips)
	}
	raws, _ := s.Raw.List(ctx, RawMarkets)
	sum := sha256.Sum256([]byte(`{"markets":[]}`))
	if len(raws) != 1 || raws[0].ID != id || string(raws[0].Body) != `{"markets":[]}` || raws[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("raw: %+v", raws)
	}
	counts, _ := s.Subjects.ForCycle(ctx, c)
	if len(counts) != 2 || counts[0].Subject != "" || counts[1].Count != 3 {
		t.Errorf("counts: %+v", counts)
	}
	if _, err := s.Raw.Save(ctx, Raw{CycleID: c, Venue: "a", Kind: "orders", Body: nil}); err == nil {
		t.Error("unknown raw kind must be rejected")
	}
}
