package polymarket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"equinox/internal/card"
)

// A 429 with Retry-After waits as long as the gateway asks (capped), not just
// the 1 s first backoff, then succeeds on the retry.
func TestPolymarket_HonorsRetryAfter(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "10")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	f := New(card.FetcherConfig{BaseURL: srv.URL}, nil, srv.Client(), time.Now)
	var waits []time.Duration
	f.gap, f.sleep = 0, func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	body, err := f.download(context.Background(), "/v1/markets")
	if err != nil || string(body) != `{"ok":true}` {
		t.Fatalf("download: %q %v", body, err)
	}
	if len(waits) != 1 || waits[0] != 10*time.Second {
		t.Fatalf("waits %v, want one 10s wait from Retry-After", waits)
	}
}
