package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/stackrox/image-prefetcher/internal/credentialprovider"
	metricsProto "github.com/stackrox/image-prefetcher/internal/metrics/gen"
	"github.com/stackrox/image-prefetcher/internal/metrics/submitter"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	criV1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type TimingConfig struct {
	ImageListTimeout          time.Duration
	InitialPullAttemptTimeout time.Duration
	MaxPullAttemptTimeout     time.Duration
	OverallTimeout            time.Duration
	InitialPullAttemptDelay   time.Duration
	MaxPullAttemptDelay       time.Duration
}

func Run(logger *slog.Logger, criSocketPath string, dockerConfigJSONPath string, timing TimingConfig, metricsEndpoint string, imageNames ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timing.OverallTimeout)
	defer cancel()

	criConn, err := grpc.NewClient("unix://"+criSocketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to dial CRI socket %q: %w", criSocketPath, err)
	}
	criClient := criV1.NewImageServiceClient(criConn)

	if err := listImagesForDebugging(ctx, logger, criClient, timing.ImageListTimeout, "before"); err != nil {
		return fmt.Errorf("failed to list images for debugging before pulling: %w", err)
	}

	var metricsSink *submitter.Submitter
	if metricsEndpoint != "" {
		metricsConn, err := grpc.NewClient(metricsEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to dial metrics endpoint %q: %w", metricsEndpoint, err)
		}
		metricsSink = submitter.NewSubmitter(logger, metricsProto.NewMetricsClient(metricsConn))
		go func() { _ = metricsSink.Run(ctx) }() // returned error is for testing, sink already handles errors
	}

	kr := credentialprovider.BasicDockerKeyring{}
	if err := loadPullSecret(logger, &kr, dockerConfigJSONPath); err != nil {
		return fmt.Errorf("failed to load image pull secrets: %w", err)
	}

	var wg sync.WaitGroup
	for _, imageName := range imageNames {
		auths := getAuthsForImage(ctx, logger, &kr, imageName)
		for i, auth := range auths {
			wg.Add(1)
			request := &criV1.PullImageRequest{
				Image: &criV1.ImageSpec{
					Image: imageName,
				},
				Auth: auth,
			}
			go pullImageWithRetries(ctx, logger.With("image", imageName, "authNum", i), &wg, criClient, metricsSink.Chan(), imageName, request, timing)
		}
	}
	wg.Wait()
	logger.Info("pulling images finished")
	metricsSink.Await()
	if err := listImagesForDebugging(ctx, logger, criClient, timing.ImageListTimeout, "after"); err != nil {
		return fmt.Errorf("failed to list images for debugging after pulling: %w", err)
	}
	return nil
}

func listImagesForDebugging(ctx context.Context, logger *slog.Logger, client criV1.ImageServiceClient, timeout time.Duration, stage string) error {
	if !logger.Enabled(ctx, slog.LevelDebug) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	logger.DebugContext(ctx, "starting to list images")
	imagesResp, err := client.ListImages(ctx, &criV1.ListImagesRequest{})
	if err != nil {
		return fmt.Errorf("failed to call ListImages: %w", err)
	}
	logger.DebugContext(ctx, "finished listing images")
	for _, i := range imagesResp.Images {
		logger.DebugContext(ctx, "image present in runtime", "image", i, "stage", stage)
	}
	return nil
}

func loadPullSecret(logger *slog.Logger, kr *credentialprovider.BasicDockerKeyring, dockerConfigJSONPath string) error {
	if dockerConfigJSONPath == "" {
		logger.Info("no image pull secret path provided, will pull without credentials")
		return nil
	}
	f, err := os.ReadFile(dockerConfigJSONPath)
	if err != nil {
		return fmt.Errorf("failed read %q: %w", dockerConfigJSONPath, err)
	}
	dockerConfigJSON := credentialprovider.DockerConfigJSON{}
	if err := json.Unmarshal(f, &dockerConfigJSON); err != nil {
		return fmt.Errorf("unmarshalling docker config failed: %w", err)
	}
	kr.Add(dockerConfigJSON.Auths)
	return nil
}

