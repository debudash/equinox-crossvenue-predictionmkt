package card

import (
	"fmt"
	"strings"
	"time"
)

// Op is a cutoff comparison operator.
type Op string

const (
	Gt  Op = "gt"
	Gte Op = "gte"
	Lt  Op = "lt"
	Lte Op = "lte"
)

// ParseOp returns the operator for s, or false if s is not one of gt, gte, lt, lte.
func ParseOp(s string) (Op, bool) {
	switch op := Op(s); op {
	case Gt, Gte, Lt, Lte:
		return op, true
	}
	return "", false
}

// Complement returns the operator that is true exactly when o is false at the
// same value: gt <-> lte, gte <-> lt.
func (o Op) Complement() Op {
	switch o {
	case Gt:
		return Lte
	case Lte:
		return Gt
	case Gte:
		return Lt
	case Lt:
		return Gte
	}
	return ""
}

// UnclearResult is what a market pays if the result is unclear, cancelled or split.
type UnclearResult string

const (
	YesOrNoOnly UnclearResult = "yes_or_no_only"
	Half        UnclearResult = "half"
	LastPrice   UnclearResult = "last_price"
	Cancelled   UnclearResult = "cancelled"
)

// ParseUnclearResult returns Some for a known value and None otherwise, so a
// value outside the list is stored as empty.
func ParseUnclearResult(s string) Opt[UnclearResult] {
	switch u := UnclearResult(s); u {
	case YesOrNoOnly, Half, LastPrice, Cancelled:
		return Some(u)
	}
	return None[UnclearResult]()
}

// Instant is a moment in UTC held in canonical RFC 3339 form with trailing
// fractional zeros removed, e.g. "2026-12-10T19:00:00Z". Two Instants are ==
// exactly when they are the same moment.
type Instant struct{ s string }

// ParseInstant parses an RFC 3339 / ISO 8601 timestamp with an offset and
// converts it to UTC. A timestamp without an offset is rejected rather than
// guessed.
func ParseInstant(s string) (Instant, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return Instant{}, fmt.Errorf("deadline does not parse: %q", s)
	}
	return InstantOf(t), nil
}

// InstantOf converts a time to its canonical UTC Instant.
func InstantOf(t time.Time) Instant { return Instant{s: t.UTC().Format(time.RFC3339Nano)} }

// String returns the canonical form.
func (i Instant) String() string { return i.s }

// Time returns the instant as a time.Time in UTC.
func (i Instant) Time() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, i.s)
	return t
}

// Payoff holds the fields that decide whether two cards are the same bet.
// Every field is optional; an empty field never matches.
type Payoff struct {
	Subject       Opt[string] // vocab.yaml subjects
	CutoffOp      Opt[Op]
	CutoffValue   Opt[Decimal]
	CutoffUnit    Opt[string] // vocab.yaml cutoff_units
	Deadline      Opt[Instant]
	Decider       Opt[string] // vocab.yaml deciders
	UnclearResult Opt[UnclearResult]
}

// Payoff field names, in canonical order. Reasons in the log use these names.
const (
	FieldSubject       = "subject"
	FieldCutoffOp      = "cutoff_op"
	FieldCutoffValue   = "cutoff_value"
	FieldCutoffUnit    = "cutoff_unit"
	FieldDeadline      = "deadline"
	FieldDecider       = "decider"
	FieldUnclearResult = "unclear_result"
)

// Field is one payoff field in text form. Value is "" when Set is false.
type Field struct {
	Name  string
	Value string
	Set   bool
}

// Fields returns the payoff fields in canonical order as text. Two cards with
// equal payoff produce byte-equal Fields (test 16).
func (p Payoff) Fields() []Field {
	return []Field{
		textField(FieldSubject, p.Subject, func(v string) string { return v }),
		textField(FieldCutoffOp, p.CutoffOp, func(v Op) string { return string(v) }),
		textField(FieldCutoffValue, p.CutoffValue, Decimal.String),
		textField(FieldCutoffUnit, p.CutoffUnit, func(v string) string { return v }),
		textField(FieldDeadline, p.Deadline, Instant.String),
		textField(FieldDecider, p.Decider, func(v string) string { return v }),
		textField(FieldUnclearResult, p.UnclearResult, func(v UnclearResult) string { return string(v) }),
	}
}

