package route

import (
	"fmt"
	"strings"
	"time"

	"equinox/internal/card"
)

// Input is everything one routing decision may look at. It is built by the
// runner from saved records; Decide never fetches anything itself.
type Input struct {
	Match  card.Result
	A, B   card.MarketRef // A is the reference venue card, B the counter venue card
	Side   card.Side      // requested side, stated relative to card A
	PriceA *card.PriceRecord
	PriceB *card.PriceRecord // nil when no saved price exists
	AsOf   time.Time         // passed in; the router never reads the clock
	MaxAge time.Duration     // price_max_age from config
	Roles  card.Roles
}

// Decide applies the build requirements' routing rules in order and returns
// one route record. It is pure: same input, same record.
func Decide(in Input) card.LogRecord {
	rec := card.LogRecord{Type: card.RouteRecord, AsOf: in.AsOf, MarketA: in.A, MarketB: in.B, Match: in.Match, SideA: in.Side}

	// Rule 1: not_same is never routed.
	if in.Match != card.Same && in.Match != card.Opposite {
		rec.Action = card.NoRoute
		rec.Reason = "No route: the bets are not the same."
		return rec
	}

	// Rule 2: same compares the same side; opposite compares the other side,
	// because YES on one card pays exactly when NO on the other pays.
	rec.SideB = in.Side
	if in.Match == card.Opposite {
		rec.SideB = in.Side.Other()
	}

	// Rule 3: a missing or stale price means no route, naming which.
	a, problemA := usable(in.PriceA, in.A, rec.SideA, in.AsOf, in.MaxAge)
	b, problemB := usable(in.PriceB, in.B, rec.SideB, in.AsOf, in.MaxAge)
	if problemA != "" || problemB != "" {
		rec.Action = card.NoRoute
		rec.Reason = "No route: " + strings.TrimSpace(problemA+" "+problemB)
		return rec
	}
	priceA, _ := a.BuyPrice.Get()
	priceB, _ := b.BuyPrice.Get()
	rec.PriceA, rec.PriceB = a.BuyPrice, b.BuyPrice
	rec.PricedAtA, rec.PricedAtB = a.PricedAt, b.PricedAt

	// Rule 4: fees count only when known on both venues, so the comparison is like for like.
	costA, costB := priceA, priceB
	feeA, okA := a.Fee.Get()
	feeB, okB := b.Fee.Get()
	feeNote := "Fees were not applied because a fee is unknown on " + unknownFees(okA, okB, in.A, in.B) + "."
	if okA && okB {
		costA, costB = priceA+feeA, priceB+feeB
		rec.FeeApplied = true
		feeNote = fmt.Sprintf("Costs include fees of %s and %s.", feeA, feeB)
	}

	// Rules 5 and 6: lower cost wins; an exact tie goes to the reference venue.
	winner, why := in.Roles.Reference, "is cheaper"
	switch {
	case costB < costA:
		winner = in.Roles.Counter
	case costA == costB:
		why = "ties and wins exact ties as the reference venue"
	}
	rec.Action = card.BuyOn(winner)
	rec.Reason = fmt.Sprintf("Buy %s on %s at %s vs %s on %s at %s; %s %s. %s",
		rec.SideA, in.A.Venue, costA, rec.SideB, in.B.Venue, costB, winner, why, feeNote)
	return rec
}

// usable returns the price if it can be used, or a sentence saying why not.
func usable(p *card.PriceRecord, ref card.MarketRef, side card.Side, asOf time.Time, maxAge time.Duration) (card.PriceRecord, string) {
	switch {
	case p == nil || p.BuyPrice.IsEmpty():
		return card.PriceRecord{}, fmt.Sprintf("The %s %s price is missing for %s.", ref.Venue, side, ref.MarketID)
	case p.PricedAt.After(asOf):
		return card.PriceRecord{}, fmt.Sprintf("The %s %s price for %s was saved after as_of.", ref.Venue, side, ref.MarketID)
	case asOf.Sub(p.PricedAt) > maxAge:
		return card.PriceRecord{}, fmt.Sprintf("The %s %s price is stale for %s: saved %s before as_of, over the %s limit.",
			ref.Venue, side, ref.MarketID, asOf.Sub(p.PricedAt), maxAge)
	}
	return *p, ""
}

func unknownFees(okA, okB bool, a, b card.MarketRef) string {
	switch {
	case !okA && !okB:
		return "both venues"
	case !okA:
		return a.Venue
	}
	return b.Venue
}
