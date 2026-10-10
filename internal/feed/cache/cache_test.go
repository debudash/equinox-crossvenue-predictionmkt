package cache

import (
	"context"
	"errors"
	"testing"

	"equinox/internal/card"
	"equinox/internal/store"
)

// fakeReader serves a latest completed cycle and records the scope of each read.
type fakeReader struct {
	latest store.Cycle
	fail   error
	scopes []store.Scope
}

func (f *fakeReader) LatestCompleted(context.Context) (store.Cycle, bool, error) {
	return f.latest, f.latest.ID > 0, f.fail
}

func (f *fakeReader) Cards(_ context.Context, sc store.Scope) ([]card.Card, error) {
	f.scopes = append(f.scopes, sc)
	return []card.Card{{Venue: "v", MarketID: "m"}}, nil
}

func (f *fakeReader) Prices(context.Context, store.Scope) ([]card.PriceRecord, error) {
	return nil, nil
}

func (f *fakeReader) Matches(context.Context, store.Scope) ([]card.LogRecord, error) {
	return nil, nil
}

func (f *fakeReader) Routes(context.Context, store.Scope) ([]card.LogRecord, error) {
	return nil, nil
}

func (f *fakeReader) Status(context.Context) (Status, error) { return Status{}, nil }

func TestCache_RefreshSwapsAndKeepsPrevious(t *testing.T) {
	ctx := context.Background()
	r := &fakeReader{}
	c := New(r, 2)
	if swapped, err := c.Refresh(ctx); err != nil || swapped || c.Current() != nil {
		t.Fatalf("no completed cycle: swapped=%v err=%v", swapped, err)
	}
	for _, id := range []int64{3, 7, 9} {
		r.latest = store.Cycle{ID: id, Status: store.StatusCompleted}
		if swapped, err := c.Refresh(ctx); err != nil || !swapped {
			t.Fatalf("cycle %d: swapped=%v err=%v", id, swapped, err)
		}
		if swapped, _ := c.Refresh(ctx); swapped {
			t.Fatalf("cycle %d: second refresh swapped with no newer cycle", id)
		}
	}
	if got := r.scopes[len(r.scopes)-1]; got != (store.Scope{UpTo: 9}) {
		t.Errorf("snapshot read with scope %+v, want UpTo 9 and no own cycle", got)
	}
	if c.Current().Cycle != 9 {
		t.Errorf("current is cycle %d, want 9", c.Current().Cycle)
	}
	if _, ok := c.At(7); !ok {
		t.Error("previous snapshot 7 should still be held")
	}
	if _, ok := c.At(3); ok {
		t.Error("snapshot 3 should be dropped with keep 2")
	}

	r.fail = errors.New("database is locked")
	if _, err := c.Refresh(ctx); err == nil || c.Current().Cycle != 9 {
		t.Error("a failed refresh should report the error and keep serving cycle 9")
	}
}
