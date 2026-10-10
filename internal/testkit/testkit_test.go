package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"equinox/internal/store"
)

func get(t *testing.T, c *http.Client, ctx context.Context, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c.Do(req)
}

func TestServesFixturesByPathAndQuery(t *testing.T) {
	tr := Scenario(t, filepath.Join("testdata", "demo", "ok"))
	c := tr.Client()
	// Host is ignored and query order does not matter.
	resp, err := get(t, c, context.Background(), "https://any.venue.example/v1/markets?limit=2&status=open")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct{ Markets []struct{ ID string } }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || resp.StatusCode != 200 || len(body.Markets) != 1 || body.Markets[0].ID != "M1" {
		t.Fatalf("status %d, body %+v, err %v", resp.StatusCode, body, err)
	}
	if got := tr.Requests(); len(got) != 1 || got[0] != "GET /v1/markets?limit=2&status=open" {
		t.Errorf("requests: %v", got)
	}
}

func TestUnknownRequestFails(t *testing.T) {
	c := Scenario(t, filepath.Join("testdata", "demo", "ok")).Client()
	for _, url := range []string{
		"https://x/v1/markets",                     // missing query
		"https://x/v1/markets?status=open&limit=3", // different query value
		"https://x/v1/markets/M2/book",             // unknown path
	} {
		if _, err := get(t, c, context.Background(), url); !errors.Is(err, ErrUnknownRequest) {
			t.Errorf("%s: want ErrUnknownRequest, got %v", url, err)
		}
	}
}

func TestFaultTimeout(t *testing.T) {
	tr := Scenario(t, filepath.Join("testdata", "demo", "faults"))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := get(t, tr.Client(), ctx, "https://x/v1/slow")
	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("want a timeout net.Error, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("timeout fault must end when the request context ends")
	}

	tr.TimeoutCap = 20 * time.Millisecond
	if _, err := get(t, tr.Client(), context.Background(), "https://x/v1/slow"); !errors.As(err, &ne) {
		t.Errorf("timeout without a context deadline must still fail, got %v", err)
	}
}

func TestFaultHTTP500AndMalformedJSON(t *testing.T) {
	c := Scenario(t, filepath.Join("testdata", "demo", "faults")).Client()
	resp, err := get(t, c, context.Background(), "https://x/v1/broken")
	if err != nil || resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want HTTP 500, got %v %v", resp, err)
	}
	resp.Body.Close()

	resp, err = get(t, c, context.Background(), "https://x/v1/garbled")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var v any
	if json.Unmarshal(b, &v) == nil {
		t.Errorf("malformed_json body must not parse: %s", b)
	}
}

func TestBadManifestsAreRejected(t *testing.T) {
	cases := map[string]string{
		"missing file":  `{"routes":[{"path":"/a","file":"nope.json"}]}`,
		"no file":       `{"routes":[{"path":"/a"}]}`,
		"unknown fault": `{"routes":[{"path":"/a","fault":"slow_loris"}]}`,
		"duplicate":     `{"routes":[{"path":"/a","fault":"http_500"},{"path":"/a","fault":"timeout"}]}`,
		"not json":      `{"routes":`,
	}
	for name, m := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(m), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadScenario(dir); err == nil {
			t.Errorf("%s: manifest must be rejected", name)
		}
	}
}

func TestNoNetworkRefusesEverything(t *testing.T) {
	n := &NoNetwork{}
	if _, err := get(t, n.Client(), context.Background(), "https://x/anything"); err == nil {
		t.Fatal("NoNetwork must fail every request")
	}
	if n.Calls() != 1 {
		t.Errorf("calls = %d", n.Calls())
	}
}

func TestTempStoreAndFixedTime(t *testing.T) {
	s := TempStore(t)
	id, err := s.Cycles.Begin(context.Background(), store.KindFetch, store.TriggerCLI)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Cycles.Get(context.Background(), id)
	if err != nil || !c.StartedAt.Equal(AsOf) {
		t.Errorf("temp store clock must read AsOf: %+v %v", c, err)
	}
	if AsOf.Location() != time.UTC {
		t.Error("AsOf must be UTC")
	}
	if _, err := os.Stat(filepath.Join(RepoRoot(t), "go.mod")); err != nil {
		t.Errorf("RepoRoot: %v", err)
	}
	if got := Fixture(t, "kalshi", "fed"); filepath.Base(got) != "fed" || filepath.Base(filepath.Dir(got)) != "kalshi" {
		t.Errorf("Fixture path: %s", got)
	}
}
