package feed

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"equinox/internal/feed/cache"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

func TestAmd_W5_CacheShowsOnlyCompletedCycles(t *testing.T) {
	f := newFixture(t)
	last := seed(f)
	c, srv := serve(t, f, 25, 2)

	snap := c.Current()
	if snap.Cycle != last || !snap.CompletedAt.Equal(t2) {
		t.Fatalf("snapshot is cycle %d completed %s, want cycle %d completed %s", snap.Cycle, snap.CompletedAt, last, t2)
	}
	for _, cd := range snap.Cards() {
		if cd.MarketID == "KF" || cd.MarketID == "KO" {
			t.Errorf("card %s from a failed or open cycle is in the snapshot", cd.MarketID)
		}
	}

	feed := get(t, srv, "/")
	post := get(t, srv, "/post?cycle="+strconv.FormatInt(last, 10)+"&key=kalshi|Fed+December")
	for _, page := range []string{feed, post} {
		for _, hidden := range []string{"Failed cycle event", "Open cycle event", "9999", "8888", "0.99", "0.01", "from the failed cycle"} {
			if strings.Contains(page, hidden) {
				t.Errorf("page shows %q, which only a failed or open cycle wrote", hidden)
			}
		}
	}
	if !strings.Contains(post, "Yes <b>0.45</b>") {
		t.Errorf("post does not show the completed cycle's yes price 0.45")
	}
}

func TestAmd_W6_LoadMorePagesWithinReadersCycle(t *testing.T) {
	f := newFixture(t)
	last := seed(f)
	c, srv := serve(t, f, 1, 2)
	_, full := serve(t, f, 100, 2)
	want := postKeys(get(t, full, "/?cycle="+strconv.FormatInt(last, 10)))
	if len(want) != 3 {
		t.Fatalf("fixture should give 3 posts in cycle %d, got %v", last, want)
	}

	page := get(t, srv, "/")
	got := postKeys(page)
	if strings.Contains(page, "New data saved at") {
		t.Fatal("banner shown before any newer cycle completed")
	}

	// A newer cycle completes mid-scroll. Its new post would sort first by volume.
	t5 := testkit.AsOf.Add(-10 * time.Minute)
	f.cycle(store.KindFetch, store.StatusCompleted, t5, func(id int64) {
		f.card(id, "kalshi", "KN", "Newer cycle event", "Economics", "100000", 24*time.Hour)
	})
	if swapped, err := c.Refresh(context.Background()); err != nil || !swapped {
		t.Fatalf("refresh after a new completed cycle: swapped=%v err=%v", swapped, err)
	}

	for more := moreLink(page); more != ""; more = moreLink(page) {
		if !strings.Contains(more, "cycle="+strconv.FormatInt(last, 10)) {
			t.Fatalf("Load more link %q does not carry the page's cycle %d", more, last)
		}
		page = get(t, srv, more)
		if !strings.Contains(text(page), "New data saved at "+stamp(t5)+". Reload") {
			t.Errorf("page after the newer cycle lacks the new-data banner")
		}
		got = append(got, postKeys(page)...)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("paging cycle %d gave %v, want %v (no skips, no repeats)", last, got, want)
	}

	// Reload moves to the newest cycle, which has the new post first.
	if keys := postKeys(get(t, srv, "/")); len(keys) == 0 || keys[0] != "kalshi|Newer cycle event" {
		t.Errorf("reload shows %v, want the newer cycle's post first", keys)
	}
}

