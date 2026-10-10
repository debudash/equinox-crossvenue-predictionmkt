package match

import (
	"context"
	"strings"
	"testing"

	"equinox/internal/card"
	"equinox/internal/store"
	"equinox/internal/testkit"
)

// Hand-built cards only. Venue names are neutral ("ref", "ctr") because this
// package must not contain real venue names (AGENTS.md hard rule 2).

func dec(s string) card.Decimal { d, _ := card.ParseDecimal(s); return d }
func at(s string) card.Instant  { i, _ := card.ParseInstant(s); return i }

func fed(venue, id, title string, op card.Op, value string) card.Card {
	return card.Card{
		Venue: venue, MarketID: id,
		Payoff: card.Payoff{
			Subject:       card.Some("fed_funds_upper_bound"),
			CutoffOp:      card.Some(op),
			CutoffValue:   card.Some(dec(value)),
			CutoffUnit:    card.Some("percent"),
			Deadline:      card.Some(at("2026-12-10T19:00:00Z")),
			Decider:       card.Some("federal_reserve"),
			UnclearResult: card.Some(card.YesOrNoOnly),
		},
		Record: card.RecordFields{Title: title, RulesText: "rules for " + title, FetchedAt: testkit.AsOf},
	}
}

func expect(t *testing.T, a, b card.Card, want card.Result, reasonHas ...string) card.MatchResult {
	t.Helper()
	got := Compare(a, b)
	if got.Result != want {
		t.Fatalf("got %s (%s), want %s", got.Result, got.Reason, want)
	}
	for _, s := range reasonHas {
		if !strings.Contains(got.Reason, s) {
			t.Errorf("reason %q does not contain %q", got.Reason, s)
		}
	}
	return got
}

func TestReq01_EqualPayoffIsSameDespiteTitles(t *testing.T) {
	a := fed("ref", "K1", "Fed upper bound above 3.5% in December?", card.Gt, "3.5")
	b := fed("ctr", "P1", "Will rates be over 3.50% after the Dec FOMC?", card.Gt, "3.50")
	expect(t, a, b, card.Same)
}

func TestReq02_GtVsLteIsOpposite(t *testing.T) {
	a := fed("ref", "K1", "above 3.5", card.Gt, "3.5")
	b := fed("ctr", "P1", "at or below 3.5", card.Lte, "3.5")
	expect(t, a, b, card.Opposite, "gt", "lte")
	c := fed("ref", "K2", "at or above 3.5", card.Gte, "3.5")
	d := fed("ctr", "P2", "below 3.5", card.Lt, "3.5")
	expect(t, c, d, card.Opposite)
}

func TestReq03_DifferentDeciderIsNotSame(t *testing.T) {
	a := fed("ref", "K1", "Same title", card.Gt, "3.5")
	b := fed("ctr", "P1", "Same title", card.Gt, "3.5")
	b.Payoff.Decider = card.Some("other_source")
	expect(t, a, b, card.NotSame, card.FieldDecider)
}

func TestReq04_DifferentDeadlineIsNotSame(t *testing.T) {
	a := fed("ref", "K1", "Same title", card.Gt, "3.5")
	b := fed("ctr", "P1", "Same title", card.Gt, "3.5")
	b.Payoff.Deadline = card.Some(at("2026-12-09T19:00:00Z"))
	expect(t, a, b, card.NotSame, card.FieldDeadline, "differs")
}

func TestReq05_GtVsGteIsNotSame(t *testing.T) {
	a := fed("ref", "K1", "above 3.5", card.Gt, "3.5")
	b := fed("ctr", "P1", "above 3.5", card.Gte, "3.5")
	expect(t, a, b, card.NotSame, card.FieldCutoffOp)
}

func TestReq06_EmptyDeadlineIsNotSame(t *testing.T) {
	a := fed("ref", "K1", "x", card.Gt, "3.5")
	b := fed("ctr", "P1", "x", card.Gt, "3.5")
	b.Payoff.Deadline = card.None[card.Instant]()
	expect(t, a, b, card.NotSame, "deadline is empty on ctr:P1")
	a.Payoff.Deadline = card.None[card.Instant]()
	expect(t, a, b, card.NotSame, "deadline is empty on both cards")
}