func textField[T comparable](name string, o Opt[T], str func(T) string) Field {
	v, ok := o.Get()
	if !ok {
		return Field{Name: name}
	}
	return Field{Name: name, Value: str(v), Set: true}
}

// RecordFields are stored with the card for display and audit. They are never
// used to match.
type RecordFields struct {
	Title     string
	RulesText string // full settlement text, verbatim
	RawRef    string // id of the saved raw response this card came from
	// FetchedAt is the time of the raw response, not the time of normalization,
	// so rebuild reproduces it exactly.
	FetchedAt  time.Time
	EventTitle string
	Category   string
	Volume24h  string // decimal string as the venue reported it, display only
	URL        string
}

// Card is one market from one venue.
type Card struct {
	Venue    string
	MarketID string
	Payoff   Payoff
	Record   RecordFields
}

// Ref returns the card's market reference.
func (c Card) Ref() MarketRef { return MarketRef{Venue: c.Venue, MarketID: c.MarketID} }

// MarketRef names one market on one venue. Its text form is "venue:market_id".
type MarketRef struct {
	Venue    string
	MarketID string
}

func (r MarketRef) String() string {
	if r.Venue == "" && r.MarketID == "" {
		return ""
	}
	return r.Venue + ":" + r.MarketID
}

// ParseMarketRef splits "venue:market_id" on the first colon.
func ParseMarketRef(s string) (MarketRef, error) {
	v, id, ok := strings.Cut(s, ":")
	if !ok || v == "" || id == "" {
		return MarketRef{}, fmt.Errorf("not a market reference: %q", s)
	}
	return MarketRef{Venue: v, MarketID: id}, nil
}

// Side is yes or no.
type Side string

const (
	Yes Side = "yes"
	No  Side = "no"
)

// ParseSide accepts exactly "yes" or "no".
func ParseSide(s string) (Side, error) {
	switch Side(s) {
	case Yes, No:
		return Side(s), nil
	}
	return "", inconsistent("side %q is not yes or no", s)
}

// Other returns the opposite side.
func (s Side) Other() Side {
	if s == Yes {
		return No
	}
	return Yes
}

// PriceRecord is one saved price for one side of one market.
type PriceRecord struct {
	Venue    string
	MarketID string
	Side     Side
	BuyPrice Opt[Micros] // executable cost to buy this side; empty if unknown, never 0 or 1 as a stand-in
	Fee      Opt[Micros] // known fee per contract; empty if unknown
	PricedAt time.Time
}

// Result is the matcher's answer for a pair of cards.
type Result string

const (
	Same     Result = "same"
	Opposite Result = "opposite"
	NotSame  Result = "not_same"
)

// MatchResult is the matcher's output. There is no score or confidence:
// not_same is a finished answer.
type MatchResult struct {
	Result Result
	Reason string // for not_same, names the field that differed or was empty
}

// Action is the router's output.
type Action string

// NoRoute is the refusal action.
const NoRoute Action = "no_route"

// BuyOn returns the action for buying on a venue, e.g. "buy_on_" + the
// configured reference venue. Venue names come from config, never from code.
func BuyOn(venue string) Action { return Action("buy_on_" + venue) }

// RecordType distinguishes match records from route records in the decision log.
type RecordType string

const (
	MatchRecord RecordType = "match"
	RouteRecord RecordType = "route"
)

// LogRecord is one row of the append-only decision log. A refusal is recorded
// the same way as a choice.
type LogRecord struct {
	ID         int64 // set by the store
	CycleID    int64 // set by the store from the cycle passed to Append
	Type       RecordType
	DecidedAt  time.Time // set by the store when the record is written
	AsOf       time.Time // zero for match records
	MarketA    MarketRef // the reference venue card
	MarketB    MarketRef // the counter venue card
	Match      Result
	SideA      Side // empty for match records
	SideB      Side
	PriceA     Opt[Micros]
	PriceB     Opt[Micros]
	PricedAtA  time.Time // zero when no price was used
	PricedAtB  time.Time
	FeeApplied bool   // stored as yes or no on route records, empty on match records
	Action     Action // empty for match records
	Reason     string
}

// Roles names the venues by their role, from config/equinox.yaml. The reference
// venue's card states the requested side and wins exact cost ties.
type Roles struct {
	Reference string
	Counter   string
}
