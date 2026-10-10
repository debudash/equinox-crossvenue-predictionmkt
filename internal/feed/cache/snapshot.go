package cache

import (
	"context"
	"time"

	"equinox/internal/card"
	"equinox/internal/store"
)

// Snapshot is the feed's view of one completed cycle. It is built once from
// SQLite with store.Scope{UpTo: cycle} and never changed afterwards, so every
// page rendered from it agrees with itself. Callers must not modify the
// slices it returns.
type Snapshot struct {
	Cycle       int64
	Kind        string // the cycle's kind, e.g. match or prices
	CompletedAt time.Time

	cards   []card.Card
	prices  map[card.MarketRef]map[card.Side]card.PriceRecord
	matches map[card.MarketRef][]card.LogRecord // latest match record of every pair naming the market
	routes  map[pair][]card.LogRecord           // latest route record per side, for each pair
}

type pair struct{ a, b card.MarketRef }

// Build reads cycle c's view from SQLite. Only rows from completed cycles up
// to c are visible, so an open or failed cycle never reaches the feed.
func Build(ctx context.Context, r Reader, c store.Cycle) (*Snapshot, error) {
	sc := store.Scope{UpTo: c.ID}
	cards, err := r.Cards(ctx, sc)
	if err != nil {
		return nil, err
	}
	prices, err := r.Prices(ctx, sc)
	if err != nil {
		return nil, err
	}
	matches, err := r.Matches(ctx, sc)
	if err != nil {
		return nil, err
	}
	routes, err := r.Routes(ctx, sc)
	if err != nil {
		return nil, err
	}
	s := &Snapshot{
		Cycle: c.ID, Kind: c.Kind, CompletedAt: c.CompletedAt,
		cards:   cards,
		prices:  map[card.MarketRef]map[card.Side]card.PriceRecord{},
		matches: map[card.MarketRef][]card.LogRecord{},
		routes:  map[pair][]card.LogRecord{},
	}
	for _, p := range prices {
		ref := card.MarketRef{Venue: p.Venue, MarketID: p.MarketID}
		if s.prices[ref] == nil {
			s.prices[ref] = map[card.Side]card.PriceRecord{}
		}
		s.prices[ref][p.Side] = p
	}
	for _, m := range matches {
		s.matches[m.MarketA] = append(s.matches[m.MarketA], m)
		s.matches[m.MarketB] = append(s.matches[m.MarketB], m)
	}
	for _, rt := range routes {
		k := pair{rt.MarketA, rt.MarketB}
		s.routes[k] = append(s.routes[k], rt)
	}
	return s, nil
}

// Cards returns the latest version of every card, ordered by venue then market id.
func (s *Snapshot) Cards() []card.Card { return s.cards }

// Price returns the latest saved price for one side of one market.
func (s *Snapshot) Price(ref card.MarketRef, side card.Side) (card.PriceRecord, bool) {
	p, ok := s.prices[ref][side]
	return p, ok
}

// Matches returns the latest match record of every pair that names ref.
func (s *Snapshot) Matches(ref card.MarketRef) []card.LogRecord { return s.matches[ref] }

// Routes returns the latest route record for each requested side of a pair,
// ordered by side.
func (s *Snapshot) Routes(a, b card.MarketRef) []card.LogRecord { return s.routes[pair{a, b}] }
