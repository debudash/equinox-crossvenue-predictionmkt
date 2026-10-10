package match

import (
	"context"
	"sort"

	"equinox/internal/card"
	"equinox/internal/store"
)

// Summary counts what one match run did.
type Summary struct {
	Compared, Same, Opposite, NotSame, Skipped int
	Counts                                     []store.SubjectCount
}

// Run compares every reference-venue card with every counter-venue card that
// has the same non-empty subject, and appends one match record per pair. It
// saves the card count per subject (and venue) on every run. Cards that fail
// validation are skipped and logged; they never stop the run.
func Run(ctx context.Context, st *store.Store, cycleID int64, cfg *card.Config) (Summary, error) {
	var sum Summary
	cards, err := st.Cards.LatestAll(ctx, store.Writing(cycleID))
	if err != nil {
		return sum, err
	}
	roles := cfg.Roles()
	counts := map[store.SubjectCount]int{}
	bySubject := map[string]map[string][]card.Card{} // subject -> venue -> cards
	for _, c := range cards {
		if err := card.CheckCard(c, cfg.Venues.Allowed); err != nil {
			sum.Skipped++
			if err := st.Skips.Append(ctx, store.Skip{CycleID: cycleID, Stage: store.StageMatch, Venue: c.Venue, MarketID: c.MarketID, Reason: err.Error()}); err != nil {
				return sum, err
			}
			continue
		}
		subject, _ := c.Payoff.Subject.Get()
		counts[store.SubjectCount{Subject: subject, Venue: c.Venue}]++
		if subject == "" {
			continue
		}
		if bySubject[subject] == nil {
			bySubject[subject] = map[string][]card.Card{}
		}
		bySubject[subject][c.Venue] = append(bySubject[subject][c.Venue], c)
	}
	for k, n := range counts {
		k.Count = n
		sum.Counts = append(sum.Counts, k)
	}
	sort.Slice(sum.Counts, func(i, j int) bool {
		a, b := sum.Counts[i], sum.Counts[j]
		return a.Subject < b.Subject || a.Subject == b.Subject && a.Venue < b.Venue
	})
	if err := st.Subjects.Append(ctx, cycleID, sum.Counts); err != nil {
		return sum, err
	}

	subjects := make([]string, 0, len(bySubject))
	for s := range bySubject {
		subjects = append(subjects, s)
	}
	sort.Strings(subjects)
	for _, s := range subjects {
		for _, a := range bySubject[s][roles.Reference] {
			for _, b := range bySubject[s][roles.Counter] {
				r := Compare(a, b)
				rec := card.LogRecord{Type: card.MatchRecord, MarketA: a.Ref(), MarketB: b.Ref(), Match: r.Result, Reason: r.Reason}
				if _, err := st.Log.Append(ctx, cycleID, rec); err != nil {
					return sum, err
				}
				sum.Compared++
				switch r.Result {
				case card.Same:
					sum.Same++
				case card.Opposite:
					sum.Opposite++
				default:
					sum.NotSame++
				}
			}
		}
	}
	return sum, nil
}
