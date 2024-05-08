package logging

import (
	"log/slog"
	"os"

	"github.com/spf13/pflag"
)

var debug bool

func GetLogger() *slog.Logger {
	opts := &slog.HandlerOptions{AddSource: true}
	if debug {
		opts.Level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

func AddFlags(flags *pflag.FlagSet) {
	flags.BoolVar(&debug, "debug", false, "Whether to enable debug logging.")
}