func TestAmd_W10_ServeKeepsLastCycleWhenWatchStopped(t *testing.T) {
	f := newFixture(t)
	last := seed(f) // cycle 4 is left open: watch crashed mid-cycle
	if err := f.st.Lock.Acquire(context.Background(), "equinox watch (pid 1)"); err != nil {
		t.Fatal(err)
	}
	c, srv := serve(t, f, 25, 2)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, 5*time.Millisecond, func(err error) { t.Error(err) }); close(done) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	if swapped, err := c.Refresh(context.Background()); err != nil || swapped {
		t.Fatalf("refresh with no new cycle: swapped=%v err=%v", swapped, err)
	}
	page := get(t, srv, "/")
	if c.Current().Cycle != last {
		t.Fatalf("cache moved off cycle %d to %d", last, c.Current().Cycle)
	}
	for _, want := range []string{
		"cycle " + strconv.FormatInt(last, 10) + " · completed " + stamp(t2),
		"no cycle has completed since " + stamp(t2) + "; it may have crashed",
		`data-post="polymarket|Big game"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

func TestFeedPost_CrosspostBadgeAndBotReplyVerbatim(t *testing.T) {
	f := newFixture(t)
	last := seed(f)
	_, srv := serve(t, f, 25, 2)

	feed := get(t, srv, "/")
	if !strings.Contains(feed, `<span class="pill x">Same bet on Polymarket</span>`) ||
		!strings.Contains(feed, `<span class="pill x">Same bet on Kalshi</span>`) {
		t.Error("feed lacks the crosspost badge on both sides of the same pair")
	}

	cy := strconv.FormatInt(last, 10)
	post := get(t, srv, "/post?cycle="+cy+"&key=kalshi|Fed+December")
	for _, want := range []string{
		`<span class="lab">Match</span><span><code>same</code> ` + sameReason,
		"<code>buy_on_polymarket</code> " + routeYesReason,
		`<code class="no">no_route</code> ` + routeNoReason,
		`<code class="no">not_same</code> ` + notSameReason,
		`Yes <b>0.45</b></span><span class="px">No <b>0.56</b></span><span class="muted small">saved ` + stamp(t1),
		`<span class="mt">Market KA</span><span class="pill x">Same bet on Polymarket</span>`,
	} {
		if !strings.Contains(post, want) {
			t.Errorf("post lacks %q", want)
		}
	}
	// KB only has a not_same record, so it carries no badge.
	if strings.Contains(post, `Market KB</span><span class="pill`) {
		t.Error("KB shows a crosspost badge but its only match is not_same")
	}

	other := get(t, srv, "/post?cycle="+cy+"&key=polymarket|Big+game")
	if !strings.Contains(other, "No match record for this market.") || !strings.Contains(other, "Yes <b>-</b>") {
		t.Error("a market with no match and no price should say so and show - for prices")
	}
}

func TestFeed_Sorts(t *testing.T) {
	f := newFixture(t)
	seed(f)
	_, srv := serve(t, f, 25, 2)
	for _, tc := range []struct {
		sort string
		want []string
	}{
		{"", []string{"polymarket|Big game", "polymarket|Fed rate December", "kalshi|Fed December"}},
		{"top", []string{"polymarket|Big game", "polymarket|Fed rate December", "kalshi|Fed December"}},
		{"closing", []string{"polymarket|Big game", "kalshi|Fed December", "polymarket|Fed rate December"}},
		{"matched", []string{"polymarket|Fed rate December", "kalshi|Fed December", "polymarket|Big game"}},
	} {
		if got := postKeys(get(t, srv, "/?sort="+tc.sort)); !slices.Equal(got, tc.want) {
			t.Errorf("sort %q: got %v, want %v", tc.sort, got, tc.want)
		}
	}
	// Score is the summed volume_24h of the event's markets.
	if page := get(t, srv, "/"); !strings.Contains(page, "<b>150</b>24h vol") {
		t.Error("Fed December score should be 100 + 50 = 150")
	}
}

