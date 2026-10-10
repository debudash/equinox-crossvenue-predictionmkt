// Package acceptance holds the end-to-end tests for the required tests that
// cross packages (16, 17, 19, 20) and the synthetic happy path, plus the small
// harness they share with internal/redteam.
//
// The harness runs the real pieces in order, as the CLI does: the Kalshi and
// Polymarket fetchers over saved fixtures (testkit transports, never the
// network), then match.Run, then route.Run, all in one SQLite store with a
// fixed clock. Nothing here decides anything; it only wires and reads back.
package acceptance

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"equinox/internal/card"
	"equinox/internal/fetch/kalshi"
	"equinox/internal/fetch/polymarket"
	"equinox/internal/match"
	"equinox/internal/route"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

// Synthetic market ids (decision D-19). See fixtures/*/synthetic/README.md.
const (
	KalshiT375 = "KXFED-26DEC-T3.75"
	KalshiT400 = "KXFED-26DEC-T4.00"
	PolyGT375  = "synthetic-fedfnds-2026-12-09-gt3pt75"
	PolyLTE375 = "synthetic-fedfnds-2026-12-09-lte3pt75"
)

// FetchedAt is the fixed fetcher clock for a fresh fetch: two minutes before
// testkit.AsOf, well inside the 10 minute price age limit.
var FetchedAt = testkit.AsOf.Add(-2 * time.Minute)

// Env is one store plus the real config and vocabulary.
type Env struct {
	T     testing.TB
	Ctx   context.Context
	St    *store.Store
	Cfg   *card.Config
	Vocab *card.Vocab
}

