package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBackoffDelayIsCapped(t *testing.T) {
	const cap = 5 * time.Second
	prev := time.Duration(0)
	for attempt := 1; attempt <= 20; attempt++ {
		d := backoffDelay(attempt)
		if d <= 0 {
			t.Fatalf("attempt %d: non-positive delay %s", attempt, d)
		}
		if d > cap {
			t.Fatalf("attempt %d: delay %s exceeds cap %s", attempt, d, cap)
		}
		if attempt > 1 && d < prev && prev != cap {
			t.Fatalf("attempt %d: delay %s decreased from %s before reaching cap", attempt, d, prev)
		}
		prev = d
	}
}

func TestFetchWithRetrySucceedsAfterTransientFailure(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 2 {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`[{"attempt_id":"x"}]`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	body, err := fetchWithRetry(ctx, srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != `[{"attempt_id":"x"}]` {
		t.Fatalf("unexpected body: %s", body)
	}
	if calls < 2 {
		t.Fatalf("expected at least 2 calls, got %d", calls)
	}
}

func TestFetchOnceRejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := fetchOnce(ctx, srv.URL); err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}
