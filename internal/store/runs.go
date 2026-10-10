package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Fetch run statuses.
const (
	RunRunning = "running"
	RunOK      = "ok"
	RunFailed  = "failed"
)

// FetchRun is one fetcher's run within a cycle.
type FetchRun struct {
	ID         int64
	CycleID    int64
	Venue      string
	StartedAt  time.Time
	FinishedAt time.Time
	Status     string
	Saved      int
	Skipped    int
	Error      string
}

// FetchRuns is the fetch_runs repository.
type FetchRuns struct{ s *Store }

// Start records that a fetcher began, and returns the run id.
func (r *FetchRuns) Start(ctx context.Context, cycleID int64, venue string) (int64, error) {
	res, err := r.s.q.ExecContext(ctx,
		`INSERT INTO fetch_runs (cycle_id, venue, started_at, status) VALUES (?, ?, ?, 'running')`,
		cycleID, venue, ts(r.s.now()))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Finish records a run's outcome once. status is RunOK or RunFailed; errText is
// the saved venue error for a failed run.
func (r *FetchRuns) Finish(ctx context.Context, runID int64, status string, saved, skipped int, errText string) error {
	if status != RunOK && status != RunFailed {
		return fmt.Errorf("fetch run status %q", status)
	}
	res, err := r.s.q.ExecContext(ctx,
		`UPDATE fetch_runs SET status = ?, saved_count = ?, skipped_count = ?, error = ?, finished_at = ?
		 WHERE id = ? AND status = 'running'`,
		status, saved, skipped, nullable(errText), ts(r.s.now()), runID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("fetch run %d is not running", runID)
	}
	return nil
}

// ForCycle lists the fetch runs of one cycle in id order.
func (r *FetchRuns) ForCycle(ctx context.Context, cycleID int64) ([]FetchRun, error) {
	rows, err := r.s.q.QueryContext(ctx,
		`SELECT id, cycle_id, venue, started_at, finished_at, status, saved_count, skipped_count, error
		 FROM fetch_runs WHERE cycle_id = ? ORDER BY id`, cycleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FetchRun
	for rows.Next() {
		var f FetchRun
		var started, finished, errText sql.NullString
		if err := rows.Scan(&f.ID, &f.CycleID, &f.Venue, &started, &finished, &f.Status, &f.Saved, &f.Skipped, &errText); err != nil {
			return nil, err
		}
		if f.StartedAt, err = parseTS(started); err != nil {
			return nil, err
		}
		if f.FinishedAt, err = parseTS(finished); err != nil {
			return nil, err
		}
		f.Error = errText.String
		out = append(out, f)
	}
	return out, rows.Err()
}

// Skip stages.
const (
	StageFetch   = "fetch"
	StageRebuild = "rebuild"
	StageMatch   = "match"
	StageRoute   = "route"
)

// Skip is one record that was not used, and why. No record disappears silently.
type Skip struct {
	CycleID  int64
	RunID    int64 // 0 when not part of a fetch run
	Stage    string
	Venue    string
	MarketID string
	Reason   string
}

// Skips is the append-only skips repository.
type Skips struct{ s *Store }

// Append saves one skip.
func (r *Skips) Append(ctx context.Context, k Skip) error {
	var run any
	if k.RunID != 0 {
		run = k.RunID
	}
	_, err := r.s.q.ExecContext(ctx,
		`INSERT INTO skips (cycle_id, run_id, stage, venue, market_id, reason) VALUES (?, ?, ?, ?, ?, ?)`,
		k.CycleID, run, k.Stage, nullable(k.Venue), nullable(k.MarketID), k.Reason)
	return err
}

// ForCycle lists the skips of one cycle in insertion order.
func (r *Skips) ForCycle(ctx context.Context, cycleID int64) ([]Skip, error) {
	rows, err := r.s.q.QueryContext(ctx,
		`SELECT cycle_id, run_id, stage, venue, market_id, reason FROM skips WHERE cycle_id = ? ORDER BY id`, cycleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Skip
	for rows.Next() {
		var k Skip
		var run sql.NullInt64
		var venue, market sql.NullString
		if err := rows.Scan(&k.CycleID, &run, &k.Stage, &venue, &market, &k.Reason); err != nil {
			return nil, err
		}
		k.RunID, k.Venue, k.MarketID = run.Int64, venue.String, market.String
		out = append(out, k)
	}
	return out, rows.Err()
}

// Raw response kinds. Retention prunes price responses and keeps market
// responses so rebuild still works (amendment A5).
const (
	RawMarkets = "markets"
	RawPrices  = "prices"
)

// Raw is one saved venue response, saved before parsing.
type Raw struct {
	ID        int64
	CycleID   int64
	RunID     int64
	Venue     string
	Kind      string
	Endpoint  string // request path and query, as sent
	FetchedAt time.Time
	SHA256    string // set by Save
	Body      []byte
}

// RawResponses is the raw_responses repository.
type RawResponses struct{ s *Store }

// Save stores a raw response exactly as received and returns its id. The id,
// in decimal, is the card's raw_ref.
func (r *RawResponses) Save(ctx context.Context, raw Raw) (int64, error) {
	if raw.Kind != RawMarkets && raw.Kind != RawPrices {
		return 0, fmt.Errorf("raw response kind %q", raw.Kind)
	}
	sum := sha256.Sum256(raw.Body)
	var run any
	if raw.RunID != 0 {
		run = raw.RunID
	}
	res, err := r.s.q.ExecContext(ctx,
		`INSERT INTO raw_responses (cycle_id, run_id, venue, kind, endpoint, fetched_at, sha256, body)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		raw.CycleID, run, raw.Venue, raw.Kind, raw.Endpoint, ts(raw.FetchedAt), hex.EncodeToString(sum[:]), raw.Body)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// List returns every raw response of a kind in id order, for rebuild. Rebuild
// reads every saved response, including those of failed cycles.
func (r *RawResponses) List(ctx context.Context, kind string) ([]Raw, error) {
	rows, err := r.s.q.QueryContext(ctx,
		`SELECT id, cycle_id, run_id, venue, kind, endpoint, fetched_at, sha256, body
		 FROM raw_responses WHERE kind = ? ORDER BY id`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Raw
	for rows.Next() {
		var x Raw
		var run sql.NullInt64
		var fetched sql.NullString
		if err := rows.Scan(&x.ID, &x.CycleID, &run, &x.Venue, &x.Kind, &x.Endpoint, &fetched, &x.SHA256, &x.Body); err != nil {
			return nil, err
		}
		x.RunID = run.Int64
		if x.FetchedAt, err = parseTS(fetched); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SubjectCount is the number of cards for one subject on one venue.
type SubjectCount struct {
	Subject string // "" for cards with an empty subject
	Venue   string
	Count   int
}

// SubjectCounts is the append-only repository for per-subject card counts,
// written on every match run.
type SubjectCounts struct{ s *Store }

// Append saves the counts for one cycle.
func (r *SubjectCounts) Append(ctx context.Context, cycleID int64, counts []SubjectCount) error {
	for _, c := range counts {
		if _, err := r.s.q.ExecContext(ctx,
			`INSERT INTO subject_counts (cycle_id, subject, venue, count) VALUES (?, ?, ?, ?)`,
			cycleID, c.Subject, c.Venue, c.Count); err != nil {
			return err
		}
	}
	return nil
}

// ForCycle returns the counts saved by one cycle.
func (r *SubjectCounts) ForCycle(ctx context.Context, cycleID int64) ([]SubjectCount, error) {
	rows, err := r.s.q.QueryContext(ctx,
		`SELECT subject, venue, count FROM subject_counts WHERE cycle_id = ? ORDER BY subject, venue`, cycleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubjectCount
	for rows.Next() {
		var c SubjectCount
		if err := rows.Scan(&c.Subject, &c.Venue, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// LatestPerVenue returns each venue's newest finished fetch run in a completed
// cycle, and its newest successful one, for status displays.
func (r *FetchRuns) LatestPerVenue(ctx context.Context) (latest, lastOK map[string]FetchRun, err error) {
	latest, lastOK = map[string]FetchRun{}, map[string]FetchRun{}
	rows, err := r.s.q.QueryContext(ctx,
		`SELECT id, cycle_id, venue, started_at, finished_at, status, saved_count, skipped_count, error
		 FROM fetch_runs WHERE status != 'running'
		 AND cycle_id IN (SELECT id FROM cycles WHERE status = 'completed') ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f FetchRun
		var started, finished, errText sql.NullString
		if err := rows.Scan(&f.ID, &f.CycleID, &f.Venue, &started, &finished, &f.Status, &f.Saved, &f.Skipped, &errText); err != nil {
			return nil, nil, err
		}
		if f.StartedAt, err = parseTS(started); err != nil {
			return nil, nil, err
		}
		if f.FinishedAt, err = parseTS(finished); err != nil {
			return nil, nil, err
		}
		f.Error = errText.String
		latest[f.Venue] = f
		if f.Status == RunOK {
			lastOK[f.Venue] = f
		}
	}
	return latest, lastOK, rows.Err()
}
