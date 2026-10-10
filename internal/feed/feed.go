package feed

import (
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"equinox/internal/feed/cache"
)

//go:embed templates/*.html
var templateFS embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{
	// postHref links to one post inside the cycle the page is reading.
	"stampOf":   stamp,
	"venueName": VenueName,
	// cardOf bundles what the post card template needs.
	"cardOf": func(p *Post, cycle int64, reference string) postCard {
		return postCard{Post: p, Cycle: cycle, Reference: reference}
	},
	// moduleCardOf is cardOf inside a category module, where the category is the heading.
	"moduleCardOf": func(p *Post, cycle int64, reference string) postCard {
		return postCard{Post: p, Cycle: cycle, Reference: reference, InModule: true}
	},
	"add": func(a, b int) int { return a + b },
	// flairOf pairs a venue with the reference venue for the flair template.
	"flairOf": func(venue, reference string) struct{ Venue, Reference string } {
		return struct{ Venue, Reference string }{venue, reference}
	},
	"postHref": func(cycle int64, key string) string {
		return "/post?" + url.Values{"cycle": {strconv.FormatInt(cycle, 10)}, "key": {key}}.Encode()
	},
}).ParseFS(templateFS, "templates/*.html"))

// postCard is the data for one post card: the post, the cycle its link reads,
// and the reference venue for flair.
type postCard struct {
	Post      *Post
	Cycle     int64
	Reference string
	InModule  bool // inside a category module, which already names the category
}

// Server renders the feed. It reads only from the cache.
type Server struct {
	cache    *cache.Cache
	pageSize int
	// Shown in the sidebar and used for venue flair; set with WithStatus.
	discoveryEvery, pricesEvery time.Duration
	reference                   string
}

// WithStatus sets the watch intervals shown in the sidebar and the reference
// venue, whose flair is filled (the other venue's is outlined).
func (s *Server) WithStatus(discoveryEvery, pricesEvery time.Duration, reference string) *Server {
	s.discoveryEvery, s.pricesEvery, s.reference = discoveryEvery, pricesEvery, reference
	return s
}

// New returns a feed server that shows pageSize posts per page.
func New(c *cache.Cache, pageSize int) *Server {
	if pageSize < 1 {
		pageSize = 25
	}
	return &Server{cache: c, pageSize: pageSize}
}

// Handler routes the feed's two pages: the post list and one post.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.feed)
	mux.HandleFunc("GET /post", s.post)
	return mux
}

// Frame is what every page shows around its content: which cycle it reads,
// the new-data banner and the sidebar status.
type Frame struct {
	Snap   *cache.Snapshot
	Latest *cache.Snapshot
	Banner bool   // a newer cycle completed after this page's cycle
	Reload string // link to the newest cycle with the same sort and filters
	Note   string
	Counts []venueCount
	Status cache.Status
	// Discovery and Prices are watch's intervals, e.g. "30 min"; empty if unset.
	Discovery, Prices string
	Reference         string
	// Watch is running, stale (holds the lock but no cycle completed within
	// three price intervals: it may have crashed) or stopped.
	Watch string
}

type venueCount struct {
	Venue string
	N     int
}

type feedPage struct {
	Frame
	Posts      []*Post
	Sort       string
	Venue      string
	Category   string
	Venues     []string
	Categories []string
	Sorts      []sortLink
	Shown      int
	Total      int
	More       string
}

// Section is one category module on the overview: its top posts by volume.
type Section struct {
	Category string
	Top      []*Post
	Total    int
	Matched  int
	Href     string // the full list for this category
}

type overviewPage struct {
	Frame
	Sections   []Section
	Stats      cache.Stats
	Venue      string
	Venues     []string
	Categories []string
	Sorts      []sortLink
	Sort       string
	Category   string
}

// overviewSize is how many posts each category module shows.
const overviewSize = 5

type sortLink struct {
	Label, Href string
	On          bool
}

type postPage struct {
	Frame
	Post *Post
	Back string
}

