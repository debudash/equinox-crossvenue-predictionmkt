package kalshi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"equinox/internal/card"
	"equinox/internal/store"
)

// seriesRule holds the payoff choices for one Kalshi series. Only series listed
// in fedSeries are fetched, and only their markets get a subject, a decider and
// a unit. Every entry is documented in NORMALIZATION.md.
type seriesRule struct {
	subject string // vocab subject, set only when rules_primary contains subjectPhrase
	unit    string // vocab cutoff unit
	// deadline picks the venue time that is the moment the outcome is read.
	deadline func(ev wireEvent, m wireMarket) string
	// unclear is set only from the series' filed contract terms (the API rules
	// text never says). Empty means the terms leave payouts to Kalshi's discretion.
	unclear card.Opt[card.UnclearResult]
}

// subjectPhrase must appear in rules_primary before the subject is set, so a
// series that one day lists a different rate never inherits the subject.
const subjectPhrase = "upper bound of the target"

// fedSeries is the V1 scope: Federal Reserve rate markets only.
var fedSeries = map[string]seriesRule{
	// KXFED: "upper bound ... following the Fed's <date> meeting". The outcome
	// is read when the FOMC statement is released. The event strike_date is
	// that moment (2:00 PM ET, e.g. 2026-12-09T19:00:00Z), while the market
	// close_time is 5 minutes earlier and expected_expiration_time 5 minutes
	// later, so strike_date is the deadline.
	//
	// unclear_result: the series' contract terms (contract_terms_url
	// https://assets.kalshi.com/contract_terms/FED.pdf, Rule 100.5, Payout
	// Criterion) say "If no data is available on the Expiration Date at the
	// Expiration Time, the Contract resolves to No." The contract pays $1 or $0
	// only, so yes_or_no_only.
	"KXFED": {subject: "fed_funds_upper_bound", unit: "percent",
		deadline: func(ev wireEvent, _ wireMarket) string { return ev.StrikeDate },
		unclear:  card.Some(card.YesOrNoOnly)},
	// KXFEDFUNDSYEAR: "upper bound ... in effect at 11:59 PM ET on December 31".
	// The event has no strike_date; the market close_time is exactly that
	// moment (e.g. 2028-01-01T04:59:00Z), so close_time is the deadline.
	//
	// unclear_result: empty. The series' contract terms
	// (https://assets.kalshi.com/contract_terms/ECONSTAT.pdf, Contingencies) say
	// "If an Expiration Value cannot be determined on the Expiration Date,
	// Kalshi has the right to determine payouts pursuant to Rule 6.3(b)", so
	// the payout in that case is discretionary, not yes or no only.
	"KXFEDFUNDSYEAR": {subject: "fed_funds_upper_bound", unit: "percent",
		deadline: func(_ wireEvent, m wireMarket) string { return m.CloseTime }},
}

// deciders maps a Kalshi settlement source name to a vocab decider. A source
// not listed here leaves the decider empty.
var deciders = map[string]string{
	"Federal Reserve Board of Governors": "federal_reserve",
}

// strikeOps maps Kalshi strike_type to a cutoff operator and says which strike
// field holds the threshold. Any other strike_type (between, structured,
// custom, ...) leaves cutoff_op and cutoff_value empty.
var strikeOps = map[string]struct {
	op    card.Op
	floor bool // true: floor_strike, false: cap_strike
}{
	"greater":          {card.Gt, true},
	"greater_or_equal": {card.Gte, true},
	"less":             {card.Lt, false},
	"less_or_equal":    {card.Lte, false},
}

type eventsPage struct {
	Cursor string            `json:"cursor"`
	Events []json.RawMessage `json:"events"`
}

type wireEvent struct {
	EventTicker       string `json:"event_ticker"`
	SeriesTicker      string `json:"series_ticker"`
	Title             string `json:"title"`
	Category          string `json:"category"`
	StrikeDate        string `json:"strike_date"`
	SettlementSources []struct {
		Name string `json:"name"`
	} `json:"settlement_sources"`
	Markets []json.RawMessage `json:"markets"`
}

type wireMarket struct {
	Ticker         string `json:"ticker"`
	EventTicker    string `json:"event_ticker"`
	MarketType     string `json:"market_type"`
	Title          string `json:"title"`
	RulesPrimary   string `json:"rules_primary"`
	RulesSecondary string `json:"rules_secondary"`
	StrikeType     string `json:"strike_type"`
	// Strikes are read as json.Number, never float64, so "3.75" stays exact.
	FloorStrike json.Number `json:"floor_strike"`
	CapStrike   json.Number `json:"cap_strike"`
	CloseTime   string      `json:"close_time"`
	YesAsk      string      `json:"yes_ask_dollars"`
	NoAsk       string      `json:"no_ask_dollars"`
	Volume24h   string      `json:"volume_24h_fp"`
	// Either field marks a multivariate combo market.
	MveCollectionTicker string          `json:"mve_collection_ticker"`
	MveSelectedLegs     json.RawMessage `json:"mve_selected_legs"`
}

