package feed

import (
	"math/big"
	"sort"
	"strings"
	"time"

	"equinox/internal/card"
	"equinox/internal/feed/cache"
)

// Sort orders.
const (
	SortTop     = "top"     // by volume_24h, highest first
	SortClosing = "closing" // by the earliest deadline still ahead, soonest first
	SortMatched = "matched" // posts with a crosspost badge first, then by volume
)

// Post is one venue event (Reddit post). Its comments are the event's markets.
type Post struct {
	Key      string // venue plus event title; stable within a snapshot
	Venue    string
	Event    string
	Category string
	URL      string
	Score    string // summed volume_24h, or "-" when no market reported one
	Badges   []string
	Markets  []Market
	// PricesSaved is the newest priced_at among the post's markets, or "".
	PricesSaved string
	// OutOfVocab is true when no market in the post has a subject in the
	// shared vocabulary, so none of them can ever match.
	OutOfVocab bool

	volume  *big.Rat // nil when unknown
	closing time.Time
	priced  time.Time
}

// Market is one comment under a post.
type Market struct {
	Card    card.Card
	Yes, No Price
	Badge   string
	Fields  []card.Field
	Bot     []BotLine
	Others  []BotLine // not_same records not shown in Bot, folded away
	Refused bool      // the bot reply holds a refusal (not_same or no_route)
}

// Price is a saved buy price for one side, ready to show.
type Price struct {
	Value    string // "-" when unknown
	PricedAt string

	at time.Time
}

// BotLine is one decision log record shown verbatim under a market.
type BotLine struct {
	Label     string // Match, or the requested side of a route
	Code      string // match result or route action, verbatim
	Reason    string // verbatim
	Refused   bool   // not_same or no_route
	Other     string // the market on the other side of the pair
	DecidedAt string

	shared int // payoff fields set and equal on both cards, for display order
}

// sharedFields counts payoff fields that are set and equal on both cards.
func sharedFields(a, b card.Card) int {
	n := 0
	fb := b.Payoff.Fields()
	for i, f := range a.Payoff.Fields() {
		if f.Set && fb[i].Set && f.Value == fb[i].Value {
			n++
		}
	}
	return n
}

// buildPosts groups a snapshot's cards into posts in a fixed base order
// (venue, then event title), so paging through one snapshot is stable.
func buildPosts(s *cache.Snapshot) []*Post {
	byKey := map[string]*Post{}
	var order []string
	titles := map[card.MarketRef]string{}
	cards := map[card.MarketRef]card.Card{}
	for _, c := range s.Cards() {
		titles[c.Ref()] = c.Record.Title
		cards[c.Ref()] = c
	}
	for _, c := range s.Cards() {
		event := c.Record.EventTitle
		key := c.Venue + "|" + event
		if event == "" {
			// No event title: the market stands as its own post.
			event, key = c.Record.Title, c.Venue+"|market:"+c.MarketID
		}
		p := byKey[key]
		if p == nil {
			p = &Post{Key: key, Venue: c.Venue, Event: event, Category: c.Record.Category, URL: c.Record.URL}
			byKey[key] = p
			order = append(order, key)
		}
		p.Markets = append(p.Markets, buildMarket(s, c, titles, cards))
	}
	sort.Strings(order)
	posts := make([]*Post, 0, len(order))
	for _, k := range order {
		p := byKey[k]
		finishPost(p, s.CompletedAt)
		posts = append(posts, p)
	}
	return posts
}

func buildMarket(s *cache.Snapshot, c card.Card, titles map[card.MarketRef]string, cards map[card.MarketRef]card.Card) Market {
	m := Market{Card: c, Fields: c.Payoff.Fields()}
	m.Yes, m.No = price(s, c.Ref(), card.Yes), price(s, c.Ref(), card.No)
	for _, rec := range s.Matches(c.Ref()) {
		other := rec.MarketB
		if other == c.Ref() {
			other = rec.MarketA
		}
		name := other.String()
		if t := titles[other]; t != "" {
			name += " (" + t + ")"
		}
		line := BotLine{Label: "Match", Code: string(rec.Match), Reason: rec.Reason, Other: name,
			Refused: rec.Match == card.NotSame, DecidedAt: stamp(rec.DecidedAt), shared: sharedFields(c, cards[other])}
		if line.Refused {
			m.Others = append(m.Others, line)
			continue
		}
		if m.Badge == "" {
			m.Badge = badge(rec.Match, other.Venue)
		}
		m.Bot = append(m.Bot, line)
		// Routes are shown only while the pair's latest match is same or
		// opposite; an older route for a pair that no longer matches is stale.
		// Yes before no, as the wireframe shows; the log orders sides alphabetically.
		routes := append([]card.LogRecord(nil), s.Routes(rec.MarketA, rec.MarketB)...)
		sort.SliceStable(routes, func(i, j int) bool { return routes[i].SideA == card.Yes && routes[j].SideA != card.Yes })
		for _, rt := range routes {
			// The requested side is stated relative to the reference venue's
			// card (market_a); on the other card the label says so.
			label := capitalize(string(rt.SideA))
			if c.Ref() != rt.MarketA {
				label = VenueName(rt.MarketA.Venue) + " " + string(rt.SideA)
			}
			m.Bot = append(m.Bot, BotLine{Label: label, Code: string(rt.Action),
				Reason: rt.Reason, Refused: rt.Action == card.NoRoute, DecidedAt: stamp(rt.DecidedAt)})
		}
	}
	// Show the closest refusal first: the one whose two cards share the most
	// set payoff fields. This orders the display only; it decides nothing.
	sort.SliceStable(m.Others, func(i, j int) bool {
		return m.Others[i].shared > m.Others[j].shared
	})
	// With no same or opposite match, the first refusal is the reply and the
	// rest fold away, so a market compared with hundreds of others stays short.
	if len(m.Bot) == 0 && len(m.Others) > 0 {
		m.Bot, m.Others = m.Others[:1], m.Others[1:]
	}
	for _, b := range m.Bot {
		m.Refused = m.Refused || b.Refused
	}
	return m
}

