package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/lmullen/legal-modernism/go/db"
	flag "github.com/spf13/pflag"
)

// progressInterval is how often --progress logs a heartbeat. A full run
// processes ~62M citations over several hours, so this is frequent enough to
// extrapolate a finish time without making the log noisy.
const progressInterval = 5 * time.Minute

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
	var showProgress bool
	var reset bool
	var batchSize int
	var workers int
	flag.BoolVar(&showProgress, "progress", false, "log a progress heartbeat (count, elapsed, rows/sec) every 5 minutes")
	flag.BoolVar(&reset, "reset", false, "before linking, delete every non-linked citation_links row (status no_match, skipped_not_whitelisted, skipped_junk) so they are re-processed; only linked_* rows are kept")
	flag.IntVar(&batchSize, "batch-size", 5000, "number of citations per insert batch")
	flag.IntVar(&workers, "workers", 32, "number of concurrent insert workers (each uses one DB connection)")
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
	})
	if err != nil {
		exitStartupError("could not connect to database", err, "database", db.Host())
	}
	defer pool.Close()
	slog.Info("connected to the database", "database", db.Host())

	store := citations.NewLinkerDBStore(pool)

	// Handle --reset: delete every non-linked row (no_match,
	// skipped_not_whitelisted, skipped_junk) so they are re-processed by this run.
	// Only linked_* rows are preserved. Done before everything else so the linking
	// below sees the post-reset state.
	if reset {
		slog.Info("resetting unresolved citation links (no_match, skipped_not_whitelisted, skipped_junk)")
		deleted, err := store.ResetUnlinked(ctx)
		if err != nil {
			exitStartupError("reset failed", err, "deleted", deleted)
		}
		slog.Info("reset complete", "deleted", deleted)
	}

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
	slog.Info("loaded reporter alternate abbreviations", "reporters", len(altAbbrs))

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
	slog.Info("loaded English Reports citations", "entries", len(erCites))

	// Bounded pipeline. A single streaming reader (this goroutine, inside
	// StreamUnprocessedCitations) feeds batches to a fixed pool of insert
	// workers through a bounded channel. The channel capacity bounds how many
	// batches are in flight, so the reader blocks — applying backpressure —
	// when the workers fall behind, instead of buffering the whole 62M-row
	// table in memory.
	batchCh := make(chan []citations.UnlinkedCitation, workers)
	var wg sync.WaitGroup
	var processed atomic.Int64

	stopHeartbeat := func() {}
	if showProgress {
		stopHeartbeat = startProgressHeartbeat(progressInterval, &processed,
			func(n int64, elapsed time.Duration) {
				slog.Info("linking progress",
					"processed", n,
					"elapsed", elapsed.Round(time.Second).String(),
					"rows_per_sec", int64(float64(n)/elapsed.Seconds()))
			})
	}

	// Mark the transition out of the loading phase. Without this the log goes
	// quiet after the last lookup table is loaded and stays quiet until the first
	// heartbeat, so there is no way to tell that linking has actually begun.
	startAttrs := []any{"workers", workers, "batch_size", batchSize}
	if showProgress {
		startAttrs = append(startAttrs, "progress_every", progressInterval.String())
	}
	slog.Info("starting to link citations", startAttrs...)

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
					r := linkCitation(&batch[j], whitelist, diffvols, capCites, freelawCites, altAbbrs, codeCites, erCites)
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
					slog.Error("could not save batch results", "error", err)
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

	slog.Info("done linking citations", "processed", processed.Load())

	// Post-run database maintenance (vacuum/analyze the churned tables and
	// refresh the chambers dashboard materialized views) is run separately
	// (make db-maintenance / db/maintenance.sh), not by the linker.
}

// startProgressHeartbeat calls report every interval with the current count and
// the time elapsed since the heartbeat started, until the returned stop function
// is called. This is what --progress does: a full run takes hours and per-batch
// results are only logged at DEBUG, so otherwise the job is silent from the
// start of linking until it finishes and an operator can't tell a stalled run
// from a slow one. Reporting on a timer rather than per batch keeps the output
// readable in a Slurm log. stop blocks until the heartbeat goroutine has exited,
// so no report can be emitted after it returns.
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

