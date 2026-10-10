package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Cycle kinds.
const (
	KindFetch     = "fetch"
	KindMatch     = "match"
	KindRoute     = "route"
	KindRebuild   = "rebuild"
	KindDiscovery = "discovery"
	KindPrices    = "prices"
	KindRetention = "retention"
)

// Cycle triggers.
const (
	TriggerCLI   = "cli"
	TriggerWatch = "watch"
)

// Cycle statuses.
const (
	StatusOpen      = "open"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Cycle is one numbered pass of fetch, match, route or rebuild (amendment A2).
type Cycle struct {
	ID          int64
	Kind        string
	Trigger     string
	StartedAt   time.Time
	CompletedAt time.Time // zero unless completed or failed
	Status      string
}

// Cycles is the cycles repository.
type Cycles struct{ s *Store }

// Begin opens a new cycle and returns its id.
func (r *Cycles) Begin(ctx context.Context, kind, trigger string) (int64, error) {
	res, err := r.s.q.ExecContext(ctx,
		`INSERT INTO cycles (kind, trigger, started_at, status) VALUES (?, ?, ?, 'open')`,
		kind, trigger, ts(r.s.now()))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Complete marks an open cycle completed, which makes its rows visible to readers.
func (r *Cycles) Complete(ctx context.Context, id int64) error {
	return r.finish(ctx, id, StatusCompleted)
}

// Fail marks an open cycle failed. Its rows stay invisible to readers.
func (r *Cycles) Fail(ctx context.Context, id int64) error {
	return r.finish(ctx, id, StatusFailed)
}

func (r *Cycles) finish(ctx context.Context, id int64, status string) error {
	res, err := r.s.q.ExecContext(ctx,
		`UPDATE cycles SET status = ?, completed_at = ? WHERE id = ? AND status = 'open'`,
		status, ts(r.s.now()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("cycle %d is not open", id)
	}
	return nil
}

// Get returns one cycle.
func (r *Cycles) Get(ctx context.Context, id int64) (Cycle, error) {
	return scanCycle(r.s.q.QueryRowContext(ctx, cycleSelect+` WHERE id = ?`, id))
}

// LatestCompleted returns the completed cycle with the highest id, or false if
// none has completed.
func (r *Cycles) LatestCompleted(ctx context.Context) (Cycle, bool, error) {
	c, err := scanCycle(r.s.q.QueryRowContext(ctx, cycleSelect+` WHERE status = 'completed' ORDER BY id DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return Cycle{}, false, nil
	}
	return c, err == nil, err
}

const cycleSelect = `SELECT id, kind, trigger, started_at, completed_at, status FROM cycles`

func scanCycle(row *sql.Row) (Cycle, error) {
	var c Cycle
	var started, completed sql.NullString
	if err := row.Scan(&c.ID, &c.Kind, &c.Trigger, &started, &completed, &c.Status); err != nil {
		return Cycle{}, err
	}
	var err error
	if c.StartedAt, err = parseTS(started); err != nil {
		return Cycle{}, err
	}
	c.CompletedAt, err = parseTS(completed)
	return c, err
}

// ErrLocked is returned when another writer holds the run lock.
var ErrLocked = errors.New("run lock held")

// LockedError says who holds the run lock and since when. Its message is the
// refusal reason shown to the user.
type LockedError struct {
	Holder string
	Since  time.Time
}

func (e *LockedError) Error() string {
	return fmt.Sprintf("refused: %s holds the run lock since %s; only one writer runs at a time",
		e.Holder, e.Since.UTC().Format(time.RFC3339))
}

func (e *LockedError) Unwrap() error { return ErrLocked }

// RunLock is the single-writer lock shared by watch and every writing CLI
// command (AGENTS.md hard rule 12).
type RunLock struct{ s *Store }

// Acquire takes the lock for holder, or returns a *LockedError naming the
// current holder.
func (r *RunLock) Acquire(ctx context.Context, holder string) error {
	_, err := r.s.q.ExecContext(ctx,
		`INSERT INTO run_lock (id, holder, acquired_at) VALUES (1, ?, ?)`, holder, ts(r.s.now()))
	if err == nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "constraint") {
		return err
	}
	h, since, held, herr := r.Holder(ctx)
	if herr != nil {
		return herr
	}
	if !held {
		return fmt.Errorf("run lock: acquire failed but no holder found: %w", err)
	}
	return &LockedError{Holder: h, Since: since}
}

// Release frees the lock if holder holds it.
func (r *RunLock) Release(ctx context.Context, holder string) error {
	res, err := r.s.q.ExecContext(ctx, `DELETE FROM run_lock WHERE id = 1 AND holder = ?`, holder)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("run lock is not held by %s", holder)
	}
	return nil
}

// Holder reports who holds the lock, if anyone.
func (r *RunLock) Holder(ctx context.Context) (holder string, since time.Time, held bool, err error) {
	var at sql.NullString
	err = r.s.q.QueryRowContext(ctx, `SELECT holder, acquired_at FROM run_lock WHERE id = 1`).Scan(&holder, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, err
	}
	since, err = parseTS(at)
	return holder, since, err == nil, err
}