// snapshot picks the snapshot a request reads: the one its cycle parameter
// names if still held, otherwise the newest. ok is false when the named cycle
// is no longer held.
func (s *Server) snapshot(q url.Values) (snap *cache.Snapshot, ok bool) {
	cur := s.cache.Current()
	if q.Get("cycle") == "" {
		return cur, true
	}
	n, err := strconv.ParseInt(q.Get("cycle"), 10, 64)
	if err != nil {
		return cur, false
	}
	if held, found := s.cache.At(n); found {
		return held, true
	}
	return cur, false
}

func (s *Server) frameFor(snap *cache.Snapshot, keep url.Values) Frame {
	latest := s.cache.Current()
	f := Frame{Snap: snap, Latest: latest, Reload: "/?" + keep.Encode(), Status: s.cache.Status(),
		Discovery: every(s.discoveryEvery), Prices: every(s.pricesEvery), Reference: s.reference}
	f.Watch = watchState(f.Status, latest, s.pricesEvery, time.Now())
	f.Banner = latest.Cycle > snap.Cycle
	n := map[string]int{}
	for _, c := range snap.Cards() {
		n[c.Venue]++
	}
	for v, k := range n {
		f.Counts = append(f.Counts, venueCount{v, k})
	}
	sort.Slice(f.Counts, func(i, j int) bool { return f.Counts[i].Venue < f.Counts[j].Venue })
	return f
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	snap, held := s.snapshot(q)
	if snap == nil {
		s.render(w, "empty", nil)
		return
	}
	// The overview is the landing page. Any sort, category, cursor or cycle
	// parameter asks for the paged list, which keeps a reader on one cycle (W6).
	if q.Get("sort") == "" && q.Get("category") == "" && q.Get("cursor") == "" && q.Get("cycle") == "" {
		s.overview(w, q, snap)
		return
	}
	order := q.Get("sort")
	if order != SortClosing && order != SortMatched {
		order = SortTop
	}
	venue, category := q.Get("venue"), q.Get("category")
	keep := url.Values{}
	for k, v := range map[string]string{"sort": order, "venue": venue, "category": category} {
		if v != "" {
			keep.Set(k, v)
		}
	}
	p := feedPage{Frame: s.frameFor(snap, keep), Sort: order, Venue: venue, Category: category}
	cursor, _ := strconv.Atoi(q.Get("cursor"))
	if !held {
		p.Note = "The cycle this page started on is no longer held, so the feed starts again from the newest cycle."
		cursor = 0
	}

	all := buildPosts(snap)
	p.Venues, p.Categories = distinct(all)
	list := filterSort(all, venue, category, order)
	cursor = max(0, min(cursor, len(list)))
	end := min(cursor+s.pageSize, len(list))
	p.Posts, p.Shown, p.Total = list[cursor:end], end, len(list)
	if end < len(list) {
		more := cloneValues(keep)
		more.Set("cycle", strconv.FormatInt(snap.Cycle, 10))
		more.Set("cursor", strconv.Itoa(end))
		p.More = "/?" + more.Encode()
	}
	p.Sorts = sortLinks(keep, order)
	s.render(w, "feed", p)
}

// sortLinks are the view tabs: the overview, then the three list sorts.
func sortLinks(keep url.Values, current string) []sortLink {
	over := url.Values{}
	if v := keep.Get("venue"); v != "" {
		over.Set("venue", v)
	}
	out := []sortLink{{Label: "Overview", Href: "/?" + over.Encode(), On: current == ""}}
	for _, sl := range []struct{ label, key string }{{"Top", SortTop}, {"Closing soon", SortClosing}, {"Matched", SortMatched}} {
		v := cloneValues(keep)
		v.Set("sort", sl.key)
		out = append(out, sortLink{Label: sl.label, Href: "/?" + v.Encode(), On: sl.key == current})
	}
	return out
}

