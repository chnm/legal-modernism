package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/lmullen/legal-modernism/go/db"
	"github.com/schollz/progressbar/v3"
	flag "github.com/spf13/pflag"
)

func main() {
	var showProgress bool
	var skipUnlisted bool
	var reset bool
	var batchSize int
	var workers int
	flag.BoolVar(&showProgress, "progress", false, "show a progress bar")
	flag.BoolVar(&skipUnlisted, "skip-unlisted", false, "batch-mark non-whitelisted citations as skipped before linking")
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
		case <-quit:
			slog.Info("quitting because shutdown signal received")
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
		slog.Error("could not connect to database", "database", db.Host(), "error", err)
		os.Exit(1)
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
			slog.Error("reset failed", "deleted", deleted, "error", err)
			os.Exit(1)
		}
		slog.Info("reset complete", "deleted", deleted)
	}

	// Handle --skip-unlisted: bulk skip, then continue to linking
	if skipUnlisted {
		slog.Info("batch-marking non-whitelisted citations as skipped")
		affected, err := store.BatchSkipNonWhitelisted(ctx)
		if err != nil {
			slog.Error("batch skip failed", "error", err)
			os.Exit(1)
		}
		slog.Info("batch skip complete", "rows_affected", affected)
	}

	slog.Info("processing settings", "batch_size", batchSize, "workers", workers)

	// Pre-load lookup tables into memory
	slog.Info("loading reporter whitelist")
	whitelist, err := store.GetReporterWhitelist(ctx)
	if err != nil {
		slog.Error("could not load reporter whitelist", "error", err)
		os.Exit(1)
	}
	slog.Info("loaded reporter whitelist", "entries", len(whitelist))

	slog.Info("loading diff-vols mapping")
	diffvols, err := store.GetDiffVols(ctx)
	if err != nil {
		slog.Error("could not load diff-vols mapping", "error", err)
		os.Exit(1)
	}
	slog.Info("loaded diff-vols mapping", "reporters", len(diffvols))

	slog.Info("loading CAP citations")
	capCites, err := store.LoadCAPCitations(ctx)
	if err != nil {
		slog.Error("could not load CAP citations", "error", err)
		os.Exit(1)
	}
	slog.Info("loaded CAP citations", "entries", len(capCites))

	slog.Info("loading FreeLaw cite crosswalk")
	freelawCites, err := store.LoadFreelawCites(ctx)
	if err != nil {
		slog.Error("could not load FreeLaw cite crosswalk", "error", err)
		os.Exit(1)
	}
	if len(freelawCites) == 0 {
		slog.Warn("FreeLaw cite crosswalk is empty; the FreeLaw fallback will do nothing — refresh the freelaw.cite_to_cap materialized view")
	}
	slog.Info("loaded FreeLaw cite crosswalk", "entries", len(freelawCites))

	slog.Info("loading code reporter citations")
	codeCites, err := store.LoadCodeReporterCitations(ctx)
	if err != nil {
		slog.Error("could not load code reporter citations", "error", err)
		os.Exit(1)
	}
	slog.Info("loaded code reporter citations", "entries", len(codeCites))

	slog.Info("loading English Reports citations")
	erCites, err := store.LoadEnglishReportsCitations(ctx)
	if err != nil {
		slog.Error("could not load English Reports citations", "error", err)
		os.Exit(1)
	}
	slog.Info("loaded English Reports citations", "entries", len(erCites))

	var pb *progressbar.ProgressBar
	if showProgress {
		// The unprocessed total is not pre-counted (an exact count is itself a
		// full anti-join), so the bar runs in count/rate mode without an ETA.
		pb = progressbar.NewOptions64(-1,
			progressbar.OptionSetWriter(os.Stdout),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
		)
	}

	// Bounded pipeline. A single streaming reader (this goroutine, inside
	// StreamUnprocessedCitations) feeds batches to a fixed pool of insert
	// workers through a bounded channel. The channel capacity bounds how many
	// batches are in flight, so the reader blocks — applying backpressure —
	// when the workers fall behind, instead of buffering the whole 62M-row
	// table in memory.
	batchCh := make(chan []citations.UnlinkedCitation, workers)
	var wg sync.WaitGroup
	var pbMu sync.Mutex
	var processed atomic.Int64

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
					r := linkCitation(&batch[j], whitelist, diffvols, capCites, freelawCites, codeCites, erCites)
					results[j] = r
					statusCounts[r.Status]++
				}

				if err := store.SaveLinkResults(ctx, results); err != nil {
					slog.Error("could not save batch results", "error", err)
					continue
				}

				processed.Add(int64(len(batch)))
				attrs := []any{"size", len(results)}
				for status, count := range statusCounts {
					attrs = append(attrs, status, count)
				}
				slog.Debug("saved batch", attrs...)

				if pb != nil {
					pbMu.Lock()
					pb.Add(len(batch))
					pbMu.Unlock()
				}
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

	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		slog.Error("streaming unprocessed citations failed", "processed", processed.Load(), "error", streamErr)
		os.Exit(1)
	}

	slog.Info("done linking citations", "processed", processed.Load())

	// Post-run database maintenance (vacuum/analyze the churned tables and
	// refresh the chambers dashboard materialized views) is run separately
	// (make db-maintenance / db/maintenance.sh), not by the linker.
}

