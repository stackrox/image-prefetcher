package cmd

import (
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"syscall"
)

// sleepCmd represents the sleep command
var sleepCmd = &cobra.Command{
	Use:   "sleep",
	Short: "Sleep forever.",
	Long:  `This can be used as main container of a DaemonSet to avoid having to pull another image.`,
	Run: func(cmd *cobra.Command, args []string) {
		println("Sleeping...")
		cancelChan := make(chan os.Signal, 1)
		signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)
		s := <-cancelChan
		println("Terminating due to", s)
	},
}

func init() {
	rootCmd.AddCommand(sleepCmd)
}
