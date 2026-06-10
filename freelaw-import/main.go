// Command freelaw-import loads CourtListener bulk data into the freelaw schema:
// a parallel-citation crosswalk (freelaw.citations) and a cluster-to-CAP-case
// crosswalk (freelaw.clusters_to_cap). Both files are streamed straight from
// their bzip2-compressed CSV form into a session-temp staging table via
// Postgres COPY, then transformed with INSERT … SELECT. These tables let the
// citation linker resolve a detected citation to a CAP case through
// CourtListener's parallel-citation data (issue #197).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lmullen/legal-modernism/go/db"
	flag "github.com/spf13/pflag"
)

func init() {
	initLogger()
}

func main() {
	var citationsPath string
	var clustersPath string
	var showProgress bool
	flag.StringVar(&citationsPath, "citations", "", "path to the citations-YYYY-MM-DD.csv.bz2 file")
	flag.StringVar(&clustersPath, "clusters", "", "path to the opinion-clusters-YYYY-MM-DD.csv.bz2 file")
	flag.BoolVar(&showProgress, "progress", false, "show a progress bar")
	flag.Parse()

	if citationsPath == "" && clustersPath == "" {
		slog.Error("provide at least one of --citations or --clusters")
		os.Exit(1)
	}

	slog.Info("starting the freelaw importer")

	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(quit)
		cancel()
	}()
	go func() {
		select {
		case <-quit:
			slog.Info("quitting because shutdown signal received")
			cancel()
		case <-ctx.Done():
		}
	}()

	slog.Info("connecting to database", "database", db.Host())
	pool, err := db.Connect(ctx)
	if err != nil {
		slog.Error("could not connect to database", "database", db.Host(), "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to the database", "database", db.Host())

	if citationsPath != "" {
		slog.Info("loading parallel-citation crosswalk", "path", citationsPath)
		if err := loadCitations(ctx, pool, citationsPath, showProgress); err != nil {
			slog.Error("could not load citations", "path", citationsPath, "error", err)
			os.Exit(1)
		}
		slog.Info("loaded parallel-citation crosswalk")
	}

	if clustersPath != "" {
		slog.Info("loading cluster-to-CAP crosswalk", "path", clustersPath)
		if err := loadClusters(ctx, pool, clustersPath, showProgress); err != nil {
			slog.Error("could not load clusters", "path", clustersPath, "error", err)
			os.Exit(1)
		}
		slog.Info("loaded cluster-to-CAP crosswalk")
	}

	slog.Info("freelaw import complete")
}
