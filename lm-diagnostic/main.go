package main

import (
	"context"
	"log/slog"
	"os"
	"runtime"

	"github.com/lmullen/legal-modernism/go/db"
)

func init() {
	initLogger()
}

func main() {
	slog.Debug("starting diagnostics")

	// Report OS, architecture, and Go version
	slog.Info("runtime", "os", runtime.GOOS, "arch", runtime.GOARCH, "go_version", runtime.Version())

	// Report the hostname
	host, err := os.Hostname()
	if err != nil {
		slog.Error("error getting hostname", "error", err)
		os.Exit(1)
	}
	slog.Info("hostname", "host", host)

	// Report CPU cores and GOMAXPROCS
	slog.Info("available CPUs", "cpu_cores", runtime.NumCPU(), "gomaxprocs", runtime.GOMAXPROCS(0))

	// Check environment variables
	_, debugSet := os.LookupEnv("LAW_DEBUG")
	if _, ok := os.LookupEnv("LAW_DBSTR"); !ok {
		slog.Error("required environment variable not set", "variable", "LAW_DBSTR")
		os.Exit(1)
	}
	slog.Info("environment variables set", "LAW_DBSTR", true, "LAW_DEBUG", debugSet)

	// Test database connectivity
	pool, err := db.Connect(context.Background())
	if err != nil {
		slog.Error("error connecting to database", "database", db.Host(), "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("successfully connected to database", "database", db.Host())

	slog.Debug("diagnostics complete")
}
