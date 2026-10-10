package polymarket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"equinox/internal/card"
	"equinox/internal/store"
)

// Display-only scope. Besides the Fed markets on the macro page, the fetcher
// can save the first markets of a few other Polymarket US categories so the
// feed shows a real multi-category venue. These cards are for display only:
// every payoff field except a display deadline stays empty, so the matcher
// never pairs them (see displayCard and NORMALIZATION.md "Display-only
// categories").
//
// The list lives in config/equinox.yaml under
// fetchers.polymarket.display_categories. card.FetcherConfig is a frozen
// contract that holds only source and base_url, so this package reads its own
// section from the same YAML file (ParseDisplay); registry.LoadDisplay hands
// the result to the fetcher. No list configured means no display fetch, which
// is what every test and the acceptance harness get.

const (
	// fedCategory is the page the Fed path reads. It cannot be a display category.
	fedCategory = "macro"
	// defaultDisplayLimit is markets per display category when limit is absent.
	defaultDisplayLimit = 10
	// displayRetryAfter is the extra wait before a display page's last try
	// after HTTP 429; the gateway sent Retry-After: 10 when measured.
	displayRetryAfter = 10 * time.Second
)

// DisplayCategory is one display-only Polymarket US category.
type DisplayCategory struct {
	// Category is the venue's category value, e.g. politics.
	Category string `yaml:"category"`
	// Limit is markets requested, one page, 1 to 100 (default 10). The API
	// returns no volume and ignores orderBy (checked 2026-10-10), so these are
	// the first markets in the venue's own listing order.
	Limit int `yaml:"limit"`
}

