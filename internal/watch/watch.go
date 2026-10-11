package watch

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"equinox/internal/card"
	"equinox/internal/fetch/registry"
	"equinox/internal/match"
	"equinox/internal/route"
	"equinox/internal/store"
)

// Validate refuses a configuration whose price loop could let matched prices
// go stale (amendment A8, test W11).
func Validate(cfg *card.Config) error {
	w, maxAge := cfg.Watch, cfg.Router.PriceMaxAge
	if w.PricesEvery <= 0 || w.DiscoveryEvery <= 0 || w.Retention <= 0 {
		return fmt.Errorf("watch: discovery_every, prices_every and retention must all be positive")
	}
	if w.PricesEvery >= maxAge {
		return fmt.Errorf("watch: refusing to start: watch.prices_every (%s) must be below router.price_max_age (%s), or matched prices go stale between polls",
			w.PricesEvery, maxAge)
	}
	return nil
}

// Watcher runs the discovery loop, the price loop and retention in one
// process. Passes never overlap, so SQLite has one writer at a time.
type Watcher struct {
	St       *store.Store
	Cfg      *card.Config
	Fetchers []registry.Fetcher
	// Clock drives the scheduler only: refresh times, as_of and retention
	// cutoffs. The router still receives as_of and never reads it.
	Clock   func() time.Time
	Log     io.Writer
	Timeout time.Duration // per fetcher, per pass
}

func (w *Watcher) logf(format string, a ...any) {
	if w.Log != nil {
		fmt.Fprintf(w.Log, "%s watch: %s\n", w.Clock().UTC().Format(time.RFC3339), fmt.Sprintf(format, a...))
	}
}

// pass runs fn as one numbered cycle. A store error fails the cycle, which
// stays invisible to readers; the next pass runs normally.
func (w *Watcher) pass(ctx context.Context, kind string, fn func(cycle int64) error) error {
	id, err := w.St.Cycles.Begin(ctx, kind, store.TriggerWatch)
	if err != nil {
		return err
	}
	if err := fn(id); err != nil {
		w.St.Cycles.Fail(ctx, id)
		w.logf("%s cycle %d failed: %v", kind, id, err)
		return err
	}
	return w.St.Cycles.Complete(ctx, id)
}

// Discovery fetches market lists from every venue, then runs match only if
// any card was added or changed in this cycle (test W1). A venue failure is
// saved in its fetch run and does not fail the cycle (test W4).
func (w *Watcher) Discovery(ctx context.Context) error {
	return w.pass(ctx, store.KindDiscovery, func(cycle int64) error {
		for _, r := range registry.FetchAll(ctx, w.St, cycle, w.Fetchers, w.Timeout) {
			if r.Err != nil {
				return fmt.Errorf("%s: %w", r.Venue, r.Err)
			}
			w.logf("discovery cycle %d: %s %s, saved %d, skipped %d %s", cycle, r.Venue, r.Run.Status, r.Run.Saved, r.Run.Skipped, r.Run.Error)
		}
		changed, err := w.cardsChangedIn(ctx, cycle)
		if err != nil {
			return err
		}
		if changed == 0 {
			w.logf("discovery cycle %d: no card added and no payoff field changed; match skipped", cycle)
			return nil
		}
		sum, err := match.Run(ctx, w.St, cycle, w.Cfg)
		if err != nil {
			return err
		}
		w.logf("discovery cycle %d: %d cards added or with changed payoff fields; matched %d pairs (%d same, %d opposite)", cycle, changed, sum.Compared, sum.Same, sum.Opposite)
		return nil
	})
}

// cardsChangedIn counts the cards this cycle added, or whose payoff fields
// changed from the market's previous version. The matcher reads only payoff
// fields (hard rule 1), so a version that changed only display data (title,
// volume, url) cannot change any match result and does not trigger a match
// (test W1, decision D-21).
func (w *Watcher) cardsChangedIn(ctx context.Context, cycle int64) (int, error) {
	vs, err := w.St.Cards.Versions(ctx)
	if err != nil {
		return 0, err
	}
	prev := map[card.MarketRef][]card.Field{}
	n := 0
	for _, v := range vs { // ordered by id, so a market's earlier versions come first
		ref, fields := v.Card.Ref(), v.Card.Payoff.Fields()
		before, seen := prev[ref]
		if v.CycleID == cycle && (!seen || !sameFields(before, fields)) {
			n++
		}
		prev[ref] = fields
	}
	return n, nil
}

