package cmd

import (
	"os"
	"strings"
	"time"

	"github.com/stackrox/image-prefetcher/internal"
	"github.com/stackrox/image-prefetcher/internal/logging"

	"github.com/spf13/cobra"
)

// fetchCmd represents the fetch command
var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch images using CRI.",
	Long: `This subcommand is intended to run in an init container of pods of a DaemonSet.

It talks to Container Runtime Interface API to pull images in parallel, with retries.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := logging.GetLogger()
		timing := internal.TimingConfig{
			ImageListTimeout:          imageListTimeout,
			InitialPullAttemptTimeout: initialPullAttemptTimeout,
			MaxPullAttemptTimeout:     maxPullAttemptTimeout,
			OverallTimeout:            overallTimeout,
			InitialPullAttemptDelay:   initialPullAttemptDelay,
			MaxPullAttemptDelay:       maxPullAttemptDelay,
		}
		imageList, err := loadImageNamesFromFile(imageListFile)
		if err != nil {
			return err
		}
		imageList = append(imageList, args...)
		return internal.Run(logger, criSocket, dockerConfigJSONPath, timing, metricsEndpoint, imageList...)
	},
}

var (
	criSocket                 string
	dockerConfigJSONPath      string
	imageListFile             string
	metricsEndpoint           string
	imageListTimeout          = time.Minute
	initialPullAttemptTimeout = 30 * time.Second
	maxPullAttemptTimeout     = 5 * time.Minute
	overallTimeout            = 20 * time.Minute
	initialPullAttemptDelay   = time.Second
	maxPullAttemptDelay       = 10 * time.Minute
)

func init() {
	rootCmd.AddCommand(fetchCmd)
	logging.AddFlags(fetchCmd.Flags())

	fetchCmd.Flags().StringVar(&criSocket, "cri-socket", "/run/containerd/containerd.sock", "Path to CRI UNIX socket.")
	fetchCmd.Flags().StringVar(&dockerConfigJSONPath, "docker-config", "", "Path to docker config json file.")
	fetchCmd.Flags().StringVar(&imageListFile, "image-list-file", "", "Path to text file containing images to pull (one per line).")
	fetchCmd.Flags().StringVar(&metricsEndpoint, "metrics-endpoint", "", "A host:port to submit image pull metrics to.")

	fetchCmd.Flags().DurationVar(&imageListTimeout, "image-list-timeout", imageListTimeout, "Timeout for image list calls (for debugging).")
	fetchCmd.Flags().DurationVar(&initialPullAttemptTimeout, "initial-pull-attempt-timeout", initialPullAttemptTimeout, "Timeout for initial image pull call. Each subsequent attempt doubles it until max.")
	fetchCmd.Flags().DurationVar(&maxPullAttemptTimeout, "max-pull-attempt-timeout", maxPullAttemptTimeout, "Maximum timeout for image pull call.")
	fetchCmd.Flags().DurationVar(&overallTimeout, "overall-timeout", overallTimeout, "Overall timeout for a single run.")
	fetchCmd.Flags().DurationVar(&initialPullAttemptDelay, "initial-pull-attempt-delay", initialPullAttemptDelay, "Timeout for initial delay between pulls of the same image. Each subsequent attempt doubles it until max.")
	fetchCmd.Flags().DurationVar(&maxPullAttemptDelay, "max-pull-attempt-delay", maxPullAttemptDelay, "Maximum delay between pulls of the same image.")
}

func loadImageNamesFromFile(fileName string) ([]string, error) {
	if fileName == "" {
		return nil, nil
	}
	bytes, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	return parseImageNames(bytes), nil
}

func parseImageNames(bytes []byte) []string {
	var imageNames []string
	for _, line := range strings.Split(string(bytes), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		imageNames = append(imageNames, line)
	}
	return imageNames
}
