package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/lmullen/legal-modernism/go/db"
	"github.com/lmullen/legal-modernism/go/linker"
	flag "github.com/spf13/pflag"
)

// progressInterval is how often the linking loop logs a progress heartbeat. A
// frozen count across consecutive lines is the signal that the run is blocked;
// one minute is frequent enough to notice that within a Slurm job without
// making the log unreadable.
const progressInterval = 1 * time.Minute

// signalExitBase is added to the signal number to form the exit status of a run
// stopped by a shutdown signal, following the shell convention: 130 for SIGINT,
// 143 for SIGTERM. An interrupted run must be distinguishable from both a
// completed one (0) and a genuine failure (1), because it committed real work
// and should simply be resubmitted.
const signalExitBase = 128

// stopSignal holds the number of the shutdown signal that cancelled the run, or
// 0 if none was received. A signal cancels the context, which then surfaces as
// an error from whichever database call was in flight; consulting this instead
// of inspecting that error is what lets an interrupt be reported as an
// interrupt rather than as a failure.
var stopSignal atomic.Int64

// exitStartupError ends a run that failed before linking began. A shutdown
// signal during startup cancels the in-flight query, so without the stopSignal
// check a routine Ctrl-C during the minutes-long lookup-table load would exit 1
// and log an ERROR, indistinguishable from a real failure.
func exitStartupError(msg string, err error, attrs ...any) {
	if sig := stopSignal.Load(); sig != 0 {
		slog.Warn("interrupted during startup; no citations were linked",
			append([]any{"step", msg, "signal", syscall.Signal(sig).String()}, attrs...)...)
		os.Exit(signalExitBase + int(sig))
	}
	slog.Error(msg, append([]any{"error", err}, attrs...)...)
	os.Exit(1)
}

func main() {
	var batchSize int
	var workers int
	var lockTimeout time.Duration
	flag.IntVar(&batchSize, "batch-size", 5000, "number of citations per insert batch")
	flag.IntVar(&workers, "workers", 32, "number of concurrent insert workers (each uses one DB connection)")
	flag.DurationVar(&lockTimeout, "lock-timeout", time.Minute, "give up on a statement that waits this long for a database lock, instead of blocking forever behind an uncommitted transaction; 0 disables")
	flag.Parse()

	if batchSize < 1 {
		batchSize = 1
	}
	if workers < 1 {
		workers = 1
	}

	slog.Info("starting the citation linker")

	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(quit)
		cancel()
	}()
	go func() {
		select {
		case s := <-quit:
			if sig, ok := s.(syscall.Signal); ok {
				stopSignal.Store(int64(sig))
			}
			slog.Info("quitting because shutdown signal received", "signal", s.String())
			cancel()
		case <-ctx.Done():
		}
	}()

	slog.Info("connecting to database", "database", db.Host())
	// Size the pool to the insert workers plus one dedicated connection for the
	// long-lived streaming read, with a small margin. Without this the default
	// pool could starve either the reader or the workers and serialize inserts.
	maxConns := int32(workers + 2)
	pool, err := db.ConnectPool(ctx, func(c *pgxpool.Config) {
		c.MaxConns = maxConns
		// Without lock_timeout a batch that collides with an uncommitted
		// transaction — a psql or GUI session left mid-transaction on
		// citation_links — waits forever, and every worker piles up behind it
		// until the whole run is wedged with no error to show for it. Setting it
		// as a connection runtime parameter covers every statement on every
		// pooled connection, including the streaming read.
		if lockTimeout > 0 {
			c.ConnConfig.RuntimeParams["lock_timeout"] = strconv.FormatInt(lockTimeout.Milliseconds(), 10)
		}
	})
	if err != nil {
		exitStartupError("could not connect to database", err, "database", db.Host())
	}
	defer pool.Close()
	slog.Info("connected to the database", "database", db.Host())

	store := citations.NewLinkerDBStore(pool)
	src := citations.NewMOMLCorpusStore(pool)

	// There is no --reset. Re-deriving existing rows means TRUNCATE
	// moml_citations.citation_links from psql and then running this program
	// unchanged: a full rebuild takes about ten minutes, and the anti-join in
	// StreamUnprocessedCitations then resumes from wherever a previous job
	// stopped. A --reset flag could only ever delete the non-linked rows, so it
	// could not clear a stale link at all, and because it deleted at startup a
	// job that hit the wall time restarted from scratch (issue #294).
	slog.Info("processing settings", "batch_size", batchSize, "workers", workers)

	tables, err := linker.Load(ctx, store)
	if err != nil {
		exitStartupError("could not load the lookup tables", err)
	}

	// Mark the transition out of the loading phase. Without this the log goes
	// quiet after the last lookup table is loaded, so there is no way to tell
	// that linking has actually begun.
	slog.Info("starting to link citations",
		"workers", workers, "batch_size", batchSize,
		"lock_timeout", lockTimeout.String(), "progress_every", progressInterval.String())

	sum, streamErr := linker.Run(ctx, tables, src, linker.Options{
		BatchSize:     batchSize,
		Workers:       workers,
		ProgressEvery: progressInterval,
	})

	// A shutdown signal cancels ctx, which surfaces as an error from whichever
	// query was in flight, so this must be checked before streamErr: the run was
	// interrupted, not broken. The count is a lower bound — a batch whose insert
	// committed on the server but whose response was never read (because the
	// context was cancelled first) is reported as unsaved and not counted. That
	// only ever undercounts, and re-processing is idempotent thanks to
	// ON CONFLICT (citation_id) DO NOTHING.
	if sig := stopSignal.Load(); sig != 0 {
		slog.Warn("interrupted before finishing; committed work is saved, resubmit to resume",
			"processed_at_least", sum.Processed,
			"signal", syscall.Signal(sig).String())
		os.Exit(signalExitBase + int(sig))
	}

	if streamErr != nil {
		slog.Error("streaming unprocessed citations failed", "processed", sum.Processed, "error", streamErr)
		os.Exit(1)
	}

	// A batch that could not be saved is left unprocessed rather than lost, but
	// the run must not report success: with --lock-timeout set, a blocking
	// transaction now turns an indefinite hang into dropped batches, and
	// swallowing that would trade a visible stall for a silent partial run.
	if n := sum.FailedBatches; n > 0 {
		slog.Error("finished with unsaved batches; re-run to pick them up",
			"processed", sum.Processed,
			"failed_batches", n,
			"failed_rows", sum.FailedRows)
		os.Exit(1)
	}

	slog.Info("done linking citations", "processed", sum.Processed)

	// Post-run database maintenance (vacuum/analyze the churned tables and
	// refresh the chambers dashboard materialized views) is run separately
	// (make db-maintenance / db/maintenance.sh), not by the linker.
}
