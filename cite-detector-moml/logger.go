package main

import (
	"log/slog"
	"os"
)

// If the environment variable reads "debug" set the logging level to DEBUG;
// otherwise use INFO. Emit logs as JSON to stderr.
func initLogger() {
	env := os.Getenv("LAW_DEBUG")
	level := slog.LevelInfo
	if env == "debug" || env == "true" {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	handler := slog.NewJSONHandler(os.Stderr, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Debug("set up the logger", "level", level)
}

func init() {
	initLogger()
}
