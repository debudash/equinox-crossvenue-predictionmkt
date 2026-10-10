package route

import (
	"context"
	"time"

	"equinox/internal/card"
	"equinox/internal/store"
)

// Summary counts what one route run did.
type Summary struct {
	Routed, NoRoute int
}

// Run routes every pair whose latest match record is same or opposite, for one
// requested side at asOf, and appends one route record per pair. It reads only
// saved match records and prices; it makes no venue call and reads no clock.
func Run(ctx context.Context, st *store.Store, cycleID int64, side card.Side, asOf time.Time, cfg *card.Config) (Summary, error) {
	var sum Summary
	scope := store.Writing(cycleID)
	matches, err := st.Log.LatestMatches(ctx, scope)
	if err != nil {
		return sum, err
	}
	for _, m := range matches {
		// Latest record per pair first, then filter: a pair that became not_same is not routed.
		if m.Match != card.Same && m.Match != card.Opposite {
			continue
		}
		in, err := Prepare(ctx, st, scope, m, side, asOf, cfg)
		if err != nil {
			return sum, err
		}
		rec := Decide(in)
		if _, err := st.Log.Append(ctx, cycleID, rec); err != nil {
			return sum, err
		}
		if rec.Action == card.NoRoute {
			sum.NoRoute++
		} else {
			sum.Routed++
		}
	}
	return sum, nil
}

// Prepare builds the routing input for one matched pair and requested side
// from saved prices: the latest price per needed card and side with priced_at
// not after asOf (decision D-07).
func Prepare(ctx context.Context, st *store.Store, scope store.Scope, m card.LogRecord, side card.Side, asOf time.Time, cfg *card.Config) (Input, error) {
	in := Input{Match: m.Match, A: m.MarketA, B: m.MarketB, Side: side, AsOf: asOf, MaxAge: cfg.Router.PriceMaxAge, Roles: cfg.Roles()}
	sideB := side
	if m.Match == card.Opposite {
		sideB = side.Other()
	}
	var err error
	if in.PriceA, err = latest(ctx, st, m.MarketA, side, asOf, scope); err != nil {
		return in, err
	}
	in.PriceB, err = latest(ctx, st, m.MarketB, sideB, asOf, scope)
	return in, err
}

func latest(ctx context.Context, st *store.Store, ref card.MarketRef, side card.Side, asOf time.Time, sc store.Scope) (*card.PriceRecord, error) {
	p, ok, err := st.Prices.LatestAt(ctx, ref, side, asOf, sc)
	if err != nil || !ok {
		return nil, err
	}
	return &p, nil
}