// decodeStrict decodes JSON keeping numbers as json.Number.
func decodeStrict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	return d.Decode(v)
}

func parseEventsPage(body []byte) (eventsPage, error) {
	var p eventsPage
	if err := decodeStrict(body, &p); err != nil {
		return eventsPage{}, fmt.Errorf("events response does not parse: %w", err)
	}
	return p, nil
}

// NormalizeMarkets turns one saved events response (with nested markets) into
// cards, price records and skips. It is pure: it reads only raw and vocab, so
// rebuild reproduces the same cards. A response that does not parse at all
// yields one skip naming the raw response.
func NormalizeMarkets(raw store.Raw, vocab *card.Vocab) (cards []card.Card, prices []card.PriceRecord, skips []store.Skip) {
	cards, prices, skips, _, err := normalizeMarkets(raw, vocab)
	if err != nil {
		skips = append(skips, skipOf(raw, "", fmt.Sprintf("raw response %d: %v", raw.ID, err)))
	}
	return cards, prices, skips
}

// normalizeMarkets also returns the page cursor, for pagination.
func normalizeMarkets(raw store.Raw, vocab *card.Vocab) (cards []card.Card, prices []card.PriceRecord, skips []store.Skip, cursor string, err error) {
	page, err := parseEventsPage(raw.Body)
	if err != nil {
		return nil, nil, nil, "", err
	}
	for i, evRaw := range page.Events {
		var ev wireEvent
		if err := decodeStrict(evRaw, &ev); err != nil {
			// A malformed event drops only its own markets.
			skips = append(skips, skipOf(raw, "", fmt.Sprintf("event %d does not parse: %v", i, err)))
			continue
		}
		for j, mRaw := range ev.Markets {
			var m wireMarket
			if err := decodeStrict(mRaw, &m); err != nil {
				skips = append(skips, skipOf(raw, "", fmt.Sprintf("event %s market %d does not parse: %v", ev.EventTicker, j, err)))
				continue
			}
			c, ps, reason := normalizeMarket(raw, vocab, ev, m)
			if reason != "" {
				skips = append(skips, skipOf(raw, m.Ticker, reason))
				continue
			}
			cards = append(cards, c)
			prices = append(prices, ps...)
		}
	}
	return cards, prices, skips, page.Cursor, nil
}

// normalizeMarket returns a card and its two price records, or a skip reason.
func normalizeMarket(raw store.Raw, vocab *card.Vocab, ev wireEvent, m wireMarket) (card.Card, []card.PriceRecord, string) {
	if reason := checkMarket(m); reason != "" {
		return card.Card{}, nil, reason
	}
	ps, err := marketPrices(m, raw.FetchedAt)
	if err != nil {
		return card.Card{}, nil, err.Error()
	}
	c := card.Card{
		Venue:    Venue,
		MarketID: m.Ticker,
		Record: card.RecordFields{
			Title: m.Title,
			// rules_text is rules_primary then rules_secondary, verbatim,
			// separated by one blank line.
			RulesText:  joinRules(m.RulesPrimary, m.RulesSecondary),
			RawRef:     strconv.FormatInt(raw.ID, 10),
			FetchedAt:  raw.FetchedAt,
			EventTitle: ev.Title,
			Category:   ev.Category,
			Volume24h:  m.Volume24h,
			URL:        marketURL(ev.SeriesTicker),
		},
	}
	p := &c.Payoff

	// Cutoff: operator from strike_type, value from the matching strike field.
	if rule, ok := strikeOps[m.StrikeType]; ok {
		p.CutoffOp = card.Some(rule.op)
		strike := m.CapStrike
		if rule.floor {
			strike = m.FloorStrike
		}
		// The strike is a bare JSON number in percent, e.g. 3.75. A value
		// that is not a plain decimal (exponent, missing) stays empty.
		if d, err := card.ParseDecimal(strike.String()); err == nil {
			p.CutoffValue = card.Some(d)
		}
	}

	// Decider: every listed settlement source must map to the same decider.
	p.Decider = vocab.Decider(decider(ev))

	rule, inScope := fedSeries[ev.SeriesTicker]
	if !inScope {
		// Out of scope series: no subject, unit, deadline or unclear_result rule exists.
		return c, ps, ""
	}
	// unclear_result comes from the series' contract terms, not the API rules
	// text, which never covers it. See fedSeries and NORMALIZATION.md.
	p.UnclearResult = rule.unclear
	if strings.Contains(strings.ToLower(m.RulesPrimary), subjectPhrase) {
		p.Subject = vocab.Subject(rule.subject)
	}
	p.CutoffUnit = vocab.CutoffUnit(rule.unit)
	if s := rule.deadline(ev, m); s != "" {
		d, err := card.CheckDeadline(s)
		if err != nil {
			// A deadline that does not parse makes the record inconsistent.
			return card.Card{}, nil, err.Error()
		}
		p.Deadline = card.Some(d)
	}
	return c, ps, ""
}