func TestFeed_Filters(t *testing.T) {
	f := newFixture(t)
	seed(f)
	_, srv := serve(t, f, 25, 2)
	if got := postKeys(get(t, srv, "/?venue=kalshi")); !slices.Equal(got, []string{"kalshi|Fed December"}) {
		t.Errorf("venue filter: %v", got)
	}
	if got := postKeys(get(t, srv, "/?category=Sports&sort=matched")); !slices.Equal(got, []string{"polymarket|Big game"}) {
		t.Errorf("category filter: %v", got)
	}
	if got := postKeys(get(t, srv, "/?venue=kalshi&category=Sports")); len(got) != 0 {
		t.Errorf("combined filters: %v", got)
	}
}

func TestFeed_WorksWithoutJavaScript(t *testing.T) {
	f := newFixture(t)
	last := seed(f)
	_, srv := serve(t, f, 1, 2)
	pages := []string{get(t, srv, "/"), get(t, srv, "/post?cycle="+strconv.FormatInt(last, 10)+"&key=kalshi|Fed+December")}
	for _, p := range pages {
		if regexp.MustCompile(`(?i)<script|\son[a-z]+=|javascript:`).MatchString(p) {
			t.Error("page uses JavaScript")
		}
	}
	feed := pages[0]
	for _, want := range []string{`<form method="get" action="/"`, `<button type="submit"`, ">Apply</button>", `<select name="venue"`, `<select name="category"`, ">Load more</a>"} {
		if !strings.Contains(feed, want) {
			t.Errorf("feed lacks plain-HTML control %q", want)
		}
	}
}

func TestFeed_NoCompletedCycleYet(t *testing.T) {
	f := newFixture(t)
	f.cycle(store.KindFetch, store.StatusOpen, t1, func(id int64) {
		f.card(id, "kalshi", "KO", "Open cycle event", "Economics", "1", time.Hour)
	})
	_, srv := serve(t, f, 25, 2)
	if page := get(t, srv, "/"); !strings.Contains(page, "No cycle has completed yet") || strings.Contains(page, "Open cycle event") {
		t.Error("with no completed cycle the feed should say so and show nothing")
	}
}

func TestFeed_EvictedCycleStartsOver(t *testing.T) {
	f := newFixture(t)
	first := seed(f)
	c, srv := serve(t, f, 1, 2)
	for i, at := range []time.Time{testkit.AsOf.Add(-9 * time.Minute), testkit.AsOf.Add(-8 * time.Minute)} {
		f.cycle(store.KindPrices, store.StatusCompleted, at, func(int64) {})
		if _, err := c.Refresh(context.Background()); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, held := c.At(first); held {
		t.Fatalf("cycle %d should be evicted with keep_snapshots 2", first)
	}
	page := get(t, srv, "/?cycle="+strconv.FormatInt(first, 10)+"&cursor=2")
	if !strings.Contains(page, "no longer held") || len(postKeys(page)) != 1 || postKeys(page)[0] != "polymarket|Big game" {
		t.Errorf("an evicted cycle should restart on the newest cycle from the first post")
	}
}

// writeCall matches a call to any store method that writes.
var writeCall = regexp.MustCompile(`\.(Append|Save|Begin|Complete|Fail|Acquire|Release|Prune|Clear|Finish|Tx)\(`)

func TestFeed_NoWriteMethodsReachable(t *testing.T) {
	// The only door to SQLite is cache.Reader, and it lists read methods only.
	var methods []string
	rt := reflect.TypeFor[cache.Reader]()
	for i := range rt.NumMethod() {
		methods = append(methods, rt.Method(i).Name)
	}
	if want := []string{"Cards", "LatestCompleted", "Matches", "Prices", "Routes", "Status"}; !slices.Equal(methods, want) {
		t.Errorf("cache.Reader methods %v, want exactly the read methods %v", methods, want)
	}

	// No non-test file under internal/feed calls a write method, imports
	// internal/watch, or touches the run lock.
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if writeCall.MatchString(line) {
				t.Errorf("%s:%d calls a write method: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			if strings.Contains(imp.Path.Value, "internal/watch") {
				t.Errorf("%s imports internal/watch", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
