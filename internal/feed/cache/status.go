package cache

import (
	"context"
	"sort"
	"strings"
	"time"

	"equinox/internal/store"
)

// Status is the refresh status shown in the feed sidebar. Unlike a snapshot it
// is reread from SQLite on every cache check, so a stopped watch shows even
// when no new cycle completes (test W10).
type Status struct {
	WatchRunning  bool
	Holder        string // run lock holder, if any
	LastDiscovery time.Time
	LastPrices    time.Time
	Venues        []VenueStatus
}

// VenueStatus is one venue's latest fetch run in a completed cycle.
type VenueStatus struct {
	Venue  string
	Saved  int
	At     time.Time // when the latest run finished
	Failed bool
	Error  string
	LastOK time.Time // when the latest successful run finished; zero if none
}

func (r storeReader) Status(ctx context.Context) (Status, error) {
	var s Status
	holder, _, held, err := r.st.Lock.Holder(ctx)
	if err != nil {
		return s, err
	}
	s.Holder = holder
	s.WatchRunning = held && strings.HasPrefix(holder, "equinox watch")
	if c, ok, err := r.st.Cycles.LatestCompletedOfKind(ctx, store.KindDiscovery); err != nil {
		return s, err
	} else if ok {
		s.LastDiscovery = c.CompletedAt
	}
	if c, ok, err := r.st.Cycles.LatestCompletedOfKind(ctx, store.KindPrices); err != nil {
		return s, err
	} else if ok {
		s.LastPrices = c.CompletedAt
	}
	latest, lastOK, err := r.st.Runs.LatestPerVenue(ctx)
	if err != nil {
		return s, err
	}
	for venue, run := range latest {
		s.Venues = append(s.Venues, VenueStatus{Venue: venue, Saved: run.Saved, At: run.FinishedAt,
			Failed: run.Status != store.RunOK, Error: run.Error, LastOK: lastOK[venue].FinishedAt})
	}
	sort.Slice(s.Venues, func(i, j int) bool { return s.Venues[i].Venue < s.Venues[j].Venue })
	return s, nil
}
