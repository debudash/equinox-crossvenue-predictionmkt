package feed

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/feed/cache"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

// Times used by the fixture. Each completed cycle gets its own completion time
// so the page can show which one it reads.
var (
	t1 = testkit.AsOf.Add(-50 * time.Minute)
	t2 = testkit.AsOf.Add(-40 * time.Minute)
	t3 = testkit.AsOf.Add(-30 * time.Minute)
	t4 = testkit.AsOf.Add(-20 * time.Minute)
)

// Verbatim log text the bot reply must show.
const (
	sameReason     = "All payoff fields are equal."
	notSameReason  = "deadline differs: 2026-11-04T00:00:00Z vs 2026-12-10T19:00:00Z"
	routeYesReason = "counter venue yes costs 0.41 and reference venue yes costs 0.45; fees applied on both venues"
	routeNoReason  = "counter venue no price is stale: saved more than 10 minutes before as_of"
)

type fixture struct {
	t  *testing.T
	st *store.Store
}

func newFixture(t *testing.T) *fixture { return &fixture{t: t, st: testkit.TempStore(t)} }

// cycle runs fn inside a new cycle and then finishes it as status at time at.
// status open leaves it open, as a crashed writer would.
func (f *fixture) cycle(kind, status string, at time.Time, fn func(id int64)) int64 {
	f.t.Helper()
	ctx := context.Background()
	f.st.Clock = testkit.FixedClock(at.Add(-time.Minute))
	id, err := f.st.Cycles.Begin(ctx, kind, store.TriggerWatch)
	if err != nil {
		f.t.Fatal(err)
	}
	fn(id)
	f.st.Clock = testkit.FixedClock(at)
	switch status {
	case store.StatusCompleted:
		err = f.st.Cycles.Complete(ctx, id)
	case store.StatusFailed:
		err = f.st.Cycles.Fail(ctx, id)
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fixture) card(id int64, venue, market, event, category, volume string, closesIn time.Duration) {
	f.t.Helper()
	c := card.Card{Venue: venue, MarketID: market}
	c.Payoff.Subject = card.Some("fed_funds_upper_bound")
	c.Payoff.Deadline = card.Some(card.InstantOf(testkit.AsOf.Add(closesIn)))
	c.Record = card.RecordFields{Title: "Market " + market, EventTitle: event, Category: category,
		Volume24h: volume, FetchedAt: testkit.AsOf, URL: "https://example.test/" + market}
	if _, err := f.st.Cards.Save(context.Background(), id, c); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) price(id int64, venue, market string, side card.Side, buy string, at time.Time) {
	f.t.Helper()
	m, err := card.ParseMicros(buy)
	if err != nil {
		f.t.Fatal(err)
	}
	p := card.PriceRecord{Venue: venue, MarketID: market, Side: side, BuyPrice: card.Some(m), PricedAt: at}
	if err := f.st.Prices.Append(context.Background(), id, p); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) log(id int64, rec card.LogRecord) {
	f.t.Helper()
	if _, err := f.st.Log.Append(context.Background(), id, rec); err != nil {
		f.t.Fatal(err)
	}
}

func ref(v, m string) card.MarketRef { return card.MarketRef{Venue: v, MarketID: m} }

// seed writes the standard scenario and returns the id of the last completed
// cycle (2):
//
//	cycle 1 fetch, completed: four cards in three events, prices for KA and PX
//	cycle 2 match and route, completed: KA same PX, KB not_same PX, two routes
//	cycle 3 fetch, failed midway: a new event, a new price, a same match for KB
//	cycle 4 fetch, open: another new event
func seed(f *fixture) int64 {
	f.cycle(store.KindFetch, store.StatusCompleted, t1, func(id int64) {
		f.card(id, "kalshi", "KA", "Fed December", "Economics", "100", 30*24*time.Hour)
		f.card(id, "kalshi", "KB", "Fed December", "Economics", "50", 5*24*time.Hour)
		f.card(id, "polymarket", "PX", "Fed rate December", "Economics", "500", 40*24*time.Hour)
		f.card(id, "polymarket", "PY", "Big game", "Sports", "1000", 2*24*time.Hour)
		f.price(id, "kalshi", "KA", card.Yes, "0.45", t1)
		f.price(id, "kalshi", "KA", card.No, "0.56", t1)
		f.price(id, "polymarket", "PX", card.Yes, "0.41", t1)
		f.price(id, "polymarket", "PX", card.No, "0.6", t1)
	})
	last := f.cycle(store.KindMatch, store.StatusCompleted, t2, func(id int64) {
		f.log(id, card.LogRecord{Type: card.MatchRecord, MarketA: ref("kalshi", "KA"), MarketB: ref("polymarket", "PX"), Match: card.Same, Reason: sameReason})
		f.log(id, card.LogRecord{Type: card.MatchRecord, MarketA: ref("kalshi", "KB"), MarketB: ref("polymarket", "PX"), Match: card.NotSame, Reason: notSameReason})
		f.log(id, card.LogRecord{Type: card.RouteRecord, AsOf: t2, MarketA: ref("kalshi", "KA"), MarketB: ref("polymarket", "PX"), Match: card.Same,
			SideA: card.Yes, SideB: card.Yes, Action: card.BuyOn("polymarket"), Reason: routeYesReason})
		f.log(id, card.LogRecord{Type: card.RouteRecord, AsOf: t2, MarketA: ref("kalshi", "KA"), MarketB: ref("polymarket", "PX"), Match: card.Same,
			SideA: card.No, SideB: card.No, Action: card.NoRoute, Reason: routeNoReason})
	})
	f.cycle(store.KindFetch, store.StatusFailed, t3, func(id int64) {
		f.card(id, "kalshi", "KF", "Failed cycle event", "Economics", "9999", time.Hour)
		f.price(id, "kalshi", "KA", card.Yes, "0.99", t3)
		f.log(id, card.LogRecord{Type: card.MatchRecord, MarketA: ref("kalshi", "KB"), MarketB: ref("polymarket", "PY"), Match: card.Same, Reason: "from the failed cycle"})
	})
	f.cycle(store.KindFetch, store.StatusOpen, t4, func(id int64) {
		f.card(id, "kalshi", "KO", "Open cycle event", "Economics", "8888", time.Hour)
		f.price(id, "polymarket", "PX", card.Yes, "0.01", t4)
	})
	return last
}

// serve builds a cache over the fixture's store, refreshes it once and
// returns a test server for the feed.
func serve(t *testing.T, f *fixture, pageSize, keep int) (*cache.Cache, *httptest.Server) {
	t.Helper()
	c := cache.New(cache.FromStore(f.st), keep)
	if _, err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(c, pageSize).Handler())
	t.Cleanup(srv.Close)
	return c, srv
}

// get fetches a path and returns the body with HTML entities decoded, so log
// text can be compared verbatim.
func get(t *testing.T, srv *httptest.Server, path string) string {
	t.Helper()
	res, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, res.StatusCode, b)
	}
	return html.UnescapeString(string(b))
}

var (
	postRE = regexp.MustCompile(`data-post="([^"]*)"`)
	moreRE = regexp.MustCompile(`<a href="([^"]*)" class="card"[^>]*>Load more</a>`)
)

// postKeys returns the post keys on a page, in order.
func postKeys(body string) []string {
	var out []string
	for _, m := range postRE.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

// text strips tags, leaving what a reader sees.
func text(body string) string { return tagRE.ReplaceAllString(body, "") }

var tagRE = regexp.MustCompile(`<[^>]*>`)

// moreLink returns the Load more link, or "".
func moreLink(body string) string {
	if m := moreRE.FindStringSubmatch(body); m != nil {
		return m[1]
	}
	return ""
}
