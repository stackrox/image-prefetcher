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
	c      chan *gen.Result
	done   chan struct{}
	client gen.MetricsClient
	logger *slog.Logger
}

func NewSubmitter(logger *slog.Logger, client gen.MetricsClient) *Submitter {
	return &Submitter{
		c:      make(chan *gen.Result, 1),
		done:   make(chan struct{}),
		client: client,
		logger: logger,
	}
}

func (s *Submitter) Run(ctx context.Context) {
	defer func() { s.done <- struct{}{} }()
	if s.client == nil {
		for range s.c {
		}
		return
	}
	hostName, err := os.Hostname()
	if err != nil {
		s.logger.WarnContext(ctx, "could not obtain hostname", "error", err)
		hostName = "unknown"
	}

	var metrics []*gen.Result
	for metric := range s.c {
		metric.Node = hostName
		s.logger.DebugContext(ctx, "metric received", "metric", metric)
		metrics = append(metrics, metric)
	}

	if err = s.submit(ctx, metrics); err == nil {
		s.logger.InfoContext(ctx, "metrics submitted")
		return
	}
	s.logger.ErrorContext(ctx, "metric Submit RPC failed, retrying", "error", err)
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 10 * time.Second
	b.MaxElapsedTime = 0
	ticker := backoff.NewTicker(backoff.WithContext(b, ctx))
	defer ticker.Stop()
	for range ticker.C {
		if ctx.Err() != nil {
			s.logger.ErrorContext(ctx, "giving up retrying metrics submission", "error", ctx.Err())
		}
		if err = s.submit(ctx, metrics); err == nil {
			s.logger.InfoContext(ctx, "metrics submitted")
			return
		}
		s.logger.ErrorContext(ctx, "metric Submit RPC failed, retrying", "error", err)
	}
}

func (s *Submitter) Await() {
	if s == nil {
		return
	}
	close(s.c)
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

func (s *Submitter) Chan() chan<- *gen.Result {
	if s == nil {
		return nil
	}
	return s.c
}
