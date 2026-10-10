package store

import (
	"context"
	"database/sql"
	"time"

	"equinox/internal/card"
)

// Prices is the prices repository. Every poll appends a record, even when the
// price is unchanged, because freshness is judged by priced_at.
type Prices struct{ s *Store }

// Append saves one price record in cycleID.
func (r *Prices) Append(ctx context.Context, cycleID int64, p card.PriceRecord) error {
	_, err := r.s.q.ExecContext(ctx,
		`INSERT INTO prices (cycle_id, venue, market_id, side, buy_price, fee, priced_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cycleID, p.Venue, p.MarketID, string(p.Side), micros(p.BuyPrice), micros(p.Fee), ts(p.PricedAt))
	return err
}

// LatestAt returns the latest visible price for one market and side with
// priced_at not after asOf (decision D-07). The caller judges staleness.
func (r *Prices) LatestAt(ctx context.Context, ref card.MarketRef, side card.Side, asOf time.Time, sc Scope) (card.PriceRecord, bool, error) {
	cond, args := sc.filter("cycle_id")
	rows, err := r.s.q.QueryContext(ctx,
		`SELECT venue, market_id, side, buy_price, fee, priced_at FROM prices
		 WHERE venue = ? AND market_id = ? AND side = ? AND priced_at <= ? AND `+cond+`
		 ORDER BY priced_at DESC, id DESC LIMIT 1`,
		append([]any{ref.Venue, ref.MarketID, string(side), ts(asOf)}, args...)...)
	if err != nil {
		return card.PriceRecord{}, false, err
	}
	ps, err := scanPrices(rows)
	if err != nil || len(ps) == 0 {
		return card.PriceRecord{}, false, err
	}
	return ps[0], true, nil
}

// LatestAll returns the latest visible price for every market and side.
func (r *Prices) LatestAll(ctx context.Context, sc Scope) ([]card.PriceRecord, error) {
	cond, args := sc.filter("cycle_id")
	rows, err := r.s.q.QueryContext(ctx,
		`SELECT venue, market_id, side, buy_price, fee, priced_at FROM prices WHERE id IN (
		   SELECT id FROM (
		     SELECT id, ROW_NUMBER() OVER (PARTITION BY venue, market_id, side ORDER BY priced_at DESC, id DESC) AS n
		     FROM prices WHERE `+cond+`) WHERE n = 1)
		 ORDER BY venue, market_id, side`, args...)
	if err != nil {
		return nil, err
	}
	return scanPrices(rows)
}

// Prune deletes price records and price raw responses older than before.
// Market raw responses, cards and log records are never pruned (amendment A5).
func (r *Prices) Prune(ctx context.Context, before time.Time) (prices, raws int64, err error) {
	res, err := r.s.q.ExecContext(ctx, `DELETE FROM prices WHERE priced_at < ?`, ts(before))
	if err != nil {
		return 0, 0, err
	}
	prices, _ = res.RowsAffected()
	res, err = r.s.q.ExecContext(ctx, `DELETE FROM raw_responses WHERE kind = 'prices' AND fetched_at < ?`, ts(before))
	if err != nil {
		return prices, 0, err
	}
	raws, _ = res.RowsAffected()
	return prices, raws, nil
}

func scanPrices(rows *sql.Rows) ([]card.PriceRecord, error) {
	defer rows.Close()
	var out []card.PriceRecord
	for rows.Next() {
		var p card.PriceRecord
		var side string
		var buy, fee, at sql.NullString
		if err := rows.Scan(&p.Venue, &p.MarketID, &side, &buy, &fee, &at); err != nil {
			return nil, err
		}
		p.Side = card.Side(side)
		var err error
		if p.BuyPrice, err = parseMicros(buy); err != nil {
			return nil, err
		}
		if p.Fee, err = parseMicros(fee); err != nil {
			return nil, err
		}
		if p.PricedAt, err = parseTS(at); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
