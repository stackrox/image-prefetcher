package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"log"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "image-prefetcher",
	Short: "An image prefetching utility.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Please use one of the subcommands. See --help")
	},
}

// Execute is the entry point to this program.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
