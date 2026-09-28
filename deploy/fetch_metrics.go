package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// fetchOptions configures the --fetch-metrics mode.
type fetchOptions struct {
	name              string // prefetcher set name; the Service is <name>-metrics
	namespace         string
	remotePort        int
	kubectl           string
	kubeContext       string
	overallTimeout    time.Duration
	onePortFwdTimeout time.Duration
}

// runFetchMetrics retrieves the JSON metrics exposed by the aggregator's HTTP
// endpoint without relying on an externally reachable LoadBalancer. It uses
// `kubectl port-forward` to the metrics Service, performs an HTTP GET against the
// local end of the tunnel, and finally tears the tunnel down.
//
// The metrics JSON is written to stdout; all diagnostics go to stderr, making it
// a drop-in replacement for `curl .../metrics > out.json`.
//
// Only the Go standard library is used on purpose: this tool lives in its own
// dependency-free module.
func runFetchMetrics(opts fetchOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.overallTimeout)
	defer cancel()

	body, err := fetchViaPortForward(ctx, opts)
	if err != nil {
		return err
	}
	if _, err := os.Stdout.Write(body); err != nil {
		return fmt.Errorf("writing metrics to stdout: %w", err)
	}
	return nil
}

// fetchViaPortForward repeatedly (re)establishes the port-forward and fetches
// the metrics until it succeeds or ctx expires.
func fetchViaPortForward(ctx context.Context, opts fetchOptions) ([]byte, error) {
	cfg := portForwardConfig{
		kubectl:    opts.kubectl,
		context:    opts.kubeContext,
		namespace:  opts.namespace,
		service:    "svc/" + opts.name + "-metrics",
		remotePort: opts.remotePort,
	}

	var lastErr error
	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("giving up establishing metrics port-forward after %d attempt(s): %w (last error: %v)", attempt-1, ctx.Err(), lastErr)
		}
		body, err := func() ([]byte, error) {
			portFwdCtx, portFwdCancel := context.WithTimeout(ctx, opts.onePortFwdTimeout)
			defer portFwdCancel()
			localPort, err := startPortForward(ctx, cfg)
			if err != nil {
				return nil, err
			}
			url := fmt.Sprintf("http://127.0.0.1:%d/metrics", localPort)
			log.Printf("port-forward to %s established on 127.0.0.1:%d; fetching %s", cfg.service, localPort, url)
			return fetchWithRetry(portFwdCtx, url)
		}()
		if err == nil {
			return body, nil
		}
		lastErr = err
		log.Printf("attempt %d to retrieve metrics failed: %v; retrying", attempt, err)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("giving up establishing metrics port-forward after %d attempt(s): %w (last error: %v)", attempt, ctx.Err(), lastErr)
		case <-time.After(portForwardRetryDelay):
		}
	}
}

// fetchWithRetry performs an HTTP GET, retrying transient failures with capped
// exponential backoff until it succeeds or ctx is done. Even with the tunnel
// established, the first request(s) can race the forwarder finishing setup, so a
// few retries make this robust.
func fetchWithRetry(ctx context.Context, url string) ([]byte, error) {
	const maxAttempts = 10
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		body, err := fetchOnce(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		log.Printf("attempt %d/%d to fetch metrics failed: %v", attempt, maxAttempts, err)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("giving up fetching metrics: %w (last error: %v)", ctx.Err(), lastErr)
		case <-time.After(backoffDelay(attempt)):
		}
	}
	return nil, fmt.Errorf("giving up after %d attempts to fetch metrics: %w", maxAttempts, lastErr)
}

func fetchOnce(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	return body, nil
}

func backoffDelay(attempt int) time.Duration {
	const (
		base     = 500 * time.Millisecond
		maxDelay = 5 * time.Second
	)
	d := base << (attempt - 1)
	if d <= 0 || d > maxDelay {
		return maxDelay
	}
	return d
}