// linkCitation processes a single citation through the linking pipeline.
// All lookups are in-memory map accesses — no database queries.
func linkCitation(
	c *citations.UnlinkedCitation,
	whitelist map[string]*citations.WhitelistEntry,
	diffvols map[string]map[int]*citations.DiffVolEntry,
	capCites map[string]int64,
	freelawCites map[string]int64,
	altAbbrs map[string][]string,
	codeCites map[string]int64,
	erCites map[string]string,
) *citations.LinkResult {
	result := &citations.LinkResult{CitationID: c.ID}

	// Step 1: whitelist check
	entry, ok := whitelist[c.ReporterAbbr]
	if !ok {
		result.Status = citations.StatusSkippedNotWhitelisted
		return result
	}
	if entry.Junk {
		result.Status = citations.StatusSkippedJunk
		return result
	}

	// If there's no standard reporter, we can't normalize the citation
	if entry.ReporterStandard == nil {
		result.Status = citations.StatusNoMatch
		return result
	}

	// Step 2: route by UK flag
	if entry.UK {
		return linkEnglishReports(c, entry, erCites, result)
	}
	return linkCAPThenCode(c, entry, diffvols, capCites, freelawCites, altAbbrs, codeCites, result)
}

// linkCAPThenCode tries CAP first, then the FreeLaw parallel-citation crosswalk
// (which also resolves to a CAP case), then the FreeLaw crosswalk again under
// alternate reporter spellings, then the Code Reporter, all using in-memory maps.
func linkCAPThenCode(
	c *citations.UnlinkedCitation,
	entry *citations.WhitelistEntry,
	diffvols map[string]map[int]*citations.DiffVolEntry,
	capCites map[string]int64,
	freelawCites map[string]int64,
	altAbbrs map[string][]string,
	codeCites map[string]int64,
	result *citations.LinkResult,
) *citations.LinkResult {

	citeCleaned := buildStandardCite(c, entry)
	citeNormalized := buildCAPCite(c, entry, diffvols)
	result.CiteCleaned = &citeCleaned
	result.CiteNormalized = &citeNormalized

	// Run the whole cascade for the form we detected before trying the volume
	// variant, so an existing link can never be rewired: the variant only ever
	// turns a no_match into a link.
	for _, f := range volumeForms(c, entry) {
		cleaned := buildStandardCite(f, entry)
		normalized := buildCAPCite(f, entry, diffvols)

		// Try CAP with the normalized cite
		if caseID, ok := capCites[normalized]; ok {
			result.Status = citations.StatusLinkedCAP
			result.CAPCaseID = &caseID
			result.CiteLinked = &normalized
			return result
		}

		// Fall back to the FreeLaw crosswalk: if any parallel form of this decision
		// is in our CAP data, this reaches the CAP case from the form we detected.
		// The result is still a CAP link (status linked_cap).
		if caseID, ok := freelawCites[normalized]; ok {
			result.Status = citations.StatusLinkedCAP
			result.CAPCaseID = &caseID
			result.CiteLinked = &normalized
			return result
		}

		// Fall back to alternate reporter spellings: the same decision may be in the
		// FreeLaw crosswalk under a CourtListener spelling that differs from our
		// reporter_standard/reporter_cap. Probe each known alternate spelling for this
		// reporter (keyed by the canonical reporter_standard, like diffvols). The
		// first hit links to the CAP case (status linked_cap). Volume-nil is handled
		// the same way as buildStandardCite/buildCAPCite.
		for _, alt := range altAbbrs[*entry.ReporterStandard] {
			var altCite string
			if f.Volume == nil {
				altCite = fmt.Sprintf("%s %d", alt, f.Page)
			} else {
				altCite = fmt.Sprintf("%d %s %d", *f.Volume, alt, f.Page)
			}
			if caseID, ok := freelawCites[altCite]; ok {
				result.Status = citations.StatusLinkedCAP
				result.CAPCaseID = &caseID
				result.CiteLinked = &altCite
				return result
			}
		}

		// Try Code Reporter with the cleaned cite
		if codeID, ok := codeCites[cleaned]; ok {
			result.Status = citations.StatusLinkedCodeReporter
			result.CodeReporterID = &codeID
			result.CiteLinked = &cleaned
			return result
		}
	}

	result.Status = citations.StatusNoMatch
	return result
}