// NewEnv opens a fresh in-memory store (clock fixed at testkit.AsOf) and loads
// config/equinox.yaml and config/vocab.yaml.
func NewEnv(t testing.TB) *Env {
	t.Helper()
	root := testkit.RepoRoot(t)
	cfg, err := card.LoadConfig(filepath.Join(root, "config", "equinox.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := card.LoadVocab(filepath.Join(root, "config", "vocab.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// An in-memory store behaves like the file store (same schema and
	// triggers) without a disk sync per write, which keeps the full real
	// fixtures (hundreds of cards) fast.
	st, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	st.Clock = testkit.FixedClock(testkit.AsOf)
	t.Cleanup(func() { st.Close() })
	return &Env{T: t, Ctx: context.Background(), St: st, Cfg: cfg, Vocab: v}
}

// Fixture returns fixtures/<parts...> from anywhere in the module.
func Fixture(t testing.TB, parts ...string) string {
	t.Helper()
	return filepath.Join(append([]string{testkit.RepoRoot(t), "fixtures"}, parts...)...)
}

// Begin opens a cycle.
func (e *Env) Begin() int64 {
	e.T.Helper()
	id, err := e.St.Cycles.Begin(e.Ctx, store.KindFetch, store.TriggerCLI)
	if err != nil {
		e.T.Fatal(err)
	}
	return id
}

// Complete marks a cycle completed, so later cycles see its rows.
func (e *Env) Complete(cycleID int64) {
	e.T.Helper()
	if err := e.St.Cycles.Complete(e.Ctx, cycleID); err != nil {
		e.T.Fatal(err)
	}
}

// FetchKalshi runs the Kalshi fetcher over a scenario directory with the
// fetcher clock fixed at at. A venue failure is on the returned run; only a
// store error fails the test.
func (e *Env) FetchKalshi(cycleID int64, dir string, at time.Time) store.FetchRun {
	e.T.Helper()
	tr := testkit.Scenario(e.T, dir)
	f := kalshi.New(e.Cfg.Fetchers[kalshi.Venue], e.Vocab, tr.Client(), testkit.FixedClock(at))
	f.Timeout = 2 * time.Second
	run, err := f.Fetch(e.Ctx, e.St, cycleID)
	if err != nil {
		e.T.Fatalf("kalshi Fetch returned a store error: %v", err)
	}
	return run
}

// FetchPolymarket runs the Polymarket US fetcher over a scenario directory.
func (e *Env) FetchPolymarket(cycleID int64, dir string, at time.Time) store.FetchRun {
	e.T.Helper()
	tr := testkit.Scenario(e.T, dir)
	f := polymarket.New(e.Cfg.Fetchers[polymarket.Venue], e.Vocab, tr.Client(), testkit.FixedClock(at))
	run, err := f.Fetch(e.Ctx, e.St, cycleID)
	if err != nil {
		e.T.Fatalf("polymarket Fetch returned a store error: %v", err)
	}
	return run
}

// FetchSynthetic fetches both synthetic scenarios at at.
func (e *Env) FetchSynthetic(cycleID int64, at time.Time) {
	e.T.Helper()
	for _, run := range []store.FetchRun{
		e.FetchKalshi(cycleID, Fixture(e.T, "kalshi", "synthetic"), at),
		e.FetchPolymarket(cycleID, Fixture(e.T, "polymarket", "synthetic"), at),
	} {
		if run.Status != store.RunOK {
			e.T.Fatalf("%s synthetic fetch run = %+v", run.Venue, run)
		}
	}
}

// Match runs the matcher in a cycle.
func (e *Env) Match(cycleID int64) match.Summary {
	e.T.Helper()
	sum, err := match.Run(e.Ctx, e.St, cycleID, e.Cfg)
	if err != nil {
		e.T.Fatalf("match.Run: %v", err)
	}
	return sum
}

// Route runs the router in a cycle for one side at asOf.
func (e *Env) Route(cycleID int64, side card.Side, asOf time.Time) route.Summary {
	e.T.Helper()
	sum, err := route.Run(e.Ctx, e.St, cycleID, side, asOf, e.Cfg)
	if err != nil {
		e.T.Fatalf("route.Run: %v", err)
	}
	return sum
}

// Card returns the latest version of a card visible to a cycle, failing the
// test when there is none.
func (e *Env) Card(cycleID int64, venue, id string) card.Card {
	e.T.Helper()
	c, ok := e.FindCard(cycleID, venue, id)
	if !ok {
		e.T.Fatalf("no card %s:%s", venue, id)
	}
	return c
}

// FindCard returns the latest version of a card visible to a cycle.
func (e *Env) FindCard(cycleID int64, venue, id string) (card.Card, bool) {
	e.T.Helper()
	c, ok, err := e.St.Cards.Latest(e.Ctx, card.MarketRef{Venue: venue, MarketID: id}, store.Writing(cycleID))
	if err != nil {
		e.T.Fatal(err)
	}
	return c, ok
}

// Cards returns every card visible to a cycle.
func (e *Env) Cards(cycleID int64) []card.Card {
	e.T.Helper()
	cs, err := e.St.Cards.LatestAll(e.Ctx, store.Writing(cycleID))
	if err != nil {
		e.T.Fatal(err)
	}
	return cs
}

// Log returns the decision log records written by one cycle, in order.
func (e *Env) Log(cycleID int64, typ card.RecordType) []card.LogRecord {
	e.T.Helper()
	all, err := e.St.Log.List(e.Ctx, store.Writing(cycleID))
	if err != nil {
		e.T.Fatal(err)
	}
	var out []card.LogRecord
	for _, r := range all {
		if r.CycleID == cycleID && r.Type == typ {
			out = append(out, r)
		}
	}
	return out
}

// Pair returns the one record of a type for a pair in a cycle (and side, for
// route records), failing the test if there is not exactly one.
func (e *Env) Pair(cycleID int64, typ card.RecordType, a, b string, side card.Side) card.LogRecord {
	e.T.Helper()
	var found []card.LogRecord
	for _, r := range e.Log(cycleID, typ) {
		if r.MarketA.MarketID == a && r.MarketB.MarketID == b && (typ == card.MatchRecord || r.SideA == side) {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		e.T.Fatalf("%d %s records for %s / %s side %q in cycle %d, want 1", len(found), typ, a, b, side, cycleID)
	}
	return found[0]
}

// Skips returns the skips of a cycle.
func (e *Env) Skips(cycleID int64) []store.Skip {
	e.T.Helper()
	ks, err := e.St.Skips.ForCycle(e.Ctx, cycleID)
	if err != nil {
		e.T.Fatal(err)
	}
	return ks
}

// Runs returns the fetch runs of a cycle keyed by venue.
func (e *Env) Runs(cycleID int64) map[string]store.FetchRun {
	e.T.Helper()
	rs, err := e.St.Runs.ForCycle(e.Ctx, cycleID)
	if err != nil {
		e.T.Fatal(err)
	}
	out := map[string]store.FetchRun{}
	for _, r := range rs {
		out[r.Venue] = r
	}
	return out
}

// FieldsText renders a card's payoff fields as "name=value" lines, so two
// cards with equal payoff render byte-equal text (test 16).
func FieldsText(c card.Card) string {
	var b strings.Builder
	for _, f := range c.Payoff.Fields() {
		b.WriteString(f.Name)
		b.WriteByte('=')
		if f.Set {
			b.WriteString(f.Value)
		} else {
			b.WriteString("<empty>")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Edit is one exact byte replacement in one scenario file.
type Edit struct {
	File     string // path relative to the scenario directory
	Old, New string
}

// Variant copies a scenario directory into a temp directory and applies exact
// replacements, failing the test if any replacement finds nothing. Red-team
// tests use it to make one-field variants of the synthetic scenarios without
// committing extra copies.
func Variant(t testing.TB, src string, edits ...Edit) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ed := range edits {
		p := filepath.Join(dst, filepath.FromSlash(ed.File))
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(b, []byte(ed.Old)) {
			t.Fatalf("variant: %s does not contain %q", ed.File, ed.Old)
		}
		if err := os.WriteFile(p, bytes.ReplaceAll(b, []byte(ed.Old), []byte(ed.New)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}