// checkMarket returns a skip reason for a market that must not become a card.
func checkMarket(m wireMarket) string {
	switch {
	case m.Ticker == "":
		return "market has no ticker"
	case m.MveCollectionTicker != "" || isPresent(m.MveSelectedLegs):
		return "multivariate combo market is out of scope"
	case m.MarketType != "binary":
		return fmt.Sprintf("market_type %q is not binary", m.MarketType)
	}
	return ""
}

func isPresent(r json.RawMessage) bool {
	s := strings.TrimSpace(string(r))
	return s != "" && s != "null" && s != "[]"
}

// marketPrices reads the executable asks. Buying yes costs yes_ask_dollars;
// buying no costs no_ask_dollars (Kalshi derives it from the best yes bid, so
// it is executable against resting yes bids). Both are dollar strings.
func marketPrices(m wireMarket, at time.Time) ([]card.PriceRecord, error) {
	yes, err := askPrice(m.YesAsk)
	if err != nil {
		return nil, fmt.Errorf("yes_ask_dollars: %w", err)
	}
	no, err := askPrice(m.NoAsk)
	if err != nil {
		return nil, fmt.Errorf("no_ask_dollars: %w", err)
	}
	// Fee is empty: see NORMALIZATION.md "Fees".
	return []card.PriceRecord{
		{Venue: Venue, MarketID: m.Ticker, Side: card.Yes, BuyPrice: yes, PricedAt: at},
		{Venue: Venue, MarketID: m.Ticker, Side: card.No, BuyPrice: no, PricedAt: at},
	}, nil
}

// askPrice parses an ask. Kalshi reports an empty ask side as "1.0000" (and an
// empty bid side as "0.0000", which makes the other side's ask "1.0000"), so 1
// and 0 mean no executable ask and are stored as empty, never as a price. A
// value that does not parse or lies outside 0 to 1 is an error.
func askPrice(s string) (card.Opt[card.Micros], error) {
	if s == "" {
		return card.None[card.Micros](), nil
	}
	v, err := card.ParseMicros(s)
	if err != nil {
		return card.None[card.Micros](), fmt.Errorf("%w: %v", card.ErrInconsistent, err)
	}
	if err := card.CheckPrice(v); err != nil {
		return card.None[card.Micros](), err
	}
	if v == 0 || v == card.OneDollar {
		return card.None[card.Micros](), nil
	}
	return card.Some(v), nil
}

func decider(ev wireEvent) string {
	out := ""
	for _, s := range ev.SettlementSources {
		d, ok := deciders[s.Name]
		if !ok || (out != "" && d != out) {
			return ""
		}
		out = d
	}
	return out
}

func joinRules(primary, secondary string) string {
	if secondary == "" {
		return primary
	}
	return primary + "\n\n" + secondary
}

// marketURL is display only: the series page on kalshi.com.
func marketURL(series string) string {
	if series == "" {
		return ""
	}
	return "https://kalshi.com/markets/" + strings.ToLower(series)
}

func skipOf(raw store.Raw, marketID, reason string) store.Skip {
	// Stage is fetch; rebuild may restamp it as rebuild.
	return store.Skip{CycleID: raw.CycleID, RunID: raw.RunID, Stage: store.StageFetch, Venue: Venue, MarketID: marketID, Reason: reason}
}

type marketsPage struct {
	Cursor  string            `json:"cursor"`
	Markets []json.RawMessage `json:"markets"`
}

// NormalizePrices turns one saved price refresh (GET /markets?tickers=...) into
// price records. It is pure, like NormalizeMarkets. A response that does not
// parse yields one skip.
func NormalizePrices(raw store.Raw) ([]card.PriceRecord, []store.Skip) {
	prices, skips, err := normalizePrices(raw)
	if err != nil {
		skips = append(skips, skipOf(raw, "", fmt.Sprintf("raw response %d: %v", raw.ID, err)))
	}
	return prices, skips
}

func normalizePrices(raw store.Raw) (prices []card.PriceRecord, skips []store.Skip, err error) {
	var page marketsPage
	if err := decodeStrict(raw.Body, &page); err != nil {
		return nil, nil, fmt.Errorf("markets response does not parse: %w", err)
	}
	for i, mRaw := range page.Markets {
		var m wireMarket
		if err := decodeStrict(mRaw, &m); err != nil {
			skips = append(skips, skipOf(raw, "", fmt.Sprintf("market %d does not parse: %v", i, err)))
			continue
		}
		if reason := checkMarket(m); reason != "" {
			skips = append(skips, skipOf(raw, m.Ticker, reason))
			continue
		}
		ps, err := marketPrices(m, raw.FetchedAt)
		if err != nil {
			skips = append(skips, skipOf(raw, m.Ticker, err.Error()))
			continue
		}
		prices = append(prices, ps...)
	}
	return prices, skips, nil
}
