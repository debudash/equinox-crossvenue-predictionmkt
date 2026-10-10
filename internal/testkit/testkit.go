// Package testkit lets fetcher, command and integration tests run without a
// live venue: a fixture-serving http.RoundTripper, fault injection from a
// manifest, a fixed as_of, and a temporary SQLite store. See fixtures/README.md.
package testkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"equinox/internal/store"
)

// AsOf is the fixed reference time for tests. Routers receive it; they never
// read the clock.
var AsOf = time.Date(2026, 10, 9, 16, 0, 0, 0, time.UTC)

// Faults a manifest route can declare instead of serving its file normally.
const (
	FaultTimeout       = "timeout"        // the request hangs until its context ends, then fails with a timeout error
	FaultHTTP500       = "http_500"       // the response is HTTP 500 with a plain-text body
	FaultMalformedJSON = "malformed_json" // the body is cut in half (or "{" with no file), so it does not parse
)

// Manifest describes one scenario: which file answers which request.
type Manifest struct {
	Description string  `json:"description"`
	Routes      []Route `json:"routes"`
}

// Route maps one request to one response. Query is matched as a set of
// parameters, so parameter order does not matter. Host is ignored.
type Route struct {
	Method string `json:"method"`           // default GET
	Path   string `json:"path"`             // e.g. /trade-api/v2/markets
	Query  string `json:"query,omitempty"`  // e.g. status=open&series_ticker=KXFED
	File   string `json:"file,omitempty"`   // body file, relative to the scenario directory
	Status int    `json:"status,omitempty"` // default 200
	Fault  string `json:"fault,omitempty"`  // one of the Fault constants
}

// Transport serves a scenario's fixtures and fails any request the manifest
// does not list. It is safe for concurrent use.
type Transport struct {
	dir    string
	routes map[string]Route
	// TimeoutCap bounds a FaultTimeout request whose context never ends, so a
	// fetcher without a timeout fails the test instead of hanging it.
	TimeoutCap time.Duration

	mu       sync.Mutex
	requests []string
}

// LoadScenario reads dir/manifest.json and checks that every route is valid
// and every file exists.
func LoadScenario(dir string) (*Transport, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s/manifest.json: %w", dir, err)
	}
	tr := &Transport{dir: dir, routes: map[string]Route{}, TimeoutCap: 5 * time.Second}
	for _, r := range m.Routes {
		if r.Method == "" {
			r.Method = http.MethodGet
		}
		if r.Status == 0 {
			r.Status = http.StatusOK
		}
		switch r.Fault {
		case "", FaultTimeout, FaultHTTP500, FaultMalformedJSON:
		default:
			return nil, fmt.Errorf("%s: route %s has unknown fault %q", dir, r.Path, r.Fault)
		}
		if r.File == "" && r.Fault == "" {
			return nil, fmt.Errorf("%s: route %s has no file", dir, r.Path)
		}
		if r.File != "" {
			if _, err := os.Stat(filepath.Join(dir, r.File)); err != nil {
				return nil, fmt.Errorf("%s: route %s: %w", dir, r.Path, err)
			}
		}
		k, err := key(r.Method, r.Path, r.Query)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		if _, dup := tr.routes[k]; dup {
			return nil, fmt.Errorf("%s: duplicate route %s", dir, k)
		}
		tr.routes[k] = r
	}
	return tr, nil
}

// Scenario loads a scenario directory for a test and fails the test if it
// does not load.
func Scenario(t testing.TB, dir string) *Transport {
	t.Helper()
	tr, err := LoadScenario(dir)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// Fixture returns the directory of fixtures/<venue>/<scenario> from anywhere in
// the module.
func Fixture(t testing.TB, venue, scenario string) string {
	t.Helper()
	return filepath.Join(RepoRoot(t), "fixtures", venue, scenario)
}

// RepoRoot returns the directory holding go.mod.
func RepoRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("testkit: go.mod not found above the working directory")
		}
		dir = parent
	}
}

// Client returns an http.Client that uses this transport.
func (tr *Transport) Client() *http.Client { return &http.Client{Transport: tr} }

// Requests returns every request seen, as "METHOD /path?query", in order.
func (tr *Transport) Requests() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return append([]string(nil), tr.requests...)
}

// ErrUnknownRequest is returned for any request the manifest does not list.
var ErrUnknownRequest = errors.New("testkit: no fixture for request")

// RoundTrip implements http.RoundTripper.
func (tr *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	tr.requests = append(tr.requests, req.Method+" "+req.URL.RequestURI())
	tr.mu.Unlock()

	k, err := key(req.Method, req.URL.Path, req.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	r, ok := tr.routes[k]
	if !ok {
		return nil, fmt.Errorf("%w: %s %s (live venues are never called in tests)", ErrUnknownRequest, req.Method, req.URL.RequestURI())
	}
	switch r.Fault {
	case FaultTimeout:
		select {
		case <-req.Context().Done():
		case <-time.After(tr.TimeoutCap):
		}
		return nil, timeoutError{}
	case FaultHTTP500:
		return respond(req, http.StatusInternalServerError, []byte("internal server error (testkit fault)")), nil
	}
	var body []byte
	if r.File != "" {
		if body, err = os.ReadFile(filepath.Join(tr.dir, r.File)); err != nil {
			return nil, err
		}
	}
	if r.Fault == FaultMalformedJSON {
		body = append([]byte("{"), body[:len(body)/2]...)
	}
	return respond(req, r.Status, body), nil
}

func respond(req *http.Request, status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

// timeoutError satisfies net.Error with Timeout() true.
type timeoutError struct{}

func (timeoutError) Error() string   { return "testkit: request timed out (fault)" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func key(method, path, rawQuery string) (string, error) {
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("query %q: %w", rawQuery, err)
	}
	// Encode sorts by key, so parameter order does not matter.
	return method + " " + path + "?" + q.Encode(), nil
}

// NoNetwork is a transport that fails every request. Use it to prove a code
// path (rebuild, match, route) makes no venue call.
type NoNetwork struct {
	mu    sync.Mutex
	calls int
}

// RoundTrip implements http.RoundTripper and always fails.
func (n *NoNetwork) RoundTrip(req *http.Request) (*http.Response, error) {
	n.mu.Lock()
	n.calls++
	n.mu.Unlock()
	return nil, fmt.Errorf("testkit: network call refused: %s %s", req.Method, req.URL)
}

// Calls returns how many requests were attempted.
func (n *NoNetwork) Calls() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.calls
}

// Client returns an http.Client that uses this transport.
func (n *NoNetwork) Client() *http.Client { return &http.Client{Transport: n} }

// FixedClock returns a clock that always reads t.
func FixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// TempStore opens a fresh SQLite file in a temp directory, with its clock fixed
// at AsOf, and closes it when the test ends.
func TempStore(t testing.TB) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "equinox.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.Clock = FixedClock(AsOf)
	t.Cleanup(func() { s.Close() })
	return s
}
