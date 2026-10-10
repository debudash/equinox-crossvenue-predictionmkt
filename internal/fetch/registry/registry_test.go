package registry

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/fetch/kalshi"
	"equinox/internal/fetch/polymarket"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

func loadConfig(t *testing.T) (*card.Config, *card.Vocab) {
	t.Helper()
	root := testkit.RepoRoot(t)
	cfg, err := card.LoadConfig(filepath.Join(root, "config", "equinox.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := card.LoadVocab(filepath.Join(root, "config", "vocab.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, v
}

// fetchFixtures runs both real fetchers against the recorded Fed fixtures.
func fetchFixtures(t *testing.T, st *store.Store, cfg *card.Config, v *card.Vocab) {
	t.Helper()
	ctx := context.Background()
	clock := testkit.FixedClock(testkit.AsOf.Add(-2 * time.Minute))
	fs := []Fetcher{
		kalshi.New(cfg.Fetchers[kalshi.Venue], v, testkit.Scenario(t, testkit.Fixture(t, "kalshi", "fed")).Client(), clock),
		polymarket.New(cfg.Fetchers[polymarket.Venue], v, testkit.Scenario(t, testkit.Fixture(t, "polymarket", "fed")).Client(), clock),
	}
	cyc, _ := st.Cycles.Begin(ctx, store.KindFetch, store.TriggerCLI)
	for _, r := range FetchAll(ctx, st, cyc, fs, time.Minute) {
		if r.Err != nil || r.Run.Status != store.RunOK {
			t.Fatalf("%s: %+v %v", r.Venue, r.Run, r.Err)
		}
	}
	st.Cycles.Complete(ctx, cyc)
}

func versionsText(t *testing.T, st *store.Store) []string {
	t.Helper()
	vs, err := st.Cards.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Each card's versions, in version order, with every field and its cycle.
	// The row id is a surrogate key: two fetchers run concurrently, so ids
	// interleave across venues differently from one run to the next.
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = fmt.Sprintf("%s|%d|%v|%+v", v.Card.Ref(), v.CycleID, v.Card.Payoff.Fields(), v.Card.Record)
	}
	sort.SliceStable(out, func(i, j int) bool { return refOf(out[i]) < refOf(out[j]) })
	return out
}

func refOf(row string) string { r, _, _ := strings.Cut(row, "|"); return r }

func TestReq18_RebuildReproducesIdenticalCards(t *testing.T) {
	ctx := context.Background()
	cfg, v := loadConfig(t)
	st := testkit.TempStore(t)
	fetchFixtures(t, st, cfg, v)
	before := versionsText(t, st)
	if len(before) == 0 {
		t.Fatal("fixtures produced no cards")
	}

	// Rebuild takes no HTTP client: it can only read saved raw responses.
	cyc, _ := st.Cycles.Begin(ctx, store.KindRebuild, store.TriggerCLI)
	sum, err := Rebuild(ctx, st, cyc, v)
	if err != nil {
		t.Fatal(err)
	}
	st.Cycles.Complete(ctx, cyc)
	after := versionsText(t, st)
	if !reflect.DeepEqual(before, after) {
		for i := range before {
			if before[i] != after[i] {
				t.Fatalf("row %d differs:\n%s\n%s", i, before[i], after[i])
			}
		}
		t.Fatalf("rebuild changed cards: %d before, %d after", len(before), len(after))
	}
	if sum.Versions != len(before) {
		t.Errorf("versions %d of %d", sum.Versions, len(before))
	}
}

// failing is a transport whose venue is unreachable.
type failing struct{}

func (failing) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("venue down")
}

func TestFetchAll_OneVenueFailingKeepsTheOther(t *testing.T) {
	ctx := context.Background()
	cfg, v := loadConfig(t)
	st := testkit.TempStore(t)
	clock := testkit.FixedClock(testkit.AsOf)
	fs := []Fetcher{
		kalshi.New(cfg.Fetchers[kalshi.Venue], v, &http.Client{Transport: failing{}}, clock),
		polymarket.New(cfg.Fetchers[polymarket.Venue], v, testkit.Scenario(t, testkit.Fixture(t, "polymarket", "fed")).Client(), clock),
	}
	cyc, _ := st.Cycles.Begin(ctx, store.KindFetch, store.TriggerCLI)
	res := FetchAll(ctx, st, cyc, fs, time.Minute)
	st.Cycles.Complete(ctx, cyc)
	if res[0].Venue != kalshi.Venue || res[0].Run.Status != store.RunFailed || res[0].Err != nil {
		t.Errorf("failed venue must be recorded, not returned: %+v", res[0])
	}
	if res[1].Run.Status != store.RunOK {
		t.Errorf("other venue must complete: %+v", res[1])
	}
	cards, _ := st.Cards.LatestAll(ctx, store.Completed)
	if len(cards) == 0 {
		t.Error("other venue's cards must be saved")
	}
}
