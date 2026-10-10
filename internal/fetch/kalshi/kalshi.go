package kalshi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"equinox/internal/card"
	"equinox/internal/store"
)

// Venue is the venue value on every Kalshi card and price record.
const Venue = "kalshi"

// DefaultBaseURL is Kalshi's public, unauthenticated market data API. Config
// fetchers.kalshi.base_url overrides it when set.
const DefaultBaseURL = "https://api.elections.kalshi.com/trade-api/v2"

const (
	// DefaultTimeout bounds each request on its own.
	DefaultTimeout = 20 * time.Second
	// pageLimit is events per page; Kalshi allows up to 200.
	pageLimit = 200
	// maxPages bounds pagination per series. Fed series have about 20 open
	// events, so one page is enough; a cursor left after maxPages is
	// recorded as a skip so nothing disappears silently.
	maxPages = 5
	// pricesChunk is tickers per price refresh request.
	pricesChunk = 100
	// maxBody bounds one response body.
	maxBody = 64 << 20
)

// series is the fetch scope, in request order: Federal Reserve rate markets.
var series = []string{"KXFED", "KXFEDFUNDSYEAR"}

// ErrTimeout marks a request that ran past its timeout.
var ErrTimeout = errors.New("kalshi: request timed out")

// HTTPError is a response with a status other than 200.
type HTTPError struct {
	Endpoint   string
	StatusCode int
	Body       string // first 200 bytes, for the run's error text
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("kalshi: %s returned HTTP %d: %s", e.Endpoint, e.StatusCode, e.Body)
}

// Fetcher downloads Kalshi Fed markets and prices.
type Fetcher struct {
	base   string
	vocab  *card.Vocab
	client *http.Client
	clock  func() time.Time
	// Timeout bounds each request; New sets DefaultTimeout.
	Timeout time.Duration
}

// New builds a fetcher. client is injected (tests pass testkit transports);
// clock stamps fetched_at and priced_at.
func New(cfg card.FetcherConfig, vocab *card.Vocab, client *http.Client, clock func() time.Time) *Fetcher {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	if client == nil {
		client = &http.Client{}
	}
	return &Fetcher{base: base, vocab: vocab, client: client, clock: clock, Timeout: DefaultTimeout}
}

// Name returns the venue.
func (f *Fetcher) Name() string { return Venue }

// tally collects one run's counts and venue errors.
type tally struct {
	saved, skipped int
	errs           []string
}

func (t *tally) fail(err error) { t.errs = append(t.errs, err.Error()) }

// Fetch runs one full fetch in cycleID. Every raw response is saved before it
// is parsed. A venue, network, HTTP or parse failure is recorded on the run
// (status failed, with the error text) and is not returned; one failed series
// does not stop the other. Only store errors are returned.
func (f *Fetcher) Fetch(ctx context.Context, st *store.Store, cycleID int64) (store.FetchRun, error) {
	runID, err := st.Runs.Start(ctx, cycleID, Venue)
	if err != nil {
		return store.FetchRun{}, err
	}
	var t tally
	for _, s := range series {
		if err := f.fetchSeries(ctx, st, cycleID, runID, s, &t); err != nil {
			return store.FetchRun{}, err
		}
	}
	return finish(ctx, st, cycleID, runID, &t)
}

