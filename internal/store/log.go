package store

import (
	"context"
	"database/sql"
	"fmt"

	"equinox/internal/card"
)

// DecisionLog is the append-only decision log. It has Append and read methods
// only; a test asserts that no update or delete path exists, and SQLite
// triggers reject UPDATE and DELETE on the table.
type DecisionLog struct{ s *Store }

// Append writes one record in cycleID and returns its id. The store sets
// CycleID and DecidedAt; values passed in those fields are ignored.
func (r *DecisionLog) Append(ctx context.Context, cycleID int64, rec card.LogRecord) (int64, error) {
	if rec.Type != card.MatchRecord && rec.Type != card.RouteRecord {
		return 0, fmt.Errorf("log record type %q", rec.Type)
	}
	if rec.Reason == "" {
		return 0, fmt.Errorf("log record has no reason")
	}
	var fee any
	if rec.Type == card.RouteRecord {
		fee = "no"
		if rec.FeeApplied {
			fee = "yes"
		}
	}
	res, err := r.s.q.ExecContext(ctx,
		`INSERT INTO decision_log (cycle_id, `+logCols+`) VALUES (?, `+logMarks+`)`,
		cycleID, string(rec.Type), ts(r.s.now()), ts(rec.AsOf), rec.MarketA.String(), rec.MarketB.String(),
		string(rec.Match), nullable(string(rec.SideA)), nullable(string(rec.SideB)),
		micros(rec.PriceA), micros(rec.PriceB), ts(rec.PricedAtA), ts(rec.PricedAtB),
		fee, nullable(string(rec.Action)), rec.Reason)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// List returns every visible record in id order.
func (r *DecisionLog) List(ctx context.Context, sc Scope) ([]card.LogRecord, error) {
	cond, args := sc.filter("cycle_id")
	return r.query(ctx, `WHERE `+cond+` ORDER BY id`, args...)
}

// LatestMatches returns the latest visible match record for every pair, in
// pair order. Callers filter by result after taking the latest, never before.
func (r *DecisionLog) LatestMatches(ctx context.Context, sc Scope) ([]card.LogRecord, error) {
	cond, args := sc.filter("cycle_id")
	return r.query(ctx, `WHERE id IN (
		SELECT MAX(id) FROM decision_log WHERE record_type = 'match' AND `+cond+` GROUP BY market_a, market_b)
		ORDER BY market_a, market_b`, args...)
}

// LatestRoutes returns the latest visible route record for every pair and
// requested side.
func (r *DecisionLog) LatestRoutes(ctx context.Context, sc Scope) ([]card.LogRecord, error) {
	cond, args := sc.filter("cycle_id")
	return r.query(ctx, `WHERE id IN (
		SELECT MAX(id) FROM decision_log WHERE record_type = 'route' AND `+cond+` GROUP BY market_a, market_b, side_a)
		ORDER BY market_a, market_b, side_a`, args...)
}

// ForMarket returns every visible record that names a market, in id order.
func (r *DecisionLog) ForMarket(ctx context.Context, ref card.MarketRef, sc Scope) ([]card.LogRecord, error) {
	cond, args := sc.filter("cycle_id")
	return r.query(ctx, `WHERE (market_a = ? OR market_b = ?) AND `+cond+` ORDER BY id`,
		append([]any{ref.String(), ref.String()}, args...)...)
}

const (
	logCols = `record_type, decided_at, as_of, market_a, market_b, match_result, side_a, side_b,
		price_a, price_b, priced_at_a, priced_at_b, fee_applied, action, reason`
	logMarks = `?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?`
)

func (r *DecisionLog) query(ctx context.Context, where string, args ...any) ([]card.LogRecord, error) {
	rows, err := r.s.q.QueryContext(ctx, `SELECT id, cycle_id, `+logCols+` FROM decision_log `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []card.LogRecord
	for rows.Next() {
		rec, err := scanLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func scanLog(row scanner) (card.LogRecord, error) {
	var rec card.LogRecord
	var typ, a, b, match, reason string
	var decided, asOf, sideA, sideB, priceA, priceB, atA, atB, fee, action sql.NullString
	if err := row.Scan(&rec.ID, &rec.CycleID, &typ, &decided, &asOf, &a, &b, &match, &sideA, &sideB,
		&priceA, &priceB, &atA, &atB, &fee, &action, &reason); err != nil {
		return rec, err
	}
	rec.Type, rec.Match, rec.Reason = card.RecordType(typ), card.Result(match), reason
	rec.SideA, rec.SideB, rec.Action = card.Side(sideA.String), card.Side(sideB.String), card.Action(action.String)
	rec.FeeApplied = fee.String == "yes"
	var err error
	if rec.MarketA, err = card.ParseMarketRef(a); err != nil {
		return rec, err
	}
	if rec.MarketB, err = card.ParseMarketRef(b); err != nil {
		return rec, err
	}
	if rec.PriceA, err = parseMicros(priceA); err != nil {
		return rec, err
	}
	if rec.PriceB, err = parseMicros(priceB); err != nil {
		return rec, err
	}
	if rec.DecidedAt, err = parseTS(decided); err != nil {
		return rec, err
	}
	if rec.AsOf, err = parseTS(asOf); err != nil {
		return rec, err
	}
	if rec.PricedAtA, err = parseTS(atA); err != nil {
		return rec, err
	}
	rec.PricedAtB, err = parseTS(atB)
	return rec, err
}
