package polymarket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // the ET to UTC conversion must not depend on the host's zone database

	"equinox/internal/card"
	"equinox/internal/store"
)

// Venue is the card venue value. Polymarket US keeps the value "polymarket"
// (amendment A9).
const Venue = "polymarket"

// marketsPage is GET /v1/markets. Each market is decoded on its own so one
// broken record is skipped without losing the page.
type marketsPage struct {
	Markets []json.RawMessage `json:"markets"`
}

// market holds the Polymarket US market fields this fetcher reads. Numbers are
// json.Number, never float64 (AGENTS.md hard rule 5).
type market struct {
	ID          string       `json:"id"`
	Slug        string       `json:"slug"`
	Question    string       `json:"question"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Category    string       `json:"category"`
	Volume24hr  json.Number  `json:"volume24hr"`
	MarketSides []marketSide `json:"marketSides"`
}

type marketSide struct {
	Description string `json:"description"`
	Long        *bool  `json:"long"`
}

// slugOnly recovers the slug of a record that does not decode, for the skip reason.
type slugOnly struct {
	Slug string `json:"slug"`
}

// NormalizeMarkets turns one saved GET /v1/markets response into cards and
// skips. It is pure: no clock, no network, so rebuild reproduces the same
// cards from the same raw response.
//
// Prices: the markets payload carries no executable ask. marketSides[].price
// and outcomePrices are display prices, so this returns no price records;
// prices come only from the order book (NormalizePrices).
func NormalizeMarkets(raw store.Raw, vocab *card.Vocab) (cards []card.Card, prices []card.PriceRecord, skips []store.Skip) {
	cards, skips, _, _, err := normalizeMarkets(raw, vocab)
	if err != nil {
		skips = append(skips, skip(raw, "", err.Error()))
	}
	return cards, nil, skips
}

// normalizeMarkets also reports how many records the page held (for
// pagination), how many were outside V1 scope (summarized in one skip), and
// whether the page itself failed to parse.
func normalizeMarkets(raw store.Raw, vocab *card.Vocab) (cards []card.Card, skips []store.Skip, n, outOfScope int, err error) {
	var page marketsPage
	if err := json.Unmarshal(raw.Body, &page); err != nil {
		return nil, nil, 0, 0, fmt.Errorf("markets response %s does not parse: %v", raw.Endpoint, err)
	}
	for _, rec := range page.Markets {
		var m market
		dec := json.NewDecoder(bytes.NewReader(rec))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			// Malformed record: skip, count, log, continue (hard rule 8).
			var s slugOnly
			_ = json.Unmarshal(rec, &s)
			skips = append(skips, skip(raw, s.Slug, "malformed market record: "+err.Error()))
			continue
		}
		// V1 scope: only markets whose rules mention the federal funds rate.
		// Everything else on the macro page is counted in one summary skip below.
		if !fedScope.MatchString(m.Description) {
			outOfScope++
			continue
		}
		c, reason := normalizeMarket(m, raw, vocab)
		if reason != "" {
			skips = append(skips, skip(raw, m.Slug, reason))
			continue
		}
		cards = append(cards, c)
	}
	// One summary skip per page keeps the skips table readable; the fetch run's
	// skipped count still includes every out-of-scope record (see Fetch).
	if outOfScope > 0 {
		skips = append(skips, skip(raw, "", fmt.Sprintf("%d %s", outOfScope, outOfScopeReason)))
	}
	return cards, skips, len(page.Markets), outOfScope, nil
}

const outOfScopeReason = "markets outside V1 scope (rules do not mention the federal funds rate)"

var fedScope = regexp.MustCompile(`(?i)federal funds rate`)

// normalizeMarket builds one card. It returns a non-empty reason when the
// record cannot become a card at all.
func normalizeMarket(m market, raw store.Raw, vocab *card.Vocab) (card.Card, string) {
	// market_id is the slug: Polymarket US uses the slug everywhere (book, BBO,
	// orders), so the price loop can refresh a card from its market_id alone.
	if m.Slug == "" {
		return card.Card{}, "market record has no slug"
	}
	// YES and NO are identified by the side label, never by array position.
	// Polymarket US has one instrument per market: the long side is YES and
	// the order book prices refer to it. A market whose long side is not
	// labeled "Yes" and short side "No" (for example a sports market labeled
	// by team) is skipped, because its book could not be read as YES and NO.
	if !yesNoSides(m.MarketSides) {
		return card.Card{}, "market sides are not labeled Yes (long) and No (short)"
	}
	rules := m.Description
	c := card.Card{
		Venue:    Venue,
		MarketID: m.Slug,
		Payoff: card.Payoff{
			Subject:       subjectOf(rules, vocab),
			Deadline:      deadlineOf(rules),
			Decider:       deciderOf(rules, vocab),
			UnclearResult: unclearResultOf(rules),
		},
		Record: card.RecordFields{
			Title:      firstNonEmpty(m.Title, m.Question),
			RulesText:  rules,
			RawRef:     strconv.FormatInt(raw.ID, 10),
			FetchedAt:  raw.FetchedAt,
			EventTitle: m.Question,
			Category:   m.Category,
			// volume24hr is shares, display only; kept as the venue's decimal text.
			Volume24h: m.Volume24hr.String(),
			// No URL: the public API returns no page link and the market slug path
			// redirects to the home page. Empty rather than invented.
		},
	}
	c.Payoff.CutoffOp, c.Payoff.CutoffValue, c.Payoff.CutoffUnit = cutoffOf(rules, vocab)
	return c, ""
}

func yesNoSides(sides []marketSide) bool {
	var yes, no bool
	for _, s := range sides {
		if s.Long == nil {
			return false
		}
		switch {
		case *s.Long && strings.EqualFold(s.Description, "Yes"):
			yes = true
		case !*s.Long && strings.EqualFold(s.Description, "No"):
			no = true
		default:
			return false
		}
	}
	return yes && no
}

// subjectOf: fed_funds_upper_bound when the rules are about the upper bound of
// the target federal funds rate (threshold, decision-bucket and change markets
// all qualify; only threshold markets also get a cutoff).
var upperBound = regexp.MustCompile(`(?i)upper bound of the target (range for the )?federal funds rate`)

func subjectOf(rules string, vocab *card.Vocab) card.Opt[string] {
	if !upperBound.MatchString(rules) {
		return card.None[string]()
	}
	return vocab.Subject("fed_funds_upper_bound")
}

// deciderOf: Polymarket US Fed rules end with "Outcome sourced from the
// (United States) Federal Reserve." That maps to federal_reserve. Any other
// source (for example a regional Federal Reserve Bank) is not mapped and stays
// empty, so it never matches.
var fedSource = regexp.MustCompile(`Outcome sourced from the (United States )?Federal Reserve\.`)

func deciderOf(rules string, vocab *card.Vocab) card.Opt[string] {
	if !fedSource.MatchString(rules) {
		return card.None[string]()
	}
	return vocab.Decider("federal_reserve")
}

// cutoffOf reads a threshold proposition: "the upper bound ... is <op> X%".
// Exactly one such sentence must appear. Decision buckets ("does not change",
// "decreases ... by more than 25 basis points") have no such sentence, so their
// cutoff stays empty and they never match.
var threshold = regexp.MustCompile(`(?i)federal funds rate is (greater than or equal to|at or above|greater than|above|less than or equal to|at or below|less than|below) (\d+(?:\.\d+)?)%`)

var opWords = map[string]card.Op{
	"greater than or equal to": card.Gte,
	"at or above":              card.Gte,
	"greater than":             card.Gt,
	"above":                    card.Gt,
	"less than or equal to":    card.Lte,
	"at or below":              card.Lte,
	"less than":                card.Lt,
	"below":                    card.Lt,
}

func cutoffOf(rules string, vocab *card.Vocab) (card.Opt[card.Op], card.Opt[card.Decimal], card.Opt[string]) {
	ms := threshold.FindAllStringSubmatch(rules, -1)
	if len(ms) != 1 {
		return card.None[card.Op](), card.None[card.Decimal](), card.None[string]()
	}
	op, ok := opWords[strings.ToLower(ms[0][1])]
	v, err := card.ParseDecimal(ms[0][2])
	if !ok || err != nil {
		return card.None[card.Op](), card.None[card.Decimal](), card.None[string]()
	}
	return card.Some(op), card.Some(v), vocab.CutoffUnit("percent")
}

// deadlineOf: the deadline is the moment the rules say the outcome is read,
// written as "<Month> <D>, <YYYY> at <h:mm> AM|PM ET" (or with a comma instead
// of "at"). ET is America/New_York, so daylight saving is applied for that
// date. Exactly one distinct moment must appear; otherwise the deadline is
// empty. endDate and gameStartTime are not used: endDate is a settlement
// window end (two weeks later), not the reading moment.
var etMoment = regexp.MustCompile(`(January|February|March|April|May|June|July|August|September|October|November|December) (\d{1,2}), (\d{4})(?:,| at) (\d{1,2}):(\d{2}) (AM|PM) ET`)

var newYork = mustLoad("America/New_York")

func mustLoad(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

func deadlineOf(rules string) card.Opt[card.Instant] {
	seen := map[string]bool{}
	var out card.Instant
	for _, m := range etMoment.FindAllStringSubmatch(rules, -1) {
		t, err := time.ParseInLocation("January 2 2006 3:04 PM", fmt.Sprintf("%s %s %s %s:%s %s", m[1], m[2], m[3], m[4], m[5], m[6]), newYork)
		if err != nil {
			return card.None[card.Instant]()
		}
		i, err := card.ParseInstant(t.Format(time.RFC3339))
		if err != nil {
			return card.None[card.Instant]()
		}
		seen[i.String()] = true
		out = i
	}
	if len(seen) != 1 {
		return card.None[card.Instant]()
	}
	return card.Some(out)
}

// unclearResultOf (owner decision D-20): a payoff field is filled only from a
// source tied to that contract. The market's own rules text is such a source,
// so one explicit sentence saying the market resolves to Yes or No when the
// outcome cannot be determined gives yes_or_no_only. Anything else is empty.
// Real Polymarket US Fed rules (2026-10-10) have no such sentence, and no
// Fed-specific clause was found in the filed contract terms or the PMUS
// Rulebook; the venue's general docs ($1 or $0) are not contract specific.
var binaryFallback = regexp.MustCompile(`(?i)if the outcome cannot be determined[^.]*\bresolves? to (yes|no)\b`)

func unclearResultOf(rules string) card.Opt[card.UnclearResult] {
	if binaryFallback.MatchString(rules) {
		return card.Some(card.YesOrNoOnly)
	}
	return card.None[card.UnclearResult]()
}

// bookResponse is GET /v1/markets/{slug}/book.
type bookResponse struct {
	MarketData *struct {
		MarketSlug string      `json:"marketSlug"`
		Bids       []bookEntry `json:"bids"`
		Offers     []bookEntry `json:"offers"`
		State      string      `json:"state"`
	} `json:"marketData"`
}

type bookEntry struct {
	Px struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"px"`
	Qty string `json:"qty"`
}