func getAuthsForImage(ctx context.Context, logger *slog.Logger, kr credentialprovider.DockerKeyring, imageName string) []*criV1.AuthConfig {
	credsList, _ := kr.Lookup(imageName)
	var auths []*criV1.AuthConfig
	if len(credsList) == 0 {
		logger.DebugContext(ctx, "no credentials present for image", "image", imageName)
		// un-authenticated pull
		auths = append(auths, nil)
	}
	for _, creds := range credsList {
		auth := &criV1.AuthConfig{
			Username:      creds.Username,
			Password:      creds.Password,
			Auth:          creds.Auth,
			ServerAddress: creds.ServerAddress,
			IdentityToken: creds.IdentityToken,
			RegistryToken: creds.RegistryToken,
		}
		auths = append(auths, auth)
	}
	return auths
}

func pullImageWithRetries(ctx context.Context, logger *slog.Logger, wg *sync.WaitGroup, client criV1.ImageServiceClient, metricsSink chan<- *metricsProto.Result, name string, request *criV1.PullImageRequest, timing TimingConfig) {
	defer wg.Done()
	attemptTimeout := timing.InitialPullAttemptTimeout
	delay := timing.InitialPullAttemptDelay
	for {
		logger.Info("attempting image pull", "timeout", attemptTimeout)
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		start := time.Now()
		response, err := client.PullImage(attemptCtx, request)
		elapsed := time.Since(start)
		cancel()
		if err == nil {
			logger.InfoContext(ctx, "image pulled successfully", "response", response, "elapsed", elapsed)
			sizeBytes := getImageSize(ctx, logger, client, response)
			noteSuccess(metricsSink, name, start, elapsed, sizeBytes)
			return
		}
		logger.ErrorContext(ctx, "image failed to pull", "error", err, "timeout", attemptTimeout, "elapsed", elapsed)
		noteFailure(metricsSink, name, start, elapsed, err)
		if ctx.Err() != nil {
			logger.ErrorContext(ctx, "not retrying any more", "error", ctx.Err())
			return
		}
		// Be exponentially more patient on each attempt, but prevent overflows.
		attemptTimeout = min(attemptTimeout*2, timing.MaxPullAttemptTimeout)
		logger.InfoContext(ctx, "sleeping before retry", "timeout", delay)
		time.Sleep(delay)
		delay = min(delay*2, timing.MaxPullAttemptDelay)
	}
}

func getImageSize(ctx context.Context, logger *slog.Logger, client criV1.ImageServiceClient, response *criV1.PullImageResponse) uint64 {
	imageStatus, err := client.ImageStatus(ctx, &criV1.ImageStatusRequest{
		Image: &criV1.ImageSpec{
			Image: response.ImageRef,
		},
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to obtain pulled image status", "image", response.ImageRef, "error", err)
		return 0
	}
	return imageStatus.GetImage().GetSize_()
}

func noteSuccess(sink chan<- *metricsProto.Result, name string, start time.Time, elapsed time.Duration, sizeBytes uint64) {
	if sink == nil {
		return
	}
	sink <- &metricsProto.Result{
		AttemptId:  uuid.NewString(),
		StartedAt:  start.Unix(),
		Image:      name,
		DurationMs: uint64(elapsed.Milliseconds()),
		SizeBytes:  sizeBytes,
	}
}
func noteFailure(sink chan<- *metricsProto.Result, name string, start time.Time, elapsed time.Duration, err error) {
	if sink == nil {
		return
	}
	sink <- &metricsProto.Result{
		AttemptId:  uuid.NewString(),
		StartedAt:  start.Unix(),
		Image:      name,
		DurationMs: uint64(elapsed.Milliseconds()),
		Error:      err.Error(),
	}
}
