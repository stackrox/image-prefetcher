package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestForwardingRe(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantPort string
		wantOK   bool
	}{
		{"typical", "Forwarding from 127.0.0.1:39956 -> 8080", "39956", true},
		{"low port", "Forwarding from 127.0.0.1:8080 -> 8080", "8080", true},
		{"ipv6 line is ignored", "Forwarding from [::1]:39956 -> 8080", "", false},
		{"unrelated", "Handling connection for 39956", "", false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := forwardingRe.FindStringSubmatch(tc.line)
			if tc.wantOK {
				if m == nil {
					t.Fatalf("expected match for %q, got none", tc.line)
				}
				if m[1] != tc.wantPort {
					t.Fatalf("expected port %q, got %q", tc.wantPort, m[1])
				}
			} else if m != nil {
				t.Fatalf("expected no match for %q, got %v", tc.line, m)
			}
		})
	}
}

// writeFakeKubectl writes an executable shell script standing in for kubectl and
// returns its path. The script's body is arbitrary shell; tests use it to make
// "kubectl" misbehave in specific ways.
func writeFakeKubectl(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake kubectl is a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("writing fake kubectl: %v", err)
	}
	return path
}

func testConfig(kubectl string) portForwardConfig {
	return portForwardConfig{
		kubectl:    kubectl,
		namespace:  "test-ns",
		service:    "svc/test-metrics",
		remotePort: 8080,
	}
}

// When kubectl announces the forwarded port, startPortForward parses it and the
// returned stop() promptly tears the (otherwise long-lived) process down.
func TestStartPortForwardParsesLocalPortAndStops(t *testing.T) {
	fake := writeFakeKubectl(t, `echo "Forwarding from 127.0.0.1:34567 -> 8080"; exec sleep 30`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	port, stop, err := startPortForward(ctx, testConfig(fake))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 34567 {
		t.Fatalf("expected port 34567, got %d", port)
	}

	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("stop() did not return promptly; kubectl was likely left running")
	}
}

// A pod that is not yet running makes kubectl exit immediately; startPortForward
// must surface that as an error carrying kubectl's stderr.
func TestStartPortForwardKubectlExitsEarly(t *testing.T) {
	fake := writeFakeKubectl(t, `echo "error: unable to forward port because pod is not running. Current status=Pending" >&2; exit 1`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _, err := startPortForward(ctx, testConfig(fake))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "exited before becoming ready") {
		t.Fatalf("error should mention early exit, got: %v", err)
	}
	if !strings.Contains(err.Error(), "pod is not running") {
		t.Fatalf("error should include kubectl stderr, got: %v", err)
	}
}

// If kubectl never announces the port, startPortForward times out when the
// context deadline is reached.
func TestStartPortForwardTimesOutWhenNeverReady(t *testing.T) {
	fake := writeFakeKubectl(t, `exec sleep 30`)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := startPortForward(ctx, testConfig(fake))
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("error should mention context deadline, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Fatalf("startPortForward took %s; it should have given up near the timeout", elapsed)
	}
}

// While the aggregator pod is still Pending, kubectl exits immediately;
// fetchViaPortForward must retry the whole cycle and succeed once kubectl finally
// establishes the tunnel. The fake fails twice, then "forwards" to a real local
// HTTP server so the fetch can complete deterministically (no reliance on
// timing).
func TestFetchViaPortForwardRetriesUntilReady(t *testing.T) {
	const want = `[{"attempt_id":"x"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(want))
	}))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]

	counter := filepath.Join(t.TempDir(), "attempts")
	fake := writeFakeKubectl(t, `c=$(cat "`+counter+`" 2>/dev/null || echo 0)
c=$((c+1)); echo "$c" > "`+counter+`"
if [ "$c" -lt 3 ]; then echo "error: unable to forward port because pod is not running. Current status=Pending" >&2; exit 1; fi
echo "Forwarding from 127.0.0.1:`+port+` -> 8080"; exec sleep 30`)

	restore := portForwardRetryDelay
	portForwardRetryDelay = 10 * time.Millisecond
	defer func() { portForwardRetryDelay = restore }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	body, err := fetchViaPortForward(ctx, testConfig(fake))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != want {
		t.Fatalf("unexpected body: %q", body)
	}
	data, readErr := os.ReadFile(counter)
	if readErr != nil {
		t.Fatalf("reading attempts file: %v", readErr)
	}
	if got := strings.TrimSpace(string(data)); got != "3" {
		t.Fatalf("expected kubectl to be invoked 3 times, got %q", got)
	}
}

// When kubectl never succeeds, fetchViaPortForward keeps retrying and eventually
// gives up (after ctx expires) having attempted more than once.
func TestFetchViaPortForwardGivesUpWhenKubectlAlwaysFails(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "attempts")
	fake := writeFakeKubectl(t, `echo x >> "`+counter+`"; echo "error: unable to forward port because pod is not running" >&2; exit 1`)

	restore := portForwardRetryDelay
	portForwardRetryDelay = 10 * time.Millisecond
	defer func() { portForwardRetryDelay = restore }()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := fetchViaPortForward(ctx, testConfig(fake))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "giving up establishing metrics port-forward") {
		t.Fatalf("unexpected error: %v", err)
	}
	data, readErr := os.ReadFile(counter)
	if readErr != nil {
		t.Fatalf("reading attempts file: %v", readErr)
	}
	if attempts := strings.Count(string(data), "x"); attempts < 2 {
		t.Fatalf("expected fetchViaPortForward to retry (>=2 kubectl invocations), got %d", attempts)
	}
}
