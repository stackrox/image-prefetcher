package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"
)

type portForwardConfig struct {
	kubectl    string
	context    string
	namespace  string
	service    string
	remotePort int
}

// portForwardRetryDelay is how long we wait between attempts to (re)establish
// the port-forward. It is a var (not a const) so tests can shorten it.
var portForwardRetryDelay = 3 * time.Second

// forwardingRe matches the line kubectl prints once the tunnel is ready, e.g.:
//
//	Forwarding from 127.0.0.1:39956 -> 8080
var forwardingRe = regexp.MustCompile(`Forwarding from 127\.0\.0\.1:(\d+)`)

// startPortForward starts `kubectl port-forward` asking the OS to pick a free
// local port (by passing ":<remotePort>") to avoid ToCToU issues.
//
// On success, it returns the chosen local port.
// The returned func must be called by the caller after the context is cancelled.
func startPortForward(ctx context.Context, cfg portForwardConfig) (int, func(), error) {
	args := []string{"-n", cfg.namespace, "port-forward", cfg.service, fmt.Sprintf(":%d", cfg.remotePort)}
	if cfg.context != "" {
		args = append(args, "--context", cfg.context)
	}

	cmd := exec.CommandContext(ctx, cfg.kubectl, args...)
	cmd.WaitDelay = 1 * time.Millisecond

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, func() {}, fmt.Errorf("obtaining kubectl stdout pipe: %w", err)
	}
	var stderr syncBuffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return 0, func() {}, fmt.Errorf("starting kubectl port-forward: %w", err)
	}

	type result struct {
		port int
		err  error
	}
	ready := make(chan result, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if m := forwardingRe.FindStringSubmatch(scanner.Text()); m != nil {
				port, convErr := strconv.Atoi(m[1])
				if convErr != nil {
					ready <- result{err: fmt.Errorf("parsing local port from %q: %w", scanner.Text(), convErr)}
					return
				}
				// Keep draining stdout so kubectl never blocks writing to a full pipe.
				go func() { _, _ = io.Copy(io.Discard, stdout) }()
				ready <- result{port: port}
				return
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			ready <- result{err: fmt.Errorf("reading kubectl output: %w", scanErr)}
			return
		}
		ready <- result{err: fmt.Errorf("kubectl port-forward exited before becoming ready: %s", stderr.String())}
	}()

	select {
	case res := <-ready:
		if res.err != nil {
			return 0, func() { _ = cmd.Wait() }, res.err
		}
		return res.port, func() { _ = cmd.Wait() }, nil
	case <-ctx.Done():
		return 0, func() { _ = cmd.Wait() }, ctx.Err()
	}
}

// syncBuffer is a minimal concurrency-safe buffer so we can read kubectl's
// stderr from the select arms while os/exec's copier goroutine writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
