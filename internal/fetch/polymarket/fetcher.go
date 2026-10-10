package polymarket

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"equinox/internal/card"
	"equinox/internal/store"
)

// Request settings. Polymarket US publishes a limit of 25 requests per second
// per IP for the public API and asks clients to back off on 429
// (docs.polymarket.us, "Rate Limits").
const (
	defaultBaseURL = "https://gateway.polymarket.us"
	requestTimeout = 15 * time.Second       // each request gets its own timeout
	requestGap     = 100 * time.Millisecond // at most 10 requests per second, well under 25
	maxRetries     = 3                      // retries after a 429, waiting 1s, 2s, 4s
	pageSize       = 100
	// maxPages bounds discovery: 5 pages of 100 macro markets. In October 2026
	// the macro category held 207 open markets.
	maxPages = 5
)

// Fetcher downloads Polymarket US public market data. It is the only code
// that knows Polymarket formats.
type Fetcher struct {
	baseURL string
	vocab   *card.Vocab
	client  *http.Client
	clock   func() time.Time

	// gap and sleep pace requests; tests set gap to 0 and sleep to a no-op.
	gap   time.Duration
	sleep func(ctx context.Context, d time.Duration) error
	last  time.Time
}

// New builds a fetcher. client is injected (tests pass testkit transports);
// clock stamps fetched_at and priced_at.
func New(cfg card.FetcherConfig, vocab *card.Vocab, client *http.Client, clock func() time.Time) *Fetcher {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	return &Fetcher{baseURL: base, vocab: vocab, client: client, clock: clock, gap: requestGap, sleep: sleepCtx}
}

// Name returns the venue value.
func (f *Fetcher) Name() string { return Venue }

// marketsQuery lists open markets in the macro category, where Polymarket US
// lists its Federal Reserve markets. Parameter order is fixed so the saved
// endpoint is stable.
func marketsQuery(offset int) string {
	return fmt.Sprintf("categories=macro&active=true&closed=false&limit=%d&offset=%d", pageSize, offset)
}

// Fetch runs one full fetch in cycleID. A venue, network, HTTP or parse
// failure is saved on the fetch run as failed and is not returned; only store
// errors are returned.
func (f *Fetcher) Fetch(ctx context.Context, st *store.Store, cycleID int64) (store.FetchRun, error) {
	runID, err := st.Runs.Start(ctx, cycleID, Venue)
	if err != nil {
		return store.FetchRun{}, err
	}
	r := &run{f: f, st: st, cycleID: cycleID, runID: runID}

	var slugs []string
	var venueErr error
	for page := 0; page < maxPages; page++ {
		raw, verr, err := r.get(ctx, "/v1/markets", marketsQuery(page*pageSize), store.RawMarkets)
		if err != nil {
			return store.FetchRun{}, err
		}
		if verr != nil {
			venueErr = verr
			break
		}
		cards, skips, n, outOfScope, perr := normalizeMarkets(raw, f.vocab)
		if perr != nil {
			venueErr = perr
			break
		}
		if outOfScope > 0 {
			// One summary skip row stands for outOfScope records; r.skips counts the row once.
			r.skipped += outOfScope - 1
		}
		if err := r.skips(ctx, skips); err != nil {
			return store.FetchRun{}, err
		}
		for _, c := range cards {
			if _, err := st.Cards.Save(ctx, cycleID, c); err != nil {
				return store.FetchRun{}, err
			}
			r.saved++
			slugs = append(slugs, c.MarketID)
		}
		if n < pageSize {
			break
		}
		if page == maxPages-1 {
			if err := r.skips(ctx, []store.Skip{{CycleID: cycleID, RunID: runID, Stage: store.StageFetch, Venue: Venue,
				Reason: fmt.Sprintf("pagination bound of %d pages reached; later markets not fetched", maxPages)}}); err != nil {
				return store.FetchRun{}, err
			}
		}
	}
	if venueErr == nil {
		if err := r.books(ctx, slugs); err != nil {
			return store.FetchRun{}, err
		}
	}
	return r.finish(ctx, venueErr)
}

// FetchPrices refreshes only the given markets (watch price loop). A book that
// fails to download is skipped with a reason; the run fails only if every
// requested book failed.
func (f *Fetcher) FetchPrices(ctx context.Context, st *store.Store, cycleID int64, marketIDs []string) (store.FetchRun, error) {
	runID, err := st.Runs.Start(ctx, cycleID, Venue)
	if err != nil {
		return store.FetchRun{}, err
	}
	r := &run{f: f, st: st, cycleID: cycleID, runID: runID}
	if err := r.books(ctx, marketIDs); err != nil {
		return store.FetchRun{}, err
	}
	var venueErr error
	if len(marketIDs) > 0 && r.booksOK == 0 {
		venueErr = fmt.Errorf("no order book could be fetched: %s", r.lastBookErr)
	}
	return r.finish(ctx, venueErr)
}

