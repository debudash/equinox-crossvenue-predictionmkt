package registry

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"equinox/internal/card"
	"equinox/internal/fetch/kalshi"
	"equinox/internal/fetch/polymarket"
	"equinox/internal/store"
)

// Fetcher is what every venue fetcher provides.
type Fetcher interface {
	Name() string
	Fetch(ctx context.Context, st *store.Store, cycleID int64) (store.FetchRun, error)
	FetchPrices(ctx context.Context, st *store.Store, cycleID int64, marketIDs []string) (store.FetchRun, error)
}

// Normalizer turns one saved markets raw response into cards. It is the same
// code path fetch uses, so rebuild reproduces fetch exactly.
type Normalizer func(raw store.Raw, vocab *card.Vocab) ([]card.Card, []card.PriceRecord, []store.Skip)

var normalizers = map[string]Normalizer{
	kalshi.Venue:     kalshi.NormalizeMarkets,
	polymarket.Venue: polymarket.NormalizeMarkets,
}

// All returns one fetcher per allowed venue, in config order.
func All(cfg *card.Config, vocab *card.Vocab, client *http.Client, clock func() time.Time) ([]Fetcher, error) {
	var out []Fetcher
	for _, venue := range cfg.Venues.Allowed {
		fc := cfg.Fetchers[venue]
		switch venue {
		case kalshi.Venue:
			out = append(out, kalshi.New(fc, vocab, client, clock))
		case polymarket.Venue:
			out = append(out, polymarket.New(fc, vocab, client, clock))
		default:
			return nil, fmt.Errorf("no fetcher for venue %q", venue)
		}
	}
	return out, nil
}

// NormalizerFor returns the markets normalizer for a venue.
func NormalizerFor(venue string) (Normalizer, bool) {
	n, ok := normalizers[venue]
	return n, ok
}

// Result is one fetcher's outcome. Err is a store error; a venue failure is
// recorded in Run (status failed, error text) and is not an Err.
type Result struct {
	Venue string
	Run   store.FetchRun
	Err   error
}

// FetchAll runs every fetcher concurrently, each with its own timeout. One
// failing never stops the others. Results are in venue order.
func FetchAll(ctx context.Context, st *store.Store, cycleID int64, fs []Fetcher, timeout time.Duration) []Result {
	out := make([]Result, len(fs))
	var wg sync.WaitGroup
	for i, f := range fs {
		wg.Add(1)
		go func(i int, f Fetcher) {
			defer wg.Done()
			fctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			run, err := f.Fetch(fctx, st, cycleID)
			out[i] = Result{Venue: f.Name(), Run: run, Err: err}
		}(i, f)
	}
	wg.Wait()
	sort.SliceStable(out, func(i, j int) bool { return out[i].Venue < out[j].Venue })
	return out
}

// RebuildSummary counts what a rebuild did.
type RebuildSummary struct {
	Raws, Versions, Skipped int
}

// Rebuild clears cards and re-normalizes every saved markets raw response in
// id order through the fetchers' own normalizers, in one transaction, without
// any network access. Each card is saved in the cycle of the raw response it
// came from, so the rows are identical to those fetch wrote (test 18).
// Skips are logged in rebuildCycle.
func Rebuild(ctx context.Context, st *store.Store, rebuildCycle int64, vocab *card.Vocab) (RebuildSummary, error) {
	var sum RebuildSummary
	raws, err := st.Raw.List(ctx, store.RawMarkets)
	if err != nil {
		return sum, err
	}
	err = st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.Cards.Clear(ctx); err != nil {
			return err
		}
		for _, raw := range raws {
			sum.Raws++
			norm, ok := NormalizerFor(raw.Venue)
			if !ok {
				sum.Skipped++
				if err := tx.Skips.Append(ctx, store.Skip{CycleID: rebuildCycle, Stage: store.StageRebuild, Venue: raw.Venue,
					Reason: fmt.Sprintf("raw response %d: no normalizer for venue %q", raw.ID, raw.Venue)}); err != nil {
					return err
				}
				continue
			}
			cards, _, skips := norm(raw, vocab)
			for _, c := range cards {
				r, err := tx.Cards.Save(ctx, raw.CycleID, c)
				if err != nil {
					return err
				}
				if r.Changed {
					sum.Versions++
				}
			}
			for _, k := range skips {
				sum.Skipped++
				k.CycleID, k.RunID, k.Stage = rebuildCycle, 0, store.StageRebuild
				if err := tx.Skips.Append(ctx, k); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return sum, err
}