// NormalizePrices turns one saved order book response into one price record per
// side. priced_at is the raw response's fetched_at, so it is reproducible.
//
// The book is the book of the single Polymarket US instrument, whose long side
// is labeled YES (checked on the card); all book prices refer to YES.
//   - YES buy_price = the lowest offer: buying YES lifts the best offer.
//   - NO buy_price = 1 - the highest bid: buying NO is selling YES at the best
//     bid, which costs 1 minus that bid per contract.
//
// Prices are decimal dollar strings (e.g. "0.2700", currency USD), parsed exactly
// into micro-dollars; the 1 - bid step is integer math. An empty side of the book
// gives an empty buy_price, never 0 or 1. A book whose state is not
// MARKET_STATE_OPEN gives empty prices on both sides: nothing is executable.
// Fee is empty: see NORMALIZATION.md.
func NormalizePrices(raw store.Raw) ([]card.PriceRecord, []store.Skip) {
	var b bookResponse
	if err := json.Unmarshal(raw.Body, &b); err != nil || b.MarketData == nil || b.MarketData.MarketSlug == "" {
		reason := "book response has no marketData.marketSlug"
		if err != nil {
			reason = "book response does not parse: " + err.Error()
		}
		return nil, []store.Skip{skip(raw, "", reason)}
	}
	md := b.MarketData
	var skips []store.Skip
	yes, no := card.None[card.Micros](), card.None[card.Micros]()
	if md.State == "MARKET_STATE_OPEN" {
		ask, err := bestLevel(md.Offers, func(a, b card.Micros) bool { return a < b })
		if err != nil {
			skips = append(skips, skip(raw, md.MarketSlug, "yes price: "+err.Error()))
		} else {
			yes = ask
		}
		bid, err := bestLevel(md.Bids, func(a, b card.Micros) bool { return a > b })
		if err != nil {
			skips = append(skips, skip(raw, md.MarketSlug, "no price: "+err.Error()))
		} else if v, ok := bid.Get(); ok {
			no = card.Some(card.OneDollar - v)
		}
	}
	rec := func(s card.Side, p card.Opt[card.Micros]) card.PriceRecord {
		return card.PriceRecord{Venue: Venue, MarketID: md.MarketSlug, Side: s, BuyPrice: p, PricedAt: raw.FetchedAt}
	}
	return []card.PriceRecord{rec(card.Yes, yes), rec(card.No, no)}, skips
}

