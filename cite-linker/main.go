package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/gammazero/workerpool"
	"github.com/google/uuid"
	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/lmullen/legal-modernism/go/db"
	"github.com/schollz/progressbar/v3"
	flag "github.com/spf13/pflag"
)

func main() {
	var showProgress bool
	var skipUnlisted bool
	var batchSize int
	var workers int
	flag.BoolVar(&showProgress, "progress", false, "show a progress bar")
	flag.BoolVar(&skipUnlisted, "skip-unlisted", false, "batch-mark non-whitelisted citations as skipped before linking")
	flag.IntVar(&batchSize, "batch-size", 5000, "number of citations to fetch per batch (max 8000)")
	flag.IntVar(&workers, "workers", runtime.NumCPU()*2, "number of concurrent workers")
	flag.Parse()

	if batchSize > 8000 {
		batchSize = 8000
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
	pool, err := db.Connect(ctx)
	if err != nil {
		slog.Error("could not connect to database", "database", db.Host(), "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to the database", "database", db.Host())

	store := citations.NewLinkerDBStore(pool)

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

	// Count unprocessed for progress bar
	total, err := store.CountUnprocessedCitations(ctx)
	if err != nil {
		slog.Error("could not count unprocessed citations", "error", err)
		os.Exit(1)
	}
	slog.Info("unprocessed citations", "count", total)

	if total == 0 {
		slog.Info("no unprocessed citations, nothing to do")
		return
	}

	var pb *progressbar.ProgressBar
	if showProgress {
		pb = progressbar.NewOptions64(total,
			progressbar.OptionSetWriter(os.Stdout),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetPredictTime(true),
		)
	}

	wp := workerpool.New(workers)

	var pbMu sync.Mutex

	// Process in batches using cursor-based pagination.
	// Fetching is sequential (cursor requires it), but each batch is
	// handed off to a worker for in-memory linking + batch INSERT.
	lastID := uuid.Nil
	for {
		select {
		case <-ctx.Done():
			slog.Info("context cancelled, stopping")
			wp.StopWait()
			return
		default:
		}

		batch, err := store.GetUnprocessedCitations(ctx, lastID, batchSize)
		if err != nil {
			slog.Error("could not fetch batch", "after_id", lastID, "error", err)
			break
		}
		if len(batch) == 0 {
			break
		}

		lastID = batch[len(batch)-1].ID
		slog.Debug("fetched batch", "size", len(batch), "last_id", lastID)

		// Hand the batch to a worker
		batchCopy := batch
		wp.Submit(func() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			results := make([]*citations.LinkResult, len(batchCopy))
			var wg sync.WaitGroup
			for i := range batchCopy {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					results[idx] = linkCitation(&batchCopy[idx], whitelist, diffvols, capCites, codeCites, erCites)
				}(i)
			}
			wg.Wait()

			if err := store.SaveLinkResults(ctx, results); err != nil {
				slog.Error("could not save batch results", "error", err)
			} else {
				statusCounts := make(map[string]int)
				for _, r := range results {
					statusCounts[r.Status]++
				}
				attrs := []any{"size", len(results)}
				for status, count := range statusCounts {
					attrs = append(attrs, status, count)
				}
				slog.Debug("saved batch", attrs...)
			}

			if pb != nil {
				pbMu.Lock()
				pb.Add(len(batchCopy))
				pbMu.Unlock()
			}
		})
	}

	wp.StopWait()
	slog.Info("done linking citations")
}

// linkCitation processes a single citation through the linking pipeline.
// All lookups are in-memory map accesses — no database queries.
func linkCitation(
	c *citations.UnlinkedCitation,
	whitelist map[string]*citations.WhitelistEntry,
	diffvols map[string]map[int]*citations.DiffVolEntry,
	capCites map[string]int64,
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
	if entry.Statute {
		result.Status = citations.StatusSkippedStatute
		return result
	}
	if entry.Junk {
		result.Status = citations.StatusSkippedJunk
		return result
	}

	// If there's no standard reporter, we can't normalize the citation
	if entry.ReporterStandard == nil {
		result.Status = citations.StatusSkippedNoMatch
		return result
	}

	// Step 2: route by UK flag
	if entry.UK {
		return linkEnglishReports(c, entry, erCites, result)
	}
	return linkCAPThenCode(c, entry, diffvols, capCites, codeCites, result)
}

// linkCAPThenCode tries CAP first, then Code Reporter using in-memory maps.
func linkCAPThenCode(
	c *citations.UnlinkedCitation,
	entry *citations.WhitelistEntry,
	diffvols map[string]map[int]*citations.DiffVolEntry,
	capCites map[string]int64,
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

	// Try Code Reporter with the cleaned cite
	if codeID, ok := codeCites[citeCleaned]; ok {
		result.Status = citations.StatusLinkedCodeReporter
		result.CodeReporterID = &codeID
		result.CiteLinked = &citeCleaned
		return result
	}

	result.Status = citations.StatusSkippedNoMatch
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

	result.Status = citations.StatusSkippedNoMatch
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
