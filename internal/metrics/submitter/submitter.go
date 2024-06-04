package submitter

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/stackrox/image-prefetcher/internal/metrics/gen"

	"github.com/cenkalti/backoff/v4"
)

type Submitter struct {
	channel chan *gen.Result
	done    chan struct{}
	client  gen.MetricsClient
	logger  *slog.Logger
	timer   backoff.Timer // for testing
}

// NewSubmitter creates a new submitter object.
func NewSubmitter(logger *slog.Logger, client gen.MetricsClient) *Submitter {
	return &Submitter{
		channel: make(chan *gen.Result, 1),
		done:    make(chan struct{}),
		client:  client,
		logger:  logger,
	}
}

// Chan returns a channel on which metrics can be provided to the submitter.
func (s *Submitter) Chan() chan<- *gen.Result {
	if s == nil {
		return nil
	}
	return s.channel
}

// Run accepts metrics on the channel and submits them to the client passed to constructor until Await is called.
func (s *Submitter) Run(ctx context.Context) (err error) {
	defer func() { s.done <- struct{}{} }()
	hostName, hostErr := os.Hostname()
	if hostErr != nil {
		s.logger.WarnContext(ctx, "could not obtain hostname", "error", hostErr)
		hostName = "unknown"
	}

	var metrics []*gen.Result
	for metric := range s.channel {
		metric.Node = hostName
		s.logger.DebugContext(ctx, "metric received", "metric", metric)
		metrics = append(metrics, metric)
	}

	ticker := newTicker(ctx, s.timer)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err = s.submit(ctx, metrics); err == nil {
				s.logger.InfoContext(ctx, "metrics submitted")
				return
			}
			s.logger.ErrorContext(ctx, "metric Submit RPC failed, retrying", "error", err)
		case <-ctx.Done():
			if ctx.Err() != nil {
				s.logger.ErrorContext(ctx, "giving up retrying metrics submission", "error", ctx.Err())
				err = ctx.Err()
			}
			return
		}
	}
}

// newTicker returns a ticker that ticks once immediately, and then backs off exponentially forever.
// Caller is responsible for calling Stop() on it eventually.
func newTicker(ctx context.Context, timer backoff.Timer) *backoff.Ticker {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 10 * time.Second
	b.MaxElapsedTime = 0
	return backoff.NewTickerWithTimer(backoff.WithContext(b, ctx), timer)
}

// Await signals the goroutine running Run that no more metrics will be sent on the channel.
// Then it waits for that goroutine to submit them (with retries).
func (s *Submitter) Await() {
	if s == nil {
		return
	}
	close(s.channel)
	s.logger.Info("waiting for metrics to be submitted")
	<-s.done
}

func (s *Submitter) submit(ctx context.Context, metrics []*gen.Result) error {
	submitClient, err := s.client.Submit(ctx)
	if err != nil {
		return fmt.Errorf("invoking metric Submit RPC failed: %w", err)
	}
	for _, metric := range metrics {
		if err := submitClient.Send(metric); err != nil {
			return fmt.Errorf("streaming metric to Submit RPC failed: %w", err)
		}
	}
	if _, err := submitClient.CloseAndRecv(); err != nil {
		return fmt.Errorf("finishing metric Submit RPC failed: %w", err)
	}
	return nil
}