// linkCitation processes a single citation through the linking pipeline.
// All lookups are in-memory map accesses — no database queries.
func linkCitation(
	c *citations.UnlinkedCitation,
	whitelist map[string]*citations.WhitelistEntry,
	diffvols map[string]map[int]*citations.DiffVolEntry,
	capCites map[string]int64,
	freelawCites map[string]int64,
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
	return linkCAPThenCode(c, entry, diffvols, capCites, freelawCites, codeCites, result)
}

// linkCAPThenCode tries CAP first, then the FreeLaw parallel-citation crosswalk
// (which also resolves to a CAP case), then the Code Reporter, all using
// in-memory maps.
func linkCAPThenCode(
	c *citations.UnlinkedCitation,
	entry *citations.WhitelistEntry,
	diffvols map[string]map[int]*citations.DiffVolEntry,
	capCites map[string]int64,
	freelawCites map[string]int64,
	codeCites map[string]int64,
	result *citations.LinkResult,
) *citations.LinkResult {

	citeCleaned := buildStandardCite(c, entry)
	citeNormalized := buildCAPCite(c, entry, diffvols)
	result.CiteCleaned = &citeCleaned
	result.CiteNormalized = &citeNormalized

	// Try CAP with the normalized cite
	if caseID, ok := capCites[citeNormalized]; ok {
		result.Status = citations.StatusLinkedCAP
		result.CAPCaseID = &caseID
		result.CiteLinked = &citeNormalized
		return result
	}

	// Fall back to the FreeLaw crosswalk: if any parallel form of this decision
	// is in our CAP data, this reaches the CAP case from the form we detected.
	// The result is still a CAP link (status linked_cap).
	if caseID, ok := freelawCites[citeNormalized]; ok {
		result.Status = citations.StatusLinkedCAP
		result.CAPCaseID = &caseID
		result.CiteLinked = &citeNormalized
		return result
	}

	// Try Code Reporter with the cleaned cite
	if codeID, ok := codeCites[citeCleaned]; ok {
		result.Status = citations.StatusLinkedCodeReporter
		result.CodeReporterID = &codeID
		result.CiteLinked = &citeCleaned
		return result
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

	if erID, ok := erCites[citeCleaned]; ok {
		result.Status = citations.StatusLinkedEnglishReports
		result.ERCaseID = &erID
		result.CiteLinked = &citeCleaned
		return result
	}

	result.Status = citations.StatusNoMatch
	return result
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