func (f *Fetcher) fetchSeries(ctx context.Context, st *store.Store, cycleID, runID int64, seriesTicker string, t *tally) error {
	cursor := ""
	for page := 0; page < maxPages; page++ {
		q := url.Values{
			"series_ticker":       {seriesTicker},
			"status":              {"open"},
			"with_nested_markets": {"true"}, // one call returns event title, category and markets
			"limit":               {fmt.Sprint(pageLimit)},
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		raw, err := f.download(ctx, st, cycleID, runID, store.RawMarkets, "/events", q)
		if err != nil {
			if isVenueError(err) {
				t.fail(err)
				return nil
			}
			return err
		}
		cards, prices, skips, next, err := normalizeMarkets(raw, f.vocab)
		if err != nil {
			t.fail(fmt.Errorf("kalshi: %s: %w", raw.Endpoint, err))
			return nil
		}
		if err := save(ctx, st, cycleID, cards, prices, skips, t); err != nil {
			return err
		}
		t.saved += len(cards)
		if cursor = next; cursor == "" {
			return nil
		}
	}
	// Bounded pagination: say what was left behind.
	t.skipped++
	return st.Skips.Append(ctx, store.Skip{CycleID: cycleID, RunID: runID, Stage: store.StageFetch, Venue: Venue,
		Reason: fmt.Sprintf("series %s has more than %d pages of events; the rest were not fetched", seriesTicker, maxPages)})
}

// FetchPrices refreshes prices for the given markets only (the watch price
// loop). Each response is saved as a prices raw response before parsing. A
// requested market the venue does not return is recorded as a skip.
func (f *Fetcher) FetchPrices(ctx context.Context, st *store.Store, cycleID int64, marketIDs []string) (store.FetchRun, error) {
	runID, err := st.Runs.Start(ctx, cycleID, Venue)
	if err != nil {
		return store.FetchRun{}, err
	}
	ids := slices.Clone(marketIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var t tally
	for chunk := range slices.Chunk(ids, pricesChunk) {
		raw, err := f.download(ctx, st, cycleID, runID, store.RawPrices, "/markets", url.Values{"tickers": {strings.Join(chunk, ",")}})
		if err != nil {
			if isVenueError(err) {
				t.fail(err)
				continue
			}
			return store.FetchRun{}, err
		}
		prices, skips, err := normalizePrices(raw)
		if err != nil {
			t.fail(fmt.Errorf("kalshi: %s: %w", raw.Endpoint, err))
			continue
		}
		seen := map[string]bool{}
		for _, p := range prices {
			seen[p.MarketID] = true
		}
		for _, s := range skips {
			seen[s.MarketID] = true
		}
		for _, id := range chunk {
			if !seen[id] {
				skips = append(skips, skipOf(raw, id, "market was not returned by the venue"))
			}
		}
		if err := save(ctx, st, cycleID, nil, prices, skips, &t); err != nil {
			return store.FetchRun{}, err
		}
		t.saved += len(prices)
	}
	return finish(ctx, st, cycleID, runID, &t)
}

// download sends one GET with its own timeout and saves the body before any
// parsing. Only a 200 response is saved as a raw response; any other status
// becomes an HTTPError recorded on the run.
func (f *Fetcher) download(ctx context.Context, st *store.Store, cycleID, runID int64, kind, path string, q url.Values) (store.Raw, error) {
	full := f.base + path + "?" + q.Encode()
	u, err := url.Parse(full)
	if err != nil {
		return store.Raw{}, &venueError{err}
	}
	endpoint := u.RequestURI()
	body, err := f.get(ctx, full, endpoint)
	if err != nil {
		return store.Raw{}, &venueError{err}
	}
	raw := store.Raw{CycleID: cycleID, RunID: runID, Venue: Venue, Kind: kind, Endpoint: endpoint, FetchedAt: f.clock(), Body: body}
	if raw.ID, err = st.Raw.Save(ctx, raw); err != nil {
		return store.Raw{}, err
	}
	return raw, nil
}

func (f *Fetcher) get(ctx context.Context, full, endpoint string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, f.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		var ne net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			return nil, fmt.Errorf("%w after %s: %s", ErrTimeout, f.Timeout, endpoint)
		}
		return nil, fmt.Errorf("kalshi: %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("kalshi: %s: reading body: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, &HTTPError{Endpoint: endpoint, StatusCode: resp.StatusCode, Body: snippet}
	}
	return body, nil
}

// venueError wraps a failure that belongs on the run, not in the return value.
type venueError struct{ err error }

func (e *venueError) Error() string { return e.err.Error() }
func (e *venueError) Unwrap() error { return e.err }

func isVenueError(err error) bool {
	var ve *venueError
	return errors.As(err, &ve)
}

// save writes one response's cards, prices and skips in one transaction.
func save(ctx context.Context, st *store.Store, cycleID int64, cards []card.Card, prices []card.PriceRecord, skips []store.Skip, t *tally) error {
	err := st.Tx(ctx, func(tx *store.Store) error {
		for _, c := range cards {
			if _, err := tx.Cards.Save(ctx, cycleID, c); err != nil {
				return err
			}
		}
		for _, p := range prices {
			if err := tx.Prices.Append(ctx, cycleID, p); err != nil {
				return err
			}
		}
		for _, s := range skips {
			if err := tx.Skips.Append(ctx, s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	t.skipped += len(skips)
	return nil
}

func finish(ctx context.Context, st *store.Store, cycleID, runID int64, t *tally) (store.FetchRun, error) {
	status := store.RunOK
	if len(t.errs) > 0 {
		status = store.RunFailed
	}
	if err := st.Runs.Finish(ctx, runID, status, t.saved, t.skipped, strings.Join(t.errs, "; ")); err != nil {
		return store.FetchRun{}, err
	}
	runs, err := st.Runs.ForCycle(ctx, cycleID)
	if err != nil {
		return store.FetchRun{}, err
	}
	for _, r := range runs {
		if r.ID == runID {
			return r, nil
		}
	}
	return store.FetchRun{}, fmt.Errorf("fetch run %d not found", runID)
}