// bestLevel returns the best price among levels with a positive quantity, or
// empty when there is none. Any malformed level makes the whole side unknown.
func bestLevel(levels []bookEntry, better func(a, b card.Micros) bool) (card.Opt[card.Micros], error) {
	var best card.Micros
	found := false
	for _, l := range levels {
		if l.Px.Currency != "USD" {
			return card.None[card.Micros](), fmt.Errorf("price currency %q is not USD", l.Px.Currency)
		}
		p, err := card.ParseMicros(l.Px.Value)
		if err != nil {
			return card.None[card.Micros](), err
		}
		if err := card.CheckPrice(p); err != nil {
			return card.None[card.Micros](), err
		}
		q, err := card.ParseDecimal(l.Qty)
		if err != nil || strings.HasPrefix(q.String(), "-") {
			return card.None[card.Micros](), fmt.Errorf("quantity %q is not a non-negative decimal", l.Qty)
		}
		if q.String() == "0" {
			continue // an empty level is not executable
		}
		if !found || better(p, best) {
			best, found = p, true
		}
	}
	if !found {
		return card.None[card.Micros](), nil
	}
	return card.Some(best), nil
}

func skip(raw store.Raw, marketID, reason string) store.Skip {
	return store.Skip{CycleID: raw.CycleID, RunID: raw.RunID, Stage: store.StageFetch, Venue: Venue, MarketID: marketID, Reason: reason}
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