func price(s *cache.Snapshot, ref card.MarketRef, side card.Side) Price {
	p, ok := s.Price(ref, side)
	if !ok {
		return Price{Value: "-", PricedAt: "-"}
	}
	out := Price{Value: "-", PricedAt: stamp(p.PricedAt), at: p.PricedAt}
	if v, ok := p.BuyPrice.Get(); ok {
		out.Value = v.String()
	}
	return out
}

// finishPost sets the score, badges and closing time from the post's markets.
func finishPost(p *Post, now time.Time) {
	seen := map[string]bool{}
	p.OutOfVocab = true
	for _, m := range p.Markets {
		if !m.Card.Payoff.Subject.IsEmpty() {
			p.OutOfVocab = false
		}
		for _, at := range []time.Time{m.Yes.at, m.No.at} {
			if at.After(p.priced) {
				p.priced = at
			}
		}
		if m.Badge != "" && !seen[m.Badge] {
			seen[m.Badge] = true
			p.Badges = append(p.Badges, m.Badge)
		}
		// volume_24h is a display string; it is summed only to rank posts.
		if v, ok := new(big.Rat).SetString(m.Card.Record.Volume24h); ok && m.Card.Record.Volume24h != "" {
			if p.volume == nil {
				p.volume = new(big.Rat)
			}
			p.volume.Add(p.volume, v)
		}
		// Closing soon counts only deadlines at or after the cycle completed.
		if d, ok := m.Card.Payoff.Deadline.Get(); ok {
			if t := d.Time(); !t.Before(now) && (p.closing.IsZero() || t.Before(p.closing)) {
				p.closing = t
			}
		}
	}
	if !p.priced.IsZero() {
		p.PricesSaved = stamp(p.priced)
	}
	p.Score = "-"
	if p.volume != nil {
		p.Score = strings.TrimSuffix(strings.TrimRight(p.volume.FloatString(2), "0"), ".")
	}
}

// Lead is the market shown on the post's card: the one with the highest
// volume_24h, else the first with a saved price, else the first.
func (p *Post) Lead() *Market {
	var best *Market
	var bestVol *big.Rat
	for i := range p.Markets {
		m := &p.Markets[i]
		if v, ok := new(big.Rat).SetString(m.Card.Record.Volume24h); ok && m.Card.Record.Volume24h != "" {
			if bestVol == nil || v.Cmp(bestVol) > 0 {
				best, bestVol = m, v
			}
		}
	}
	if best != nil {
		return best
	}
	for i := range p.Markets {
		if p.Markets[i].Yes.Value != "-" || p.Markets[i].No.Value != "-" {
			return &p.Markets[i]
		}
	}
	if len(p.Markets) > 0 {
		return &p.Markets[0]
	}
	return nil
}

// Matched reports whether any market in the post has a same or opposite match.
func (p *Post) Matched() bool { return len(p.Badges) > 0 }

// Closes returns the earliest deadline still ahead, or "-".
func (p *Post) Closes() string { return stamp(p.closing) }

// filterSort returns the posts that pass the filters, in the requested order.
// Ties fall back to the post key, so the order is total and paging never
// skips or repeats a post.
func filterSort(all []*Post, venue, category, order string) []*Post {
	var out []*Post
	for _, p := range all {
		if (venue == "" || p.Venue == venue) && (category == "" || p.Category == category) {
			out = append(out, p)
		}
	}
	byVolume := func(a, b *Post) int {
		switch {
		case a.volume == nil && b.volume == nil:
			return 0
		case a.volume == nil:
			return 1
		case b.volume == nil:
			return -1
		}
		return b.volume.Cmp(a.volume)
	}
	cmp := func(a, b *Post) int {
		switch order {
		case SortClosing:
			switch {
			case a.closing.Equal(b.closing):
			case a.closing.IsZero():
				return 1
			case b.closing.IsZero():
				return -1
			case a.closing.Before(b.closing):
				return -1
			default:
				return 1
			}
		case SortMatched:
			if ab, bb := len(a.Badges) > 0, len(b.Badges) > 0; ab != bb {
				if ab {
					return -1
				}
				return 1
			}
			return byVolume(a, b)
		default:
			return byVolume(a, b)
		}
		return 0
	}
	sort.SliceStable(out, func(i, j int) bool {
		if c := cmp(out[i], out[j]); c != 0 {
			return c < 0
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// badge is the crosspost badge text, e.g. "Same bet on polymarket". The venue
// name comes from the data, not from code.
func badge(r card.Result, otherVenue string) string {
	switch r {
	case card.Same:
		return "Same bet on " + VenueName(otherVenue)
	case card.Opposite:
		return "Opposite bet on " + VenueName(otherVenue)
	}
	return ""
}

// VenueName is a venue value for display, e.g. "kalshi" -> "Kalshi". It is
// derived from the data, not from a list of venues.
func VenueName(v string) string { return capitalize(v) }

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// stamp formats a time for display in UTC, or "-" when unknown.
func stamp(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}