func sameFields(a, b []card.Field) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Prices refreshes prices for markets in same or opposite pairs only, saving a
// record on every poll (test W2). Then, with as_of = the refresh time, it
// routes both yes and no (test W9) for each pair whose needed price changed
// since that side's last route record (test W3).
func (w *Watcher) Prices(ctx context.Context) error {
	return w.pass(ctx, store.KindPrices, func(cycle int64) error {
		scope := store.Writing(cycle)
		matches, err := w.St.Log.LatestMatches(ctx, scope)
		if err != nil {
			return err
		}
		var pairs []card.LogRecord
		ids := map[string][]string{}
		seen := map[card.MarketRef]bool{}
		for _, m := range matches {
			if m.Match != card.Same && m.Match != card.Opposite {
				continue
			}
			pairs = append(pairs, m)
			for _, ref := range []card.MarketRef{m.MarketA, m.MarketB} {
				if !seen[ref] {
					seen[ref] = true
					ids[ref.Venue] = append(ids[ref.Venue], ref.MarketID)
				}
			}
		}
		if len(pairs) == 0 {
			w.logf("prices cycle %d: no matched pairs to refresh", cycle)
			return nil
		}
		for _, f := range w.Fetchers {
			if len(ids[f.Name()]) == 0 {
				continue
			}
			fctx, cancel := context.WithTimeout(ctx, w.Timeout)
			run, err := f.FetchPrices(fctx, w.St, cycle, ids[f.Name()])
			cancel()
			if err != nil {
				return fmt.Errorf("%s: %w", f.Name(), err)
			}
			w.logf("prices cycle %d: %s %s, saved %d, skipped %d %s", cycle, f.Name(), run.Status, run.Saved, run.Skipped, run.Error)
		}

		asOf := w.Clock().UTC()
		last, err := w.lastRoutes(ctx, scope)
		if err != nil {
			return err
		}
		routed := 0
		for _, m := range pairs {
			changed := false
			inputs := map[card.Side]route.Input{}
			for _, side := range []card.Side{card.Yes, card.No} {
				in, err := route.Prepare(ctx, w.St, scope, m, side, asOf, w.Cfg)
				if err != nil {
					return err
				}
				inputs[side] = in
				prev, ok := last[routeKey{m.MarketA, m.MarketB, side}]
				a := route.UsablePrice(in.PriceA, asOf, in.MaxAge)
				b := route.UsablePrice(in.PriceB, asOf, in.MaxAge)
				if !ok || !sameOpt(prev.PriceA, a) || !sameOpt(prev.PriceB, b) {
					changed = true
				}
			}
			if !changed {
				continue
			}
			for _, side := range []card.Side{card.Yes, card.No} {
				if _, err := w.St.Log.Append(ctx, cycle, route.Decide(inputs[side])); err != nil {
					return err
				}
				routed++
			}
		}
		w.logf("prices cycle %d: %d pairs, %d route records at as_of %s", cycle, len(pairs), routed, asOf.Format(time.RFC3339))
		return nil
	})
}

type routeKey struct {
	a, b card.MarketRef
	side card.Side
}

func (w *Watcher) lastRoutes(ctx context.Context, scope store.Scope) (map[routeKey]card.LogRecord, error) {
	recs, err := w.St.Log.LatestRoutes(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := map[routeKey]card.LogRecord{}
	for _, r := range recs {
		out[routeKey{r.MarketA, r.MarketB, r.SideA}] = r
	}
	return out, nil
}

// sameOpt is true when both prices are empty or both are set and equal. It is
// a change detector, not a match rule, so empty equals empty here.
func sameOpt(x, y card.Opt[card.Micros]) bool {
	return x.IsEmpty() && y.IsEmpty() || x.Equal(y)
}

// Retention prunes price records and price raw responses older than the
// retention window. Cards, market raw responses and the log are kept (test W8).
func (w *Watcher) Retention(ctx context.Context) error {
	return w.pass(ctx, store.KindRetention, func(cycle int64) error {
		cutoff := w.Clock().Add(-w.Cfg.Watch.Retention)
		p, r, err := w.St.Prices.Prune(ctx, cutoff)
		if err != nil {
			return err
		}
		w.logf("retention cycle %d: pruned %d price records and %d price raw responses before %s", cycle, p, r, cutoff.UTC().Format(time.RFC3339))
		return nil
	})
}

// Holder is the run lock name for this process.
func Holder() string { return fmt.Sprintf("equinox watch (pid %d)", os.Getpid()) }

// Run validates config, takes the run lock, and runs discovery, prices and
// retention until ctx ends; then it releases the lock. A failed pass is
// logged and the loop continues.
func (w *Watcher) Run(ctx context.Context) error {
	if err := Validate(w.Cfg); err != nil {
		return err
	}
	holder := Holder()
	if err := w.St.Lock.Acquire(ctx, holder); err != nil {
		return err
	}
	// Release with a fresh context: ctx is already cancelled on shutdown.
	defer w.St.Lock.Release(context.Background(), holder)
	w.logf("started: discovery every %s, prices every %s, retention %s", w.Cfg.Watch.DiscoveryEvery, w.Cfg.Watch.PricesEvery, w.Cfg.Watch.Retention)

	w.Discovery(ctx)
	w.Prices(ctx)
	discovery := time.NewTicker(w.Cfg.Watch.DiscoveryEvery)
	prices := time.NewTicker(w.Cfg.Watch.PricesEvery)
	retention := time.NewTicker(time.Hour)
	defer discovery.Stop()
	defer prices.Stop()
	defer retention.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logf("stopping; run lock released")
			return nil
		case <-discovery.C:
			w.Discovery(ctx)
		case <-prices.C:
			w.Prices(ctx)
		case <-retention.C:
			w.Retention(ctx)
		}
	}
}
