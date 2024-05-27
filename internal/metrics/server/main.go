package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/stackrox/image-prefetcher/internal/metrics/gen"

	"google.golang.org/grpc"
)

type metricsServer struct {
	mutex   sync.Mutex
	metrics map[string]*gen.Result
	logger  *slog.Logger
	gen.UnimplementedMetricsServer
}

func (s *metricsServer) Submit(stream gen.Metrics_SubmitServer) error {
	for {
		metric, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&gen.Empty{})
		}
		if err != nil {
			return err
		}
		s.metricSubmitted(metric)
	}
}

func (s *metricsServer) metricSubmitted(metric *gen.Result) {
	s.logger.Debug("metric submitted", "metric", metric)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, ok := s.metrics[metric.AttemptId]; ok {
		s.logger.Info("duplicate metric submitted", "metric", metric)
	}
	s.metrics[metric.AttemptId] = metric
}

func (s *metricsServer) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	resp, err := json.Marshal(s.currentMetrics())
	if err != nil {
		s.logger.Error("failed to marshal metrics", "error", err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = writer.Write(resp)
	if err != nil {
		s.logger.Error("failed to write HTTP metrics response", "error", err)
	}
}

func (s *metricsServer) currentMetrics() []*gen.Result {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	metrics := make([]*gen.Result, 0, len(s.metrics))
	for _, metric := range s.metrics {
		metrics = append(metrics, metric)
	}
	return metrics
}

func Run(logger *slog.Logger, grpcPort int, httpPort int) error {
	server := &metricsServer{
		logger:  logger,
		metrics: make(map[string]*gen.Result),
	}
	grpcErrChan := make(chan error)
	httpErrChan := make(chan error)

	grpcSpec := fmt.Sprintf(":%d", grpcPort)
	grpcListener, err := net.Listen("tcp", grpcSpec)
	if err != nil {
		return fmt.Errorf("failed to listen on %s", grpcSpec)
	}
	grpcServer := grpc.NewServer()
	gen.RegisterMetricsServer(grpcServer, server)
	logger.Info("starting to serve", "grpcSpec", grpcSpec)
	go func() { grpcErrChan <- grpcServer.Serve(grpcListener) }()

	httpSpec := fmt.Sprintf(":%d", httpPort)
	httpListener, err := net.Listen("tcp", httpSpec)
	if err != nil {
		return fmt.Errorf("failed to listen on %s", httpSpec)
	}
	httpServer := &http.Server{}
	http.Handle("/metrics", server)
	logger.Info("starting to serve", "httpSpec", httpSpec)
	go func() { httpErrChan <- httpServer.Serve(httpListener) }()

	// On shutdown of either, stop the other one.
	var httpErr, grpcErr error
	select {
	case httpErr = <-httpErrChan:
		grpcServer.Stop()
		grpcErr = <-grpcErrChan
	case grpcErr = <-grpcErrChan:
		_ = httpServer.Close()
		httpErr = <-httpErrChan
	}
	return errors.Join(grpcErr, httpErr)
}
