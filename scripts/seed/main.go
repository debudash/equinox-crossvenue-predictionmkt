// Command seed builds a demo database from the SYNTHETIC fixture pair
// (fixtures/kalshi/synthetic, fixtures/polymarket/synthetic, decision D-19)
// through the real fetchers, matcher and router. It never calls a venue: the
// fetchers' HTTP client serves the fixture files. Prices are stamped with the
// current time, so routes are fresh. Running it again on the same database
// adds new completed cycles, which makes a running feed show its new-data
// banner. Reseed if more than 10 minutes pass before the demo.
//
//	go run ./scripts/seed --config config/equinox.yaml --db data/demo.db
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"equinox/internal/card"
	"equinox/internal/fetch/kalshi"
	"equinox/internal/fetch/polymarket"
	"equinox/internal/fetch/registry"
	"equinox/internal/match"
	"equinox/internal/route"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

func main() {
	cfgPath := flag.String("config", filepath.Join("config", "equinox.yaml"), "path to equinox.yaml")
	dbPath := flag.String("db", filepath.Join("data", "demo.db"), "database to write; delete it first for a clean start")
	flag.Parse()
	if err := run(*cfgPath, *dbPath); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(cfgPath, dbPath string) error {
	ctx := context.Background()
	cfg, err := card.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	vocab, err := card.LoadVocab(filepath.Join(filepath.Dir(cfgPath), "vocab.yaml"))
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	// Whole seconds, so an --as-of typed to the second is never before these prices (D-07).
	now := time.Now().UTC().Truncate(time.Second)
	clock := func() time.Time { return now }
	scenario := func(venue string) (*testkit.Transport, error) {
		return testkit.LoadScenario(filepath.Join("fixtures", venue, "synthetic"))
	}
	kt, err := scenario(kalshi.Venue)
	if err != nil {
		return err
	}
	pt, err := scenario(polymarket.Venue)
	if err != nil {
		return err
	}
	fs := []registry.Fetcher{
		kalshi.New(cfg.Fetchers[kalshi.Venue], vocab, kt.Client(), clock),
		polymarket.New(cfg.Fetchers[polymarket.Venue], vocab, pt.Client(), clock),
	}

	cycle := func(kind string, fn func(id int64) error) error {
		id, err := st.Cycles.Begin(ctx, kind, store.TriggerCLI)
		if err != nil {
			return err
		}
		if err := fn(id); err != nil {
			st.Cycles.Fail(ctx, id)
			return err
		}
		return st.Cycles.Complete(ctx, id)
	}
	if err := cycle(store.KindFetch, func(id int64) error {
		for _, r := range registry.FetchAll(ctx, st, id, fs, time.Minute) {
			if r.Err != nil {
				return r.Err
			}
			fmt.Printf("fetch   %-12s %s, saved %d, skipped %d\n", r.Venue, r.Run.Status, r.Run.Saved, r.Run.Skipped)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := cycle(store.KindMatch, func(id int64) error {
		sum, err := match.Run(ctx, st, id, cfg)
		fmt.Printf("match   %d pairs: %d same, %d opposite, %d not_same\n", sum.Compared, sum.Same, sum.Opposite, sum.NotSame)
		return err
	}); err != nil {
		return err
	}
	for _, side := range []card.Side{card.Yes, card.No} {
		if err := cycle(store.KindRoute, func(id int64) error {
			sum, err := route.Run(ctx, st, id, side, now, cfg)
			fmt.Printf("route   side %-3s as of %s: %d routed, %d no_route\n", side, now.Format(time.RFC3339), sum.Routed, sum.NoRoute)
			return err
		}); err != nil {
			return err
		}
	}
	fmt.Printf("SYNTHETIC demo database written to %s (no venue called)\n", dbPath)
	return nil
}
