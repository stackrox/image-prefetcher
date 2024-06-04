package cmd

import (
	"github.com/stackrox/image-prefetcher/internal/logging"
	"github.com/stackrox/image-prefetcher/internal/metrics/server"

	"github.com/spf13/cobra"
)

// aggregateMetricsCmd represents the aggregate-metrics command
var aggregateMetricsCmd = &cobra.Command{
	Use:   "aggregate-metrics",
	Short: "Accept metrics submissions and serve them.",
	Long: `This subcommand is intended to run in a single pod.

It serves:
- a gRPC endpoint to which individual metrics can be submitted,
- an HTTP endpoint from which the aggregate metrics can be fetched.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.Run(logging.GetLogger(), grpcPort, httpPort)
	},
}

var (
	grpcPort int
	httpPort int
)

func init() {
	rootCmd.AddCommand(aggregateMetricsCmd)
	logging.AddFlags(aggregateMetricsCmd.Flags())
	aggregateMetricsCmd.Flags().IntVar(&grpcPort, "grpc-port", 8443, "Port for metrics submission gRPC endpoint to listen on.")
	aggregateMetricsCmd.Flags().IntVar(&httpPort, "http-port", 8080, "Port for metrics retrieval HTTP endpoint to listen on.")
}
