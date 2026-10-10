package feed

import (
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"equinox/internal/feed/cache"
)

//go:embed templates/*.html
var templateFS embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{
	// postHref links to one post inside the cycle the page is reading.
	"stampOf": stamp,
	"postHref": func(cycle int64, key string) string {
		return "/post?" + url.Values{"cycle": {strconv.FormatInt(cycle, 10)}, "key": {key}}.Encode()
	},
}).ParseFS(templateFS, "templates/*.html"))

// Server renders the feed. It reads only from the cache.
type Server struct {
	cache    *cache.Cache
	pageSize int
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
	f := Frame{Snap: snap, Latest: latest, Reload: "/?" + keep.Encode()}
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
	for _, sl := range []struct{ label, key string }{{"Top", SortTop}, {"Closing soon", SortClosing}, {"Matched", SortMatched}} {
		v := cloneValues(keep)
		v.Set("sort", sl.key)
		p.Sorts = append(p.Sorts, sortLink{Label: sl.label, Href: "/?" + v.Encode(), On: sl.key == order})
	}
	s.render(w, "feed", p)
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
