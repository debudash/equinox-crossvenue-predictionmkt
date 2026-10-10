package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"equinox/internal/card"
)

// Cards is the cards repository. Each change to a card adds a version row, so
// a reader at an older completed cycle still sees that cycle's cards.
type Cards struct{ s *Store }

// SaveResult says what SaveCard did.
type SaveResult struct {
	New           bool // first version of this card
	Changed       bool // a new version was written (New implies Changed)
	PayoffChanged bool // the payoff fields differ from the previous version
}

// Save writes a new card version in cycleID if the card is new or any field
// other than raw_ref and fetched_at changed; otherwise it writes nothing.
// The comparison is against the latest version visible to this cycle.
func (r *Cards) Save(ctx context.Context, cycleID int64, c card.Card) (SaveResult, error) {
	prev, found, err := r.Latest(ctx, c.Ref(), Writing(cycleID))
	if err != nil {
		return SaveResult{}, err
	}
	next, old := cardColumns(c), cardColumns(prev)
	if found && slices.Equal(contentOf(old), contentOf(next)) {
		return SaveResult{}, nil
	}
	_, err = r.s.q.ExecContext(ctx, `INSERT INTO cards (cycle_id, `+cardCols+`) VALUES (?, `+cardMarks+`)`,
		append([]any{cycleID}, next...)...)
	if err != nil {
		return SaveResult{}, err
	}
	res := SaveResult{New: !found, Changed: true, PayoffChanged: true}
	if found {
		res.PayoffChanged = !slices.Equal(next[2:9], old[2:9])
	}
	return res, nil
}

// Latest returns the latest version of one card visible in scope.
func (r *Cards) Latest(ctx context.Context, ref card.MarketRef, sc Scope) (card.Card, bool, error) {
	cond, args := sc.filter("cycle_id")
	rows, err := r.s.q.QueryContext(ctx,
		`SELECT `+cardCols+` FROM cards WHERE venue = ? AND market_id = ? AND `+cond+` ORDER BY id DESC LIMIT 1`,
		append([]any{ref.Venue, ref.MarketID}, args...)...)
	if err != nil {
		return card.Card{}, false, err
	}
	cs, err := scanCards(rows)
	if err != nil || len(cs) == 0 {
		return card.Card{}, false, err
	}
	return cs[0], true, nil
}

// LatestAll returns the latest visible version of every card, ordered by venue
// then market id.
func (r *Cards) LatestAll(ctx context.Context, sc Scope) ([]card.Card, error) {
	cond, args := sc.filter("cycle_id")
	rows, err := r.s.q.QueryContext(ctx,
		`SELECT `+cardCols+` FROM cards WHERE id IN (
		   SELECT MAX(id) FROM cards WHERE `+cond+` GROUP BY venue, market_id)
		 ORDER BY venue, market_id`, args...)
	if err != nil {
		return nil, err
	}
	return scanCards(rows)
}

// Version is one stored card row, for rebuild and its test.
type Version struct {
	ID      int64
	CycleID int64
	Card    card.Card
}

// Versions returns every card version in id order.
func (r *Cards) Versions(ctx context.Context) ([]Version, error) {
	rows, err := r.s.q.QueryContext(ctx, `SELECT id, cycle_id, `+cardCols+` FROM cards ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		var v Version
		c, err := scanCard(rows, &v.ID, &v.CycleID)
		if err != nil {
			return nil, err
		}
		v.Card = c
		out = append(out, v)
	}
	return out, rows.Err()
}

// Clear deletes every card version and resets ids, so a rebuild that saves
// the same cards reproduces every card's versions, fields and cycles. Row ids
// can differ across venues when fetchers ran concurrently. Run it inside
// Store.Tx together with the re-saves.
func (r *Cards) Clear(ctx context.Context) error {
	if _, err := r.s.q.ExecContext(ctx, `DELETE FROM cards`); err != nil {
		return err
	}
	_, err := r.s.q.ExecContext(ctx, `DELETE FROM sqlite_sequence WHERE name = 'cards'`)
	return err
}

const (
	cardCols = `venue, market_id, subject, cutoff_op, cutoff_value, cutoff_unit, deadline, decider, unclear_result,
		title, rules_text, raw_ref, fetched_at, event_title, category, volume_24h, url`
	cardMarks = `?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?`
)

// cardColumns encodes a card in cardCols order. Indexes 2..8 are the payoff
// fields; 11 and 12 are raw_ref and fetched_at.
func cardColumns(c card.Card) []any {
	p, rec := c.Payoff, c.Record
	return []any{
		c.Venue, c.MarketID,
		opt(p.Subject, ident),
		opt(p.CutoffOp, func(o card.Op) string { return string(o) }),
		opt(p.CutoffValue, card.Decimal.String),
		opt(p.CutoffUnit, ident),
		opt(p.Deadline, card.Instant.String),
		opt(p.Decider, ident),
		opt(p.UnclearResult, func(u card.UnclearResult) string { return string(u) }),
		rec.Title, rec.RulesText, rec.RawRef, ts(rec.FetchedAt), rec.EventTitle, rec.Category, rec.Volume24h, rec.URL,
	}
}

// contentOf drops raw_ref and fetched_at, which change on every fetch.
func contentOf(cols []any) []any {
	return append(slices.Clone(cols[:11]), cols[13:]...)
}

type scanner interface{ Scan(dest ...any) error }

func scanCards(rows *sql.Rows) ([]card.Card, error) {
	defer rows.Close()
	var out []card.Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// scanCard reads cardCols, after any extra leading columns.
func scanCard(row scanner, lead ...any) (card.Card, error) {
	var c card.Card
	var subject, op, value, unit, deadline, decider, unclear, fetched sql.NullString
	dest := append(lead, &c.Venue, &c.MarketID, &subject, &op, &value, &unit, &deadline, &decider, &unclear,
		&c.Record.Title, &c.Record.RulesText, &c.Record.RawRef, &fetched,
		&c.Record.EventTitle, &c.Record.Category, &c.Record.Volume24h, &c.Record.URL)
	if err := row.Scan(dest...); err != nil {
		return card.Card{}, err
	}
	p := &c.Payoff
	p.Subject, p.CutoffUnit, p.Decider = optString(subject), optString(unit), optString(decider)
	if op.Valid {
		o, ok := card.ParseOp(op.String)
		if !ok {
			return card.Card{}, fmt.Errorf("stored cutoff_op %q", op.String)
		}
		p.CutoffOp = card.Some(o)
	}
	if value.Valid {
		d, err := card.ParseDecimal(value.String)
		if err != nil {
			return card.Card{}, err
		}
		p.CutoffValue = card.Some(d)
	}
	if deadline.Valid {
		i, err := card.ParseInstant(deadline.String)
		if err != nil {
			return card.Card{}, err
		}
		p.Deadline = card.Some(i)
	}
	if unclear.Valid {
		if p.UnclearResult = card.ParseUnclearResult(unclear.String); p.UnclearResult.IsEmpty() {
			return card.Card{}, errors.New("stored unclear_result " + unclear.String)
		}
	}
	var err error
	c.Record.FetchedAt, err = parseTS(fetched)
	return c, err
}
