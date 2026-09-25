// Chambers is the project's internal web app for browsing the citation data:
// the treatises of the Making of Modern Law (works, editions, volumes, pages),
// the cases they cite, the reporters those cases are cited from, the citations
// themselves, and how the detector and linker fared. See README.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lmullen/legal-modernism/go/db"
	flag "github.com/spf13/pflag"
)

func init() {
	initLogger()
}

func main() {
	var port int
	flag.IntVar(&port, "port", 4567, "port to listen on")
	flag.Parse()

	slog.Info("starting chambers")

	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(quit)
		cancel()
	}()

	pool, err := db.Connect(ctx)
	if err != nil {
		slog.Error("error connecting to database", "database", db.Host(), "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to database", "database", db.Host())

	tmpls := parseTemplates()
	slog.Debug("parsed templates", "count", len(tmpls))

	s := newServer(pool, tmpls)
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		select {
		case <-quit:
			slog.Info("shutting down server")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				slog.Error("error shutting down server", "error", err)
			}
			cancel()
		case <-ctx.Done():
		}
	}()

	slog.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
