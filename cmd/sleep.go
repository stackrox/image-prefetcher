package cmd

import (
	"github.com/spf13/cobra"
	"time"
)

// sleepCmd represents the sleep command
var sleepCmd = &cobra.Command{
	Use:   "sleep",
	Short: "Sleep forever.",
	Long:  `This can be used as main container of a DaemonSet to avoid having to pull another image.`,
	Run: func(cmd *cobra.Command, args []string) {
		println("sleeping...")
		for {
			time.Sleep(time.Hour)
		}
	},
}

func init() {
	rootCmd.AddCommand(sleepCmd)
}
