package card

import (
	"errors"
	"fmt"
	"slices"
)

// ErrInconsistent marks saved data that must be skipped, counted and logged
// rather than used: unknown venue, bad side, price outside 0 to 1, or a
// deadline that does not parse. Its message is the skip reason.
var ErrInconsistent = errors.New("inconsistent record")

func inconsistent(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInconsistent, fmt.Sprintf(format, a...))
}

// CheckVenue fails if venue is not in the allowed list from config.
func CheckVenue(venue string, allowed []string) error {
	if !slices.Contains(allowed, venue) {
		return inconsistent("unknown venue %q", venue)
	}
	return nil
}

// CheckPrice fails if a price is outside 0 to 1 dollar.
func CheckPrice(m Micros) error {
	if m < 0 || m > OneDollar {
		return inconsistent("price %s is outside 0 to 1", m)
	}
	return nil
}

// CheckDeadline parses a venue deadline. A deadline that does not parse makes
// the record inconsistent.
func CheckDeadline(s string) (Instant, error) {
	i, err := ParseInstant(s)
	if err != nil {
		return Instant{}, inconsistent("%v", err)
	}
	return i, nil
}

// CheckPriceRecord validates a price record before it is saved or used.
func CheckPriceRecord(p PriceRecord, allowed []string) error {
	if err := CheckVenue(p.Venue, allowed); err != nil {
		return err
	}
	if _, err := ParseSide(string(p.Side)); err != nil {
		return err
	}
	if p.MarketID == "" {
		return inconsistent("price record has no market id")
	}
	if v, ok := p.BuyPrice.Get(); ok {
		if err := CheckPrice(v); err != nil {
			return err
		}
	}
	if v, ok := p.Fee.Get(); ok && v < 0 {
		return inconsistent("fee %s is negative", v)
	}
	return nil
}

// CheckCard validates the record fields of a card before it is saved or used.
// Payoff fields need no check: an unusable value is already stored as empty.
func CheckCard(c Card, allowed []string) error {
	if err := CheckVenue(c.Venue, allowed); err != nil {
		return err
	}
	if c.MarketID == "" {
		return inconsistent("card has no market id")
	}
	return nil
}