// ParseDisplay reads fetchers.polymarket.display_categories from config YAML
// and applies defaults. A missing section gives an empty list.
func ParseDisplay(b []byte) ([]DisplayCategory, error) {
	var doc struct {
		Fetchers struct {
			Polymarket struct {
				Display []DisplayCategory `yaml:"display_categories"`
			} `yaml:"polymarket"`
		} `yaml:"fetchers"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("config: fetchers.polymarket.display_categories: %w", err)
	}
	out := doc.Fetchers.Polymarket.Display
	for i := range out {
		d := &out[i]
		if d.Limit == 0 {
			d.Limit = defaultDisplayLimit
		}
		switch {
		case d.Category == "":
			return nil, fmt.Errorf("config: polymarket display category %d has no category", i)
		case strings.EqualFold(d.Category, fedCategory):
			return nil, fmt.Errorf("config: polymarket display category %q is the Fed scope page", d.Category)
		case d.Limit < 0 || d.Limit > pageSize:
			return nil, fmt.Errorf("config: polymarket display category %q: limit must be 1 to %d", d.Category, pageSize)
		}
	}
	return out, nil
}

// displayQuery lists the first open markets of one display category. Parameter
// order is fixed so the saved endpoint is stable.
func displayQuery(category string, limit int) string {
	return fmt.Sprintf("categories=%s&active=true&closed=false&limit=%d&offset=0", url.QueryEscape(category), limit)
}

// isDisplayPage tells a display page from the Fed (macro) page by the saved
// endpoint, so rebuild takes the same path as fetch. An empty endpoint or a
// macro page is the Fed path, unchanged.
func isDisplayPage(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	c := u.Query().Get("categories")
	return c != "" && !strings.EqualFold(c, fedCategory)
}

// display fetches one display category: one request, cards and prices from the
// same page (no order book calls, see displayPrices). A failed page is one skip
// with the reason; it never fails the run, so a display hiccup cannot mark the
// Fed fetch failed. Only store errors return.
func (r *run) display(ctx context.Context, dc DisplayCategory) error {
	skipRun := func(reason string) error {
		return r.skips(ctx, []store.Skip{{CycleID: r.cycleID, RunID: r.runID, Stage: store.StageFetch, Venue: Venue, Reason: reason}})
	}
	raw, verr, err := r.get(ctx, "/v1/markets", displayQuery(dc.Category, dc.Limit), store.RawMarkets)
	if err != nil {
		return err
	}
	if verr != nil && strings.Contains(verr.Error(), fmt.Sprintf("HTTP %d", http.StatusTooManyRequests)) {
		// Still rate limited after download's 1 s, 2 s, 4 s backoff. The
		// gateway asks for Retry-After: 10 (measured 2026-10-10), so a display
		// page waits that long once more before it gives up.
		if err := r.f.sleep(ctx, displayRetryAfter); err == nil {
			if raw, verr, err = r.get(ctx, "/v1/markets", displayQuery(dc.Category, dc.Limit), store.RawMarkets); err != nil {
				return err
			}
		}
	}
	if verr != nil {
		return skipRun(fmt.Sprintf("display category %s not fetched: %v", dc.Category, verr))
	}
	cards, prices, skips, _, _, perr := normalizeMarkets(raw, r.f.vocab)
	if perr != nil {
		return skipRun(fmt.Sprintf("display category %s: %v", dc.Category, perr))
	}
	if err := r.skips(ctx, skips); err != nil {
		return err
	}
	for _, c := range cards {
		if _, err := r.st.Cards.Save(ctx, r.cycleID, c); err != nil {
			return err
		}
		r.saved++
	}
	for _, p := range prices {
		if err := r.st.Prices.Append(ctx, r.cycleID, p); err != nil {
			return err
		}
		r.saved++
	}
	return nil
}

// listingQuotes holds the best quotes a markets page carries for each market.
// It is decoded on its own, only on display pages, so the Fed path's market
// decoding is unchanged.
type listingQuotes struct {
	Status       string     `json:"status"`
	BestAskQuote *bookPrice `json:"bestAskQuote"`
	BestBidQuote *bookPrice `json:"bestBidQuote"`
}

type bookPrice struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// displayPrices reads a display market's prices from the markets page itself,
// so display categories cost one request each and no order book calls.
// Measured 2026-10-10, the public gateway answered HTTP 429 with Retry-After
// 10 after about 5 requests in a few seconds, far below the documented 25 per
// second, so a book per display market would crowd out the Fed books.
//
// bestAskQuote and bestBidQuote are the top of the single YES instrument's
// book: in the 2026-10-10 recording they equal the book's best offer and best
// bid for every market whose book was fetched (e.g. 0.8910 and 0.8900). Read
// the same way as a book:
//   - YES buy_price = bestAskQuote: buying YES lifts the best offer.
//   - NO buy_price = 1 - bestBidQuote: buying NO is selling YES at the best bid.
//
// A missing quote gives an empty price, never 0 or 1. A quote that is not USD,
// does not parse, or lies outside 0 to 1 makes that side empty and is logged.
// A market whose status is not MARKET_STATUS_OPEN gets empty prices. Display
// cards never match, so these prices are never routed. Fee is empty.
func displayPrices(rec json.RawMessage, slug string, raw store.Raw) ([]card.PriceRecord, []store.Skip) {
	var q listingQuotes
	yes, no := card.None[card.Micros](), card.None[card.Micros]()
	var skips []store.Skip
	if err := json.Unmarshal(rec, &q); err != nil {
		skips = append(skips, skip(raw, slug, "listing quotes do not parse: "+err.Error()))
	} else if q.Status == "MARKET_STATUS_OPEN" {
		if v, err := quote(q.BestAskQuote); err != nil {
			skips = append(skips, skip(raw, slug, "yes price: "+err.Error()))
		} else {
			yes = v
		}
		if v, err := quote(q.BestBidQuote); err != nil {
			skips = append(skips, skip(raw, slug, "no price: "+err.Error()))
		} else if bid, ok := v.Get(); ok {
			no = card.Some(card.OneDollar - bid)
		}
	}
	record := func(s card.Side, p card.Opt[card.Micros]) card.PriceRecord {
		return card.PriceRecord{Venue: Venue, MarketID: slug, Side: s, BuyPrice: p, PricedAt: raw.FetchedAt}
	}
	return []card.PriceRecord{record(card.Yes, yes), record(card.No, no)}, skips
}

func quote(p *bookPrice) (card.Opt[card.Micros], error) {
	if p == nil || p.Value == "" {
		return card.None[card.Micros](), nil
	}
	if p.Currency != "USD" {
		return card.None[card.Micros](), fmt.Errorf("price currency %q is not USD", p.Currency)
	}
	v, err := card.ParseMicros(p.Value)
	if err != nil {
		return card.None[card.Micros](), err
	}
	if err := card.CheckPrice(v); err != nil {
		return card.None[card.Micros](), err
	}
	return card.Some(v), nil
}

// displayCard builds a display-only card. Every payoff field stays empty except
// the deadline, so the card never matches, even when its rules happen to
// mention the federal funds rate. The deadline uses the same rule as the Fed
// path (exactly one "<Month> <D>, <YYYY> at <h:mm> AM|PM ET" moment in the
// rules, else empty); it only feeds the "Closing soon" sort.
func displayCard(m market, raw store.Raw) (card.Card, string) {
	if m.Slug == "" {
		return card.Card{}, "market record has no slug"
	}
	// Same side check as the Fed path: the book is read as YES and NO only
	// when the long side is labeled Yes and the short side No.
	if !yesNoSides(m.MarketSides) {
		return card.Card{}, "market sides are not labeled Yes (long) and No (short)"
	}
	return card.Card{
		Venue:    Venue,
		MarketID: m.Slug,
		Payoff:   card.Payoff{Deadline: deadlineOf(m.Description)},
		Record: card.RecordFields{
			Title:      firstNonEmpty(m.Title, m.Question),
			RulesText:  m.Description,
			RawRef:     strconv.FormatInt(raw.ID, 10),
			FetchedAt:  raw.FetchedAt,
			EventTitle: m.Question,
			Category:   displayCategory(m.Category),
			// No volume field in the October 2026 payload; empty if absent.
			Volume24h: m.Volume24hr.String(),
		},
	}, ""
}

// displayCategoryNames gives a display name to a Polymarket US category where
// Kalshi uses a different word for the same thing, so the feed's category
// filter groups both venues. Display only; never used to match.
var displayCategoryNames = map[string]string{
	"finance": "Financials",          // Kalshi: Financials
	"climate": "Climate and Weather", // Kalshi: Climate and Weather
}

// displayCategory returns the mapped name, or the venue's own value with its
// first letter in upper case (politics -> Politics), to match Kalshi's casing.
func displayCategory(c string) string {
	if n, ok := displayCategoryNames[strings.ToLower(c)]; ok {
		return n
	}
	if c == "" {
		return ""
	}
	return strings.ToUpper(c[:1]) + c[1:]
}
