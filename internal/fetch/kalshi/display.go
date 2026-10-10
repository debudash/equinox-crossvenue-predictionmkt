package kalshi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"gopkg.in/yaml.v3"

	"equinox/internal/store"
)

// Display-only scope. Besides the Fed series, the fetcher can save markets
// from a configured list of Kalshi series so the feed shows a real
// multi-category venue. These cards are for display only: every payoff field
// except a display deadline stays empty, so the matcher never pairs them (see
// normalizeMarket and NORMALIZATION.md "Display-only categories").
//
// The list lives in config/equinox.yaml under fetchers.kalshi.display_categories.
// card.FetcherConfig is a frozen contract that holds only source and base_url,
// so this package reads its own section from the same YAML file (ParseDisplay);
// registry.LoadDisplay hands the result to the fetcher. No list configured
// means no display fetch, which is what every test and the acceptance harness
// get.

const (
	// defaultMaxEvents bounds one display category when max_events is absent.
	defaultMaxEvents = 10
	// defaultDisplayGap paces display requests (5 per second). The Fed
	// requests are not paced; they are two calls.
	defaultDisplayGap = 200 * time.Millisecond
	// displayRetries is how often a display request is retried after HTTP 429,
	// waiting 1 s, 2 s, 4 s.
	displayRetries = 3
)

// DisplayCategory is one display-only category: an ordered list of Kalshi
// series and how many open events to take from them.
type DisplayCategory struct {
	// Name is a label for logs and skip reasons only. The card category is
	// the event's own category string from the venue.
	Name string `yaml:"name"`
	// Series are fetched in order until MaxEvents events are saved. The order
	// is the ranking (by the series' lifetime volume when the list was made).
	Series []string `yaml:"series"`
	// MaxEvents bounds the category (default 10).
	MaxEvents int `yaml:"max_events"`
	// EventsPerSeries bounds one series (default MaxEvents), so one busy
	// series cannot fill the whole category.
	EventsPerSeries int `yaml:"events_per_series"`
}

// ParseDisplay reads fetchers.kalshi.display_categories from config YAML and
// applies defaults. A missing section gives an empty list. A Fed series in the
// list is refused: Fed series are always fetched and normalized with payoff
// fields, never as display-only.
func ParseDisplay(b []byte) ([]DisplayCategory, error) {
	var doc struct {
		Fetchers struct {
			Kalshi struct {
				Display []DisplayCategory `yaml:"display_categories"`
			} `yaml:"kalshi"`
		} `yaml:"fetchers"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("config: fetchers.kalshi.display_categories: %w", err)
	}
	out := doc.Fetchers.Kalshi.Display
	for i := range out {
		d := &out[i]
		if d.MaxEvents == 0 {
			d.MaxEvents = defaultMaxEvents
		}
		if d.EventsPerSeries == 0 {
			d.EventsPerSeries = d.MaxEvents
		}
		switch {
		case len(d.Series) == 0:
			return nil, fmt.Errorf("config: kalshi display category %q lists no series", d.Name)
		case d.MaxEvents < 0 || d.EventsPerSeries < 0 || d.EventsPerSeries > pageLimit:
			return nil, fmt.Errorf("config: kalshi display category %q: max_events and events_per_series must be 1 to %d", d.Name, pageLimit)
		}
		for _, s := range d.Series {
			if _, fed := fedSeries[s]; fed {
				return nil, fmt.Errorf("config: kalshi display category %q lists Fed series %s, which is always fetched with payoff fields", d.Name, s)
			}
		}
	}
	return out, nil
}

// fetchDisplay fetches one display category: each series' first open events
// (Kalshi lists them soonest first in practice) until the category bound is reached. A
// failed request is one skip with the reason; it never fails the run, so a
// display hiccup cannot mark the Fed fetch failed. Only store errors return.
func (f *Fetcher) fetchDisplay(ctx context.Context, st *store.Store, cycleID, runID int64, dc DisplayCategory, t *tally) error {
	skip := func(reason string) error {
		t.skipped++
		return st.Skips.Append(ctx, store.Skip{CycleID: cycleID, RunID: runID, Stage: store.StageFetch, Venue: Venue, Reason: reason})
	}
	got := 0
	for _, s := range dc.Series {
		if got >= dc.MaxEvents {
			return nil
		}
		// limit is part of the saved endpoint, so the raw response holds at
		// most this many events and rebuild normalizes exactly what fetch did.
		n := min(dc.EventsPerSeries, dc.MaxEvents-got)
		q := url.Values{
			"series_ticker":       {s},
			"status":              {"open"},
			"with_nested_markets": {"true"},
			"limit":               {fmt.Sprint(n)},
		}
		raw, err := f.downloadDisplay(ctx, st, cycleID, runID, q)
		if err != nil {
			if isVenueError(err) {
				if err := skip(fmt.Sprintf("display series %s (%s) not fetched: %v", s, dc.Name, err)); err != nil {
					return err
				}
				continue
			}
			return err
		}
		cards, prices, skips, next, err := normalizeMarkets(raw, f.vocab)
		if err != nil {
			if err := skip(fmt.Sprintf("display series %s (%s): %s: %v", s, dc.Name, raw.Endpoint, err)); err != nil {
				return err
			}
			continue
		}
		if err := save(ctx, st, cycleID, cards, prices, skips, t); err != nil {
			return err
		}
		t.saved += len(cards)
		page, _ := parseEventsPage(raw.Body) // parsed above, so no error here
		got += len(page.Events)
		if next != "" {
			// Bounded on purpose: say what was left behind.
			if err := skip(fmt.Sprintf("display series %s (%s): only the first %d open events fetched (display bound)", s, dc.Name, n)); err != nil {
				return err
			}
		}
	}
	return nil
}

// downloadDisplay paces display requests and retries HTTP 429 (1 s, 2 s, 4 s).
// A cancelled wait is a venue error, recorded as a skip by the caller.
func (f *Fetcher) downloadDisplay(ctx context.Context, st *store.Store, cycleID, runID int64, q url.Values) (store.Raw, error) {
	for attempt := 0; ; attempt++ {
		if err := f.sleep(ctx, f.DisplayGap); err != nil {
			return store.Raw{}, &venueError{err}
		}
		raw, err := f.download(ctx, st, cycleID, runID, store.RawMarkets, "/events", q)
		var he *HTTPError
		if err != nil && errors.As(err, &he) && he.StatusCode == http.StatusTooManyRequests && attempt < displayRetries {
			if err := f.sleep(ctx, time.Second<<attempt); err != nil {
				return store.Raw{}, &venueError{err}
			}
			continue
		}
		return raw, err
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
