package cache

import (
	"context"

	"equinox/internal/card"
	"equinox/internal/store"
)

// Reader is everything the cache may ask of SQLite. It lists read methods
// only, so no write path is reachable from the feed (AGENTS.md hard rule 10).
// A test asserts this method set.
type Reader interface {
	LatestCompleted(ctx context.Context) (store.Cycle, bool, error)
	Cards(ctx context.Context, sc store.Scope) ([]card.Card, error)
	Prices(ctx context.Context, sc store.Scope) ([]card.PriceRecord, error)
	Matches(ctx context.Context, sc store.Scope) ([]card.LogRecord, error)
	Routes(ctx context.Context, sc store.Scope) ([]card.LogRecord, error)
	Status(ctx context.Context) (Status, error)
}

// FromStore adapts a store to Reader. It forwards to read methods only.
func FromStore(st *store.Store) Reader { return storeReader{st} }

type storeReader struct{ st *store.Store }

func (r storeReader) LatestCompleted(ctx context.Context) (store.Cycle, bool, error) {
	return r.st.Cycles.LatestCompleted(ctx)
}

func (r storeReader) Cards(ctx context.Context, sc store.Scope) ([]card.Card, error) {
	return r.st.Cards.LatestAll(ctx, sc)
}

func (r storeReader) Prices(ctx context.Context, sc store.Scope) ([]card.PriceRecord, error) {
	return r.st.Prices.LatestAll(ctx, sc)
}

func (r storeReader) Matches(ctx context.Context, sc store.Scope) ([]card.LogRecord, error) {
	return r.st.Log.LatestMatches(ctx, sc)
}

func (r storeReader) Routes(ctx context.Context, sc store.Scope) ([]card.LogRecord, error) {
	return r.st.Log.LatestRoutes(ctx, sc)
}
