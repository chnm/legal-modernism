package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
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

	// There is no --reset. Re-deriving existing rows means TRUNCATE
	// moml_citations.citation_links from psql and then running this program
	// unchanged: a full rebuild takes about ten minutes, and the anti-join in
	// StreamUnprocessedCitations then resumes from wherever a previous job
	// stopped. A --reset flag could only ever delete the non-linked rows, so it
	// could not clear a stale link at all, and because it deleted at startup a
	// job that hit the wall time restarted from scratch (issue #294).
	slog.Info("processing settings", "batch_size", batchSize, "workers", workers)

	// Pre-load lookup tables into memory
	slog.Info("loading reporter whitelist")
	whitelist, err := store.GetReporterWhitelist(ctx)
	if err != nil {
		exitStartupError("could not load reporter whitelist", err)
	}
	slog.Info("loaded reporter whitelist", "entries", len(whitelist))

	slog.Info("loading diff-vols mapping")
	diffvols, err := store.GetDiffVols(ctx)
	if err != nil {
		exitStartupError("could not load diff-vols mapping", err)
	}
	slog.Info("loaded diff-vols mapping", "reporters", len(diffvols))

	slog.Info("loading CAP citations")
	capCites, err := store.LoadCAPCitations(ctx)
	if err != nil {
		exitStartupError("could not load CAP citations", err)
	}
	slog.Info("loaded CAP citations", "entries", len(capCites))

	slog.Info("loading FreeLaw cite crosswalk")
	freelawCites, err := store.LoadFreelawCites(ctx)
	if err != nil {
		exitStartupError("could not load FreeLaw cite crosswalk", err)
	}
	if len(freelawCites) == 0 {
		slog.Warn("FreeLaw cite crosswalk is empty; the FreeLaw fallback will do nothing — refresh the freelaw.cite_to_cap materialized view")
	}
	slog.Info("loaded FreeLaw cite crosswalk", "entries", len(freelawCites))

	slog.Info("loading reporter alternate abbreviations")
	altAbbrs, err := store.LoadReporterAltAbbrs(ctx)
	if err != nil {
		exitStartupError("could not load reporter alternate abbreviations", err)
	}
	altCount := 0
	for _, alts := range altAbbrs {
		altCount += len(alts)
	}
	// The alternate total is the number that shows #289's rule took effect: the
	// loader leaves out every alternate that is another reporter's standard.
	slog.Info("loaded reporter alternate abbreviations", "reporters", len(altAbbrs), "alternates", altCount)

	slog.Info("loading code reporter citations")
	codeCites, err := store.LoadCodeReporterCitations(ctx)
	if err != nil {
		exitStartupError("could not load code reporter citations", err)
	}
	slog.Info("loaded code reporter citations", "entries", len(codeCites))

	slog.Info("loading English Reports citations")
	erCites, err := store.LoadEnglishReportsCitations(ctx)
	if err != nil {
		exitStartupError("could not load English Reports citations", err)
	}
	erUnambiguous := 0
	for _, er := range erCites {
		if !er.Ambiguous {
			erUnambiguous++
		}
	}
	// The ambiguous count is the one number that shows #256's policy took effect,
	// so it is logged once at startup rather than left to be re-derived by query.
	slog.Info("loaded English Reports citations",
		"entries", len(erCites),
		"unambiguous", erUnambiguous,
		"ambiguous", len(erCites)-erUnambiguous)

	slog.Info("loading CAP case page spans")
	capSpans, err := store.LoadCAPCaseSpans(ctx)
	if err != nil {
		exitStartupError("could not load CAP case page spans", err)
	}
	slog.Info("loaded CAP case page spans", "entries", len(capSpans))

	slog.Info("loading English Reports case page spans")
	erSpans, err := store.LoadERCaseSpans(ctx)
	if err != nil {
		exitStartupError("could not load English Reports case page spans", err)
	}
	slog.Info("loaded English Reports case page spans", "entries", len(erSpans))

	// The stub registry is built from this program's own misses (make db-stubs),
	// so on the first run after a re-detection it is empty or stale; that is
	// expected, and the truncate-and-relink that follows db-stubs is what links
	// the citations to it. Warn rather than fail so the pipeline order is
	// visible in the log without blocking a run that does not need it.
	slog.Info("loading stub cases")
	stubs, err := store.LoadStubCases(ctx)
	if err != nil {
		exitStartupError("could not load stub cases", err)
	}
	if len(stubs) == 0 {
		slog.Warn("no stub cases loaded; citations to reporters no source covers stay no_match — run make db-stubs after this run, then truncate and relink")
	}
	slog.Info("loaded stub cases", "entries", len(stubs))

	// The years that refuse an anachronistic link (issue #319). The CAP map is
	// the large one, a year for each of the 6.9M cases.
	slog.Info("loading treatise and case years")
	years, err := linker.LoadYears(ctx, store)
	if err != nil {
		exitStartupError("could not load treatise and case years", err)
	}
	slog.Info("loaded treatise and case years",
		"treatises", len(years.Treatise), "cap_cases", len(years.CAP),
		"code_cases", len(years.Code), "er_cases", len(years.ER))

	// Assemble the lookup tables, which also walks every loaded cite string once
	// to build the reporter/volume indexes a no_match is attributed with, and the
	// page-range indexes that resolve pin cites.
	slog.Info("indexing cite strings by reporter and volume")
	tables := linker.NewTables(whitelist, diffvols, capCites, freelawCites, altAbbrs, codeCites, erCites, capSpans, erSpans, stubs, years)
	stats := tables.Stats()
	slog.Info("indexed cite strings",
		"us_reporters", stats.USReporters, "us_volumes", stats.USVolumes,
		"uk_reporters", stats.UKReporters, "uk_volumes", stats.UKVolumes)

	// The span arrays are large and fully consumed by the indexes; drop the
	// references so the 7M-element CAP slice can be collected before linking
	// starts rather than sitting alongside the maps for the whole run.
	capSpans, erSpans = nil, nil

	slog.Info("indexed case page spans",
		"cap_volumes", stats.CAPVolumes, "cap_spans", stats.CAPSpans,
		"er_volumes", stats.ERVolumes, "er_spans", stats.ERSpans)

	// A malformed span index mislinks silently and at scale, so verify the
	// invariant that has to hold by construction before any citation is linked.
	if err := tables.Check(); err != nil {
		exitStartupError("page span index is inconsistent", err)
	}

	// Bounded pipeline. A single streaming reader (this goroutine, inside
	// StreamUnprocessedCitations) feeds batches to a fixed pool of insert
	// workers through a bounded channel. The channel capacity bounds how many
	// batches are in flight, so the reader blocks — applying backpressure —
	// when the workers fall behind, instead of buffering the whole 62M-row
	// table in memory.
	batchCh := make(chan []citations.UnlinkedCitation, workers)
	var wg sync.WaitGroup
	var processed atomic.Int64
	var failedBatches atomic.Int64
	var failedRows atomic.Int64

	// Mark the transition out of the loading phase. Without this the log goes
	// quiet after the last lookup table is loaded, so there is no way to tell
	// that linking has actually begun.
	slog.Info("starting to link citations",
		"workers", workers, "batch_size", batchSize,
		"lock_timeout", lockTimeout.String(), "progress_every", progressInterval.String())

	stopHeartbeat := startProgressHeartbeat(progressInterval, &processed,
		func(n int64, elapsed time.Duration) {
			slog.Info("linking progress",
				"processed", n,
				"elapsed", elapsed.Round(time.Second).String(),
				"rows_per_sec", int64(float64(n)/elapsed.Seconds()))
		})

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range batchCh {
				select {
				case <-ctx.Done():
					continue // drain the channel without doing work
				default:
				}

				results := make([]*citations.LinkResult, len(batch))
				statusCounts := make(map[string]int)
				for j := range batch {
					r := tables.Link(&batch[j])
					results[j] = r
					statusCounts[r.Status]++
				}

				if err := store.SaveLinkResults(ctx, results); err != nil {
					if ctx.Err() != nil {
						// Shutting down. The insert was cancelled in flight, so
						// this batch is simply not committed and will be picked
						// up by the next run — not a failure worth an ERROR.
						slog.Warn("batch not saved because of shutdown", "size", len(results))
						continue
					}
					failedBatches.Add(1)
					failedRows.Add(int64(len(results)))
					slog.Error("could not save batch results", "size", len(results), "error", err)
					continue
				}

				processed.Add(int64(len(batch)))
				attrs := []any{"size", len(results)}
				for status, count := range statusCounts {
					attrs = append(attrs, status, count)
				}
				slog.Debug("saved batch", attrs...)
			}
		}()
	}

	// Stream the whole unprocessed set in one pass, pushing batches into the
	// bounded channel. The send blocks when the channel is full (backpressure).
	streamErr := store.StreamUnprocessedCitations(ctx, batchSize, func(batch []citations.UnlinkedCitation) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batchCh <- batch:
			return nil
		}
	})
	close(batchCh)
	wg.Wait()
	stopHeartbeat()

	// A shutdown signal cancels ctx, which surfaces as an error from whichever
	// query was in flight, so this must be checked before streamErr: the run was
	// interrupted, not broken. The count is a lower bound — a batch whose insert
	// committed on the server but whose response was never read (because the
	// context was cancelled first) is reported as unsaved and not counted. That
	// only ever undercounts, and re-processing is idempotent thanks to
	// ON CONFLICT (citation_id) DO NOTHING.
	if sig := stopSignal.Load(); sig != 0 {
		slog.Warn("interrupted before finishing; committed work is saved, resubmit to resume",
			"processed_at_least", processed.Load(),
			"signal", syscall.Signal(sig).String())
		os.Exit(signalExitBase + int(sig))
	}

	if streamErr != nil {
		slog.Error("streaming unprocessed citations failed", "processed", processed.Load(), "error", streamErr)
		os.Exit(1)
	}

	// A batch that could not be saved is left unprocessed rather than lost, but
	// the run must not report success: with --lock-timeout set, a blocking
	// transaction now turns an indefinite hang into dropped batches, and
	// swallowing that would trade a visible stall for a silent partial run.
	if n := failedBatches.Load(); n > 0 {
		slog.Error("finished with unsaved batches; re-run to pick them up",
			"processed", processed.Load(),
			"failed_batches", n,
			"failed_rows", failedRows.Load())
		os.Exit(1)
	}

	slog.Info("done linking citations", "processed", processed.Load())

	// Post-run database maintenance (vacuum/analyze the churned tables and
	// refresh the chambers dashboard materialized views) is run separately
	// (make db-maintenance / db/maintenance.sh), not by the linker.
}

// startProgressHeartbeat calls report every interval with the current count and
// the time elapsed since the heartbeat started, until the returned stop function
// is called. Reporting on a timer rather than per batch keeps the output
// readable in a Slurm log, and a count that does not move between consecutive
// reports is what makes a blocked run visible. stop blocks until the heartbeat
// goroutine has exited, so no report can be emitted after it returns.
func startProgressHeartbeat(interval time.Duration, processed *atomic.Int64, report func(n int64, elapsed time.Duration)) (stop func()) {
	started := time.Now()
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				report(processed.Load(), time.Since(started))
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}