// linkEnglishReports tries to link a UK citation to the English Reports
// using an in-memory map.
func linkEnglishReports(
	c *citations.UnlinkedCitation,
	entry *citations.WhitelistEntry,
	erCites map[string]string,
	result *citations.LinkResult,
) *citations.LinkResult {
	citeCleaned := buildStandardCite(c, entry)
	result.CiteCleaned = &citeCleaned
	result.CiteNormalized = &citeCleaned

	// The English Reports are inconsistent about the redundant volume on
	// single-volume nominate reporters: most are stored bare ("Cro Eliz 1") but
	// some carry it ("1 Vern 1"), so try both forms.
	for _, f := range volumeForms(c, entry) {
		cite := buildStandardCite(f, entry)
		if erID, ok := erCites[cite]; ok {
			result.Status = citations.StatusLinkedEnglishReports
			result.ERCaseID = &erID
			result.CiteLinked = &cite
			return result
		}
	}

	result.Status = citations.StatusNoMatch
	return result
}

// volumeForms returns the citation forms to probe, most-faithful first. For a
// single-volume reporter "Toth 123" and "1 Toth 123" are the same citation --
// such reports were often cited with a redundant volume 1 even though there is
// only one volume to cite -- so both are tried. Everything else yields just the
// citation as detected.
//
// The variant is a copy of the citation with the volume flipped rather than a
// rewritten string, so buildStandardCite and buildCAPCite handle it with their
// existing reporter_cap and diffvols logic. That also reaches diffvols entries
// for volume 1, which buildCAPCite skips when the volume is nil.
func volumeForms(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry) []*citations.UnlinkedCitation {
	forms := []*citations.UnlinkedCitation{c}
	if !entry.SingleVol {
		return forms
	}

	variant := *c
	switch {
	case c.Volume == nil:
		one := 1
		variant.Volume = &one
	case *c.Volume == 1:
		variant.Volume = nil
	default:
		// Volume 2 or higher is a real volume number, not a redundant 1, so
		// there is nothing equivalent to try.
		return forms
	}
	return append(forms, &variant)
}

// buildStandardCite constructs "{volume} {reporter_standard} {page}".
func buildStandardCite(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry) string {
	if c.Volume == nil {
		return fmt.Sprintf("%s %d", *entry.ReporterStandard, c.Page)
	}
	return fmt.Sprintf("%d %s %d", *c.Volume, *entry.ReporterStandard, c.Page)
}

// buildCAPCite constructs the citation string appropriate for CAP lookup,
// handling reporters with different volume numbering.
func buildCAPCite(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry, diffvols map[string]map[int]*citations.DiffVolEntry) string {
	// If this reporter uses different volume numbers in CAP, try the diffvols mapping
	if entry.CAPDifferent && c.Volume != nil {
		if vols, ok := diffvols[*entry.ReporterStandard]; ok {
			if dv, ok := vols[*c.Volume]; ok {
				return fmt.Sprintf("%d %s %d", dv.CAPVol, dv.CAPReporter, c.Page)
			}
		}
	}

	// Use reporter_cap if available, otherwise fall back to reporter_standard
	reporter := *entry.ReporterStandard
	if entry.ReporterCAP != nil {
		reporter = *entry.ReporterCAP
	}

	if c.Volume == nil {
		return fmt.Sprintf("%s %d", reporter, c.Page)
	}
	return fmt.Sprintf("%d %s %d", *c.Volume, reporter, c.Page)
}
