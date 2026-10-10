package match

import (
	"fmt"

	"equinox/internal/card"
)

// Compare returns same, opposite or not_same for two cards by exact equality
// of their normalized payoff fields (build requirements, "Equivalence").
//
// Fields are checked in canonical order. The first field that is empty on
// either card, or that differs, decides not_same and is named in the reason.
// cutoff_op may differ only as an exact complement (gt/lte, gte/lt), which
// makes the pair opposite. Titles and rules text are never read. The result
// and reason do not depend on argument order.
func Compare(a, b card.Card) card.MatchResult {
	// Order the cards by reference so the reason text is symmetric.
	if b.Ref().String() < a.Ref().String() {
		a, b = b, a
	}
	fa, fb := a.Payoff.Fields(), b.Payoff.Fields()
	complement := false
	for i := range fa {
		x, y := fa[i], fb[i]
		switch {
		case !x.Set && !y.Set:
			return notSame(fmt.Sprintf("%s is empty on both cards.", x.Name))
		case !x.Set:
			return notSame(fmt.Sprintf("%s is empty on %s.", x.Name, a.Ref()))
		case !y.Set:
			return notSame(fmt.Sprintf("%s is empty on %s.", y.Name, b.Ref()))
		case x.Value == y.Value:
			continue
		case x.Name == card.FieldCutoffOp && card.Op(x.Value).Complement() == card.Op(y.Value):
			complement = true
			continue
		default:
			return notSame(fmt.Sprintf("%s differs: %s has %s, %s has %s.", x.Name, a.Ref(), x.Value, b.Ref(), y.Value))
		}
	}
	if complement {
		return card.MatchResult{Result: card.Opposite, Reason: fmt.Sprintf(
			"All payoff fields are equal except cutoff_op, where %s is the exact complement of %s, so YES on one pays exactly when NO on the other pays.",
			fa[1].Value, fb[1].Value)}
	}
	return card.MatchResult{Result: card.Same, Reason: "All seven payoff fields are equal, so YES on one pays exactly when YES on the other pays."}
}

func notSame(reason string) card.MatchResult {
	return card.MatchResult{Result: card.NotSame, Reason: "Not the same bet: " + reason}
}