// topWithEveryVenue picks up to n posts from a ranked list, reserving a slot
// for the best post of each venue present, so a venue that reports no volume
// still appears in its category. The result keeps rank order.
func topWithEveryVenue(ranked []*Post, n int) []*Post {
	pick := map[int]bool{}
	seen := map[string]bool{}
	for i, p := range ranked {
		if !seen[p.Venue] && len(pick) < n {
			seen[p.Venue] = true
			pick[i] = true
		}
	}
	for i := range ranked {
		if len(pick) >= n {
			break
		}
		pick[i] = true
	}
	var out []*Post
	for i, p := range ranked {
		if pick[i] {
			out = append(out, p)
		}
	}
	return out
}

// overview renders one module per category with its top posts by volume.
func (s *Server) overview(w http.ResponseWriter, q url.Values, snap *cache.Snapshot) {
	venue := q.Get("venue")
	keep := url.Values{}
	if venue != "" {
		keep.Set("venue", venue)
	}
	p := overviewPage{Frame: s.frameFor(snap, keep), Stats: snap.Stats(), Venue: venue}
	all := buildPosts(snap)
	p.Venues, p.Categories = distinct(all)
	byCat := map[string][]*Post{}
	for _, post := range filterSort(all, venue, "", SortTop) {
		byCat[post.Category] = append(byCat[post.Category], post)
	}
	for cat, posts := range byCat {
		sec := Section{Category: cat, Total: len(posts)}
		for _, post := range posts {
			if post.Matched() {
				sec.Matched++
			}
		}
		sec.Top = topWithEveryVenue(posts, overviewSize)
		v := cloneValues(keep)
		v.Set("category", cat)
		v.Set("sort", SortTop)
		sec.Href = "/?" + v.Encode()
		p.Sections = append(p.Sections, sec)
	}
	// Categories with a match come first, then by number of posts, then by name.
	sort.Slice(p.Sections, func(i, j int) bool {
		a, b := p.Sections[i], p.Sections[j]
		if (a.Matched > 0) != (b.Matched > 0) {
			return a.Matched > 0
		}
		if a.Total != b.Total {
			return a.Total > b.Total
		}
		return a.Category < b.Category
	})
	p.Sorts = sortLinks(keep, "")
	s.render(w, "overview", p)
}

func (s *Server) post(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	snap, held := s.snapshot(q)
	if snap == nil || !held {
		http.Error(w, "That cycle is no longer held. Go back to the feed for the newest cycle.", http.StatusNotFound)
		return
	}
	for _, p := range buildPosts(snap) {
		if p.Key == q.Get("key") {
			s.render(w, "post", postPage{Frame: s.frameFor(snap, url.Values{}), Post: p, Back: "/"})
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// watchState judges the refresh process from the run lock and the age of the
// newest completed cycle. The feed reads the clock only for this display.
func watchState(st cache.Status, latest *cache.Snapshot, pricesEvery time.Duration, now time.Time) string {
	if !st.WatchRunning {
		return "stopped"
	}
	if pricesEvery <= 0 {
		pricesEvery = 2 * time.Minute
	}
	if latest == nil || now.Sub(latest.CompletedAt) > 3*pricesEvery {
		return "stale"
	}
	return "running"
}

// every formats an interval for the sidebar, e.g. 30m -> "30 min".
func every(d time.Duration) string {
	switch {
	case d <= 0:
		return ""
	case d%time.Minute == 0:
		return strconv.Itoa(int(d/time.Minute)) + " min"
	}
	return d.String()
}

func distinct(posts []*Post) (venues, categories []string) {
	vs, cs := map[string]bool{}, map[string]bool{}
	for _, p := range posts {
		if !vs[p.Venue] {
			vs[p.Venue] = true
			venues = append(venues, p.Venue)
		}
		if p.Category != "" && !cs[p.Category] {
			cs[p.Category] = true
			categories = append(categories, p.Category)
		}
	}
	sort.Strings(venues)
	sort.Strings(categories)
	return venues, categories
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
