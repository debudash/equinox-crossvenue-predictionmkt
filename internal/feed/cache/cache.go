package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Cache holds the current snapshot plus a few older ones, so a reader who
// started scrolling on an older cycle finishes on it (amendment A3, test W6).
type Cache struct {
	r    Reader
	keep int

	mu     sync.Mutex          // serializes Refresh; readers never take it
	set    atomic.Pointer[set] // swapped whole, so readers see one consistent set
	status atomic.Pointer[Status]
}

// set is immutable once stored. held is newest first; held[0] is current.
type set struct{ held []*Snapshot }

// New returns an empty cache that keeps up to keep snapshots (at least 1).
func New(r Reader, keep int) *Cache {
	if keep < 1 {
		keep = 1
	}
	c := &Cache{r: r, keep: keep}
	c.set.Store(&set{})
	c.status.Store(&Status{})
	return c
}

// Status returns the refresh status read at the last check.
func (c *Cache) Status() Status { return *c.status.Load() }

// Current returns the newest snapshot, or nil before any cycle has completed.
func (c *Cache) Current() *Snapshot {
	if h := c.set.Load().held; len(h) > 0 {
		return h[0]
	}
	return nil
}

// At returns the held snapshot for a cycle, if it is still held.
func (c *Cache) At(cycle int64) (*Snapshot, bool) {
	for _, s := range c.set.Load().held {
		if s.Cycle == cycle {
			return s, true
		}
	}
	return nil, false
}

// Refresh checks for a newer completed cycle and, if there is one, builds its
// snapshot and swaps it in. It reports whether a swap happened. On error the
// held snapshots stay as they were, so the feed keeps serving.
func (c *Cache) Refresh(ctx context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, err := c.r.Status(ctx)
	if err != nil {
		return false, err
	}
	c.status.Store(&st)
	latest, ok, err := c.r.LatestCompleted(ctx)
	if err != nil || !ok {
		return false, err
	}
	if cur := c.Current(); cur != nil && cur.Cycle >= latest.ID {
		return false, nil
	}
	snap, err := Build(ctx, c.r, latest)
	if err != nil {
		return false, err
	}
	held := append([]*Snapshot{snap}, c.set.Load().held...)
	if len(held) > c.keep {
		held = held[:c.keep]
	}
	c.set.Store(&set{held: held})
	return true, nil
}

// Run calls Refresh every interval until ctx ends. A failed refresh is passed
// to onErr (if set) and the loop carries on with the snapshots it holds.
func (c *Cache) Run(ctx context.Context, every time.Duration, onErr func(error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// An error caused by shutdown is not a failed refresh.
			if _, err := c.Refresh(ctx); err != nil && ctx.Err() == nil && onErr != nil {
				onErr(err)
			}
		}
	}
}