func TestReq07_OrderDoesNotMatter(t *testing.T) {
	base := fed("ref", "K1", "x", card.Gt, "3.5")
	variants := []func(*card.Card){
		func(c *card.Card) {},
		func(c *card.Card) { c.Payoff.CutoffOp = card.Some(card.Lte) },
		func(c *card.Card) { c.Payoff.CutoffOp = card.Some(card.Gte) },
		func(c *card.Card) { c.Payoff.Decider = card.Some("x") },
		func(c *card.Card) { c.Payoff.Deadline = card.None[card.Instant]() },
		func(c *card.Card) { c.Payoff.CutoffValue = card.Some(dec("3.75")) },
	}
	for i, v := range variants {
		b := fed("ctr", "P1", "y", card.Gt, "3.5")
		v(&b)
		ab, ba := Compare(base, b), Compare(b, base)
		if ab != ba {
			t.Errorf("variant %d: A,B = %+v but B,A = %+v", i, ab, ba)
		}
		if again := Compare(base, b); again != ab {
			t.Errorf("variant %d: not deterministic", i)
		}
	}
}

func TestMatch_Req17_EmptyFieldMatchesNothing(t *testing.T) {
	// An out-of-vocabulary value is stored as empty; such a card matches nothing, not even its own copy.
	a := fed("ref", "K1", "x", card.Gt, "3.5")
	a.Payoff.Decider = card.None[string]()
	b := a
	b.Venue, b.MarketID = "ctr", "P1"
	expect(t, a, b, card.NotSame, "decider is empty on both cards")
	expect(t, a, fed("ctr", "P2", "x", card.Gt, "3.5"), card.NotSame, "decider is empty on ref:K1")
}

func TestMatch_TitlesAndRulesAreIgnored(t *testing.T) {
	a := fed("ref", "K1", "Fed above 3.5%", card.Gt, "3.5")
	b := fed("ctr", "P1", "Fed above 4%", card.Gt, "3.5")
	a.Record.Title, b.Record.Title = b.Record.Title, a.Record.Title
	b.Record.RulesText = "completely different rules text"
	expect(t, a, b, card.Same)
	c := fed("ctr", "P2", a.Record.Title, card.Gt, "4")
	expect(t, a, c, card.NotSame, card.FieldCutoffValue)
}

func TestMatch_FirstFieldInOrderIsNamed(t *testing.T) {
	a := fed("ref", "K1", "x", card.Gt, "3.5")
	b := fed("ctr", "P1", "x", card.Gte, "3.5") // cutoff_op differs
	b.Payoff.Decider = card.Some("other")       // decider differs later in order
	expect(t, a, b, card.NotSame, card.FieldCutoffOp)
	c := fed("ctr", "P2", "x", card.Lte, "3.5") // complement, then a later difference
	c.Payoff.Deadline = card.Some(at("2027-01-28T19:00:00Z"))
	expect(t, a, c, card.NotSame, card.FieldDeadline)
}

func TestMatch_RunComparesWithinSubjectAndLogs(t *testing.T) {
	ctx := context.Background()
	st := testkit.TempStore(t)
	cfg := &card.Config{}
	cfg.Venues.Allowed = []string{"ref", "ctr"}
	cfg.Router.ReferenceVenue, cfg.Router.CounterVenue = "ref", "ctr"

	fetch, _ := st.Cycles.Begin(ctx, store.KindFetch, store.TriggerCLI)
	other := fed("ctr", "P9", "cpi", card.Gt, "3")
	other.Payoff.Subject = card.None[string]()
	for _, c := range []card.Card{
		fed("ref", "K1", "a", card.Gt, "3.5"),
		fed("ref", "K2", "b", card.Gt, "3.75"),
		fed("ctr", "P1", "c", card.Lte, "3.5"),
		other,
		fed("elsewhere", "Z1", "d", card.Gt, "3.5"),
	} {
		if _, err := st.Cards.Save(ctx, fetch, c); err != nil {
			t.Fatal(err)
		}
	}
	st.Cycles.Complete(ctx, fetch)

	cyc, _ := st.Cycles.Begin(ctx, store.KindMatch, store.TriggerCLI)
	sum, err := Run(ctx, st, cyc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	st.Cycles.Complete(ctx, cyc)
	if sum.Compared != 2 || sum.Opposite != 1 || sum.NotSame != 1 || sum.Skipped != 1 {
		t.Errorf("summary: %+v", sum)
	}
	recs, _ := st.Log.List(ctx, store.Completed)
	if len(recs) != 2 || recs[0].MarketA.Venue != "ref" || recs[0].MarketB.Venue != "ctr" {
		t.Errorf("log: %+v", recs)
	}
	counts, _ := st.Subjects.ForCycle(ctx, cyc)
	if len(counts) != 3 { // ("", ctr), (fed, ctr), (fed, ref)
		t.Errorf("subject counts: %+v", counts)
	}
	skips, _ := st.Skips.ForCycle(ctx, cyc)
	if len(skips) != 1 || !strings.Contains(skips[0].Reason, "unknown venue") {
		t.Errorf("skips: %+v", skips)
	}
}
