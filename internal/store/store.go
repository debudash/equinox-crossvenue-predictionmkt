package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"math"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaV1 string

// migrations[i] moves the database from user_version i to i+1.
var migrations = []string{schemaV1}

// querier is satisfied by *sql.DB and *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store is the SQLite database. Repositories hang off it as fields.
//
// The store stamps every "when was this written" time (decided_at, cycle and
// fetch run times) from Clock, so matcher and router code never read the clock.
type Store struct {
	db    *sql.DB
	q     querier
	Clock func() time.Time

	Cycles   *Cycles
	Lock     *RunLock
	Runs     *FetchRuns
	Skips    *Skips
	Raw      *RawResponses
	Cards    *Cards
	Prices   *Prices
	Log      *DecisionLog
	Subjects *SubjectCounts
}

// Open opens or creates the database at path and applies migrations.
// Use ":memory:" only through OpenMemory.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	return open(dsn)
}

// OpenMemory opens a private in-memory database, for tests.
func OpenMemory() (*Store, error) {
	s, err := open("file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// Every connection to ":memory:" is a separate database, so keep exactly one.
	s.db.SetMaxOpenConns(1)
	return s, nil
}

func open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, Clock: time.Now}
	s.bind(db)
	return s, nil
}

func (s *Store) bind(q querier) {
	s.q = q
	s.Cycles = &Cycles{s}
	s.Lock = &RunLock{s}
	s.Runs = &FetchRuns{s}
	s.Skips = &Skips{s}
	s.Raw = &RawResponses{s}
	s.Cards = &Cards{s}
	s.Prices = &Prices{s}
	s.Log = &DecisionLog{s}
	s.Subjects = &SubjectCounts{s}
}

func migrate(db *sql.DB) error {
	ctx := context.Background()
	var v int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	for ; v < len(migrations); v++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", v+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Tx runs fn with a Store whose repositories all use one transaction. The
// transaction commits if fn returns nil and rolls back otherwise.
func (s *Store) Tx(ctx context.Context, fn func(tx *Store) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	ts := &Store{db: s.db, Clock: s.Clock}
	ts.bind(tx)
	if err := fn(ts); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) now() time.Time { return s.Clock() }

// Scope selects which cycles' rows a read can see. Rows from a cycle that is
// open or failed are invisible, except the caller's own open cycle.
type Scope struct {
	UpTo int64 // only completed cycles with id <= UpTo; 0 means every completed cycle
	Own  int64 // also see rows written by this open cycle; 0 means none
}

// Completed is the scope for readers: every completed cycle.
var Completed = Scope{}

// Writing returns the scope for a writer in its own open cycle.
func Writing(cycleID int64) Scope { return Scope{Own: cycleID} }

// filter returns a SQL condition on the cycle_id column and its arguments.
func (sc Scope) filter(col string) (string, []any) {
	up := sc.UpTo
	if up == 0 {
		up = math.MaxInt64
	}
	return fmt.Sprintf("(%s IN (SELECT id FROM cycles WHERE status = 'completed' AND id <= ?) OR %s = ?)", col, col), []any{up, sc.Own}
}