// run holds the counters of one fetch run.
type run struct {
	f              *Fetcher
	st             *store.Store
	cycleID, runID int64
	saved, skipped int
	booksOK        int
	lastBookErr    string
}

// books downloads and normalizes the order book of each market. One failed
// book is a skip, not a failed run (hard rule 8).
func (r *run) books(ctx context.Context, slugs []string) error {
	for _, slug := range slugs {
		raw, verr, err := r.get(ctx, "/v1/markets/"+url.PathEscape(slug)+"/book", "", store.RawPrices)
		if err != nil {
			return err
		}
		if verr != nil {
			r.lastBookErr = verr.Error()
			if err := r.skips(ctx, []store.Skip{{CycleID: r.cycleID, RunID: r.runID, Stage: store.StageFetch, Venue: Venue, MarketID: slug,
				Reason: "order book not fetched: " + verr.Error()}}); err != nil {
				return err
			}
			continue
		}
		r.booksOK++
		prices, skips := NormalizePrices(raw)
		if err := r.skips(ctx, skips); err != nil {
			return err
		}
		for _, p := range prices {
			if err := r.st.Prices.Append(ctx, r.cycleID, p); err != nil {
				return err
			}
			r.saved++
		}
	}
	return nil
}

func (r *run) skips(ctx context.Context, ks []store.Skip) error {
	for _, k := range ks {
		if err := r.st.Skips.Append(ctx, k); err != nil {
			return err
		}
		r.skipped++
	}
	return nil
}

func (r *run) finish(ctx context.Context, venueErr error) (store.FetchRun, error) {
	status, errText := store.RunOK, ""
	if venueErr != nil {
		status, errText = store.RunFailed, venueErr.Error()
	}
	if err := r.st.Runs.Finish(ctx, r.runID, status, r.saved, r.skipped, errText); err != nil {
		return store.FetchRun{}, err
	}
	runs, err := r.st.Runs.ForCycle(ctx, r.cycleID)
	if err != nil {
		return store.FetchRun{}, err
	}
	for _, x := range runs {
		if x.ID == r.runID {
			return x, nil
		}
	}
	return store.FetchRun{}, fmt.Errorf("fetch run %d not found after finish", r.runID)
}

// get downloads one endpoint and saves the response body before anything
// parses it. venueErr is a network, timeout or non-200 failure; storeErr is a
// failed save. Only 200 responses are saved: an error body is not market data,
// and its text is kept on the failed run or skip instead.
func (r *run) get(ctx context.Context, path, query string, kind string) (raw store.Raw, venueErr, storeErr error) {
	endpoint := path
	if query != "" {
		endpoint += "?" + query
	}
	body, err := r.f.download(ctx, endpoint)
	if err != nil {
		return store.Raw{}, err, nil
	}
	raw = store.Raw{CycleID: r.cycleID, RunID: r.runID, Venue: Venue, Kind: kind, Endpoint: endpoint,
		FetchedAt: r.f.clock().UTC(), Body: body}
	if raw.ID, err = r.st.Raw.Save(ctx, raw); err != nil {
		return store.Raw{}, nil, err
	}
	return raw, nil, nil
}

// download performs one GET with its own timeout, pacing requests and backing
// off on 429 (1s, 2s, 4s, then give up).
func (f *Fetcher) download(ctx context.Context, endpoint string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		if wait := f.gap - time.Since(f.last); f.gap > 0 && wait > 0 {
			if err := f.sleep(ctx, wait); err != nil {
				return nil, err
			}
		}
		f.last = time.Now()
		status, body, err := f.once(ctx, endpoint)
		if err != nil {
			return nil, err
		}
		if status == http.StatusTooManyRequests && attempt < maxRetries {
			if err := f.sleep(ctx, time.Second<<attempt); err != nil {
				return nil, err
			}
			continue
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("GET %s: HTTP %d: %s", endpoint, status, excerpt(body))
		}
		return body, nil
	}
}

func (f *Fetcher) once(ctx context.Context, endpoint string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+endpoint, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("GET %s: reading body: %w", endpoint, err)
	}
	return resp.StatusCode, body, nil
}

func excerpt(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
