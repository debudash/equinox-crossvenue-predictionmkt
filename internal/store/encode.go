package store

import (
	"database/sql"
	"fmt"
	"time"

	"equinox/internal/card"
)

// timeLayout is fixed width so that text order in SQLite is time order.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

// ts encodes a time for storage; the zero time is NULL.
func ts(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(timeLayout)
}

func parseTS(ns sql.NullString) (time.Time, error) {
	if !ns.Valid {
		return time.Time{}, nil
	}
	t, err := time.Parse(timeLayout, ns.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("stored time %q: %w", ns.String, err)
	}
	return t, nil
}

// opt encodes an optional value as its text form, or NULL when empty.
func opt[T comparable](o card.Opt[T], str func(T) string) any {
	v, ok := o.Get()
	if !ok {
		return nil
	}
	return str(v)
}

func ident(s string) string { return s }

func micros(o card.Opt[card.Micros]) any { return opt(o, card.Micros.String) }

func parseMicros(ns sql.NullString) (card.Opt[card.Micros], error) {
	if !ns.Valid {
		return card.None[card.Micros](), nil
	}
	m, err := card.ParseMicros(ns.String)
	if err != nil {
		return card.None[card.Micros](), err
	}
	return card.Some(m), nil
}

func optString(ns sql.NullString) card.Opt[string] {
	if !ns.Valid {
		return card.None[string]()
	}
	return card.Some(ns.String)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
