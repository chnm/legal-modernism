package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/lmullen/legal-modernism/go/db"
	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/schollz/progressbar/v3"
	flag "github.com/spf13/pflag"
)

// maxDBConns caps how many connections the detector will open. PostgreSQL is
// configured with max_connections = 100 for every client of the database
// together, so a detector run that sized its pool to a large --workers would
// exhaust the server rather than go faster.
const maxDBConns = 64

func main() {
	showProgress := flag.Bool("progress", false, "show a progress bar (costs one count of moml.page_ocrtext at startup)")
	workers := flag.Int("workers", runtime.NumCPU(), "number of concurrent page workers")
	dbConns := flag.Int("db-conns", 0, "maximum database connections (default: workers plus one for the reader, capped at 64)")
	flag.Parse()

	if *workers < 1 {
		*workers = 1
	}

	// The pool is sized separately from the workers because the two are bounded
	// by different things. A worker costs a core; a connection costs one of the
	// server's max_connections, which is 100 for the whole database and shared
	// with everything else that talks to it. Running more workers than
	// connections is fine and deliberate -- a worker spends nearly all its time
	// in the regex scan, so a handful of connections serves many workers, and
	// one that finds the pool busy simply waits.
	maxConns := *workers + 1
	if maxConns > maxDBConns {
		maxConns = maxDBConns
	}
	if *dbConns > 0 {
		maxConns = *dbConns
	}

	slog.Info("starting the citation detector")
	slog.Info("CPUs", "available", runtime.NumCPU(), "workers", *workers, "db_conns", maxConns)

	// Create a context and listen for signals to gracefully shutdown the application
	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	// Clean up function that will be called at program end no matter what
	defer func() {
		signal.Stop(quit)
		cancel()
	}()
	// Listen for shutdown signals in a go routine and cancel context then
	go func() {
		select {
		case <-quit:
			slog.Info("quitting because shutdown signal received")
			cancel()
		case <-ctx.Done():
		}
	}()

	slog.Info("connecting to database", "database", db.Host())
	// One of these connections is held for the whole run by the streaming read;
	// the rest are what the workers insert through.
	pool, err := db.ConnectPool(ctx, func(c *pgxpool.Config) {
		c.MaxConns = int32(maxConns)
	})
	if err != nil {
		slog.Error("could not connect to database", "database", db.Host(), "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to the database", "database", db.Host())

	// Create the repositories
	sourcesDB := sources.NewPgxStore(pool)
	citationsDB := citations.NewDBStore(pool)

	// Create the detectors
	var detectors []*citations.Detector

	// Load the general-purpose detectors. The second finds citations whose
	// abbreviation the OCR corrupted by reading a letter as a digit ("F1ed."
	// for "Fed."), which the first cannot match at all. It scans separately so
	// that it can only add citations, never displace one.
	detectors = append(detectors, citations.GenericDetector, citations.GenericOCRDigitDetector)
	slog.Info("prepared general-purpose detectors", "num_detectors", len(detectors))

	// Create and load the single volume detectors. Each row is a
	// (reporter_standard, abbreviation) pair. The saved reporter_abbr is the
	// spelling that actually appeared in the OCR, not the reporter_standard the
	// detector was built from; cite-linker normalizes it through
	// legalhist.whitelist, so a spelling that belongs to a different reporter is
	// linked to that reporter instead of to this single volume.
	//
	// These detectors do not check what precedes the abbreviation, so they also
	// match inside longer citations ("Cal. 185" in "123 Cal. 185"). Those
	// shadows are dropped per page below, once every detector has run, by
	// citations.RemoveShadows.
	singleVolReporters, err := citationsDB.GetSingleVolReporterAbbrs(ctx)
	if err != nil {
		slog.Error("could not get single volume reporters from database", "error", err)
		os.Exit(1)
	}
	for _, sv := range singleVolReporters {
		d := citations.NewSingleVolDetector(sv.Standard, sv.Abbr)
		detectors = append(detectors, d)
	}
	slog.Info("prepared single volume detectors", "num_detectors", len(detectors))

	// Both loaders below are fatal. Continuing without the OCR corrections
	// would detect the whole corpus under different semantics than every
	// previous run, and continuing without the pages would leave nothing to do --
	// the stream would be empty and the run would log "done detecting citations"
	// and exit 0 after producing nothing (issue #285).
	slog.Info("getting OCR corrections")
	ocrSubs, err := sourcesDB.GetOCRSubstitutions(ctx)
	if err != nil {
		slog.Error("error getting OCR substitutions", "error", err)
		os.Exit(1)
	}
	slog.Info("loaded OCR corrections", "num_corrections", len(ocrSubs))
	// Built once and shared by every worker: the replacer is read-only, and
	// rebuilding it per page would repeat the sort 10.5M times.
	ocrReplacer := sources.NewOCRReplacer(ocrSubs)

	var pb *progressbar.ProgressBar
	if *showProgress {
		total, err := sourcesDB.CountTreatisePages(ctx)
		if err != nil {
			slog.Error("error counting treatise pages", "error", err)
			os.Exit(1)
		}
		pb = progressbar.Default(total)
	}

	// Bounded pipeline, the same shape cite-linker uses. One streaming reader
	// (this goroutine, inside StreamTreatisePages) feeds pages to a fixed pool
	// of workers through a bounded channel. The channel capacity bounds how many
	// pages are in flight, so the reader blocks -- applying backpressure -- when
	// the workers fall behind, instead of buffering the 25 GB corpus in memory.
	pageCh := make(chan *sources.TreatisePage, *workers)
	var wg sync.WaitGroup
	var processed, failedPages, savedCites atomic.Int64

	slog.Info("detecting citations on the treatise pages", "workers", *workers)

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range pageCh {
				select {
				case <-ctx.Done():
					continue // drain the channel without doing work
				default:
				}

				page.CorrectOCR(ocrReplacer)

				// Run every detector over the page before saving anything, so
				// that a single-volume match found inside a longer citation can
				// be recognized as a shadow of it and dropped.
				var found []*citations.Citation
				for _, detector := range detectors {
					found = append(found, detector.Detect(page)...)
				}
				kept := citations.RemoveShadows(found)
				if len(kept) < len(found) {
					slog.Debug("dropped shadow citations", "treatise_id", page.ParentID(), "page_id", page.ID(), "dropped", len(found)-len(kept))
				}

				// One insert per page rather than one per citation. Duplicate
				// spans -- which RemoveShadows deliberately keeps, because two
				// abbreviations that are prefixes of one another find the same
				// citation -- are collapsed by SaveCitations on the key of the
				// citations_unlinked_uq unique index before the write.
				if err := citationsDB.SaveCitations(ctx, kept); err != nil {
					if ctx.Err() != nil {
						slog.Warn("page not saved because of shutdown", "treatise_id", page.ParentID(), "page_id", page.ID())
						continue
					}
					failedPages.Add(1)
					slog.Error("could not save citations for page", "treatise_id", page.ParentID(), "page_id", page.ID(), "citations", len(kept), "error", err)
					continue
				}
				savedCites.Add(int64(len(kept)))
				processed.Add(1)
				if pb != nil {
					pb.Add(1)
				}
			}
		}()
	}

	streamErr := sourcesDB.StreamTreatisePages(ctx, func(page *sources.TreatisePage) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case pageCh <- page:
			return nil
		}
	})
	close(pageCh)
	wg.Wait()

	// A shutdown signal cancels ctx, which surfaces as an error from whichever
	// query was in flight, so it has to be checked before streamErr: the run was
	// interrupted, not broken. Committed pages are saved, and re-processing is
	// idempotent thanks to ON CONFLICT DO NOTHING, so the run is simply
	// resubmitted.
	if ctx.Err() != nil {
		slog.Warn("interrupted before finishing; committed work is saved, resubmit to resume",
			"pages_processed", processed.Load(), "citations_saved", savedCites.Load())
		os.Exit(1)
	}

	if streamErr != nil {
		slog.Error("streaming treatise pages failed", "pages_processed", processed.Load(), "error", streamErr)
		os.Exit(1)
	}

	// A page whose insert failed is left undetected rather than lost, but the
	// run must not report success -- swallowing that would turn a visible
	// failure into a silently partial corpus.
	if n := failedPages.Load(); n > 0 {
		slog.Error("finished with unsaved pages; re-run to pick them up",
			"pages_processed", processed.Load(), "failed_pages", n, "citations_saved", savedCites.Load())
		os.Exit(1)
	}

	slog.Info("done detecting citations",
		"pages_processed", processed.Load(), "citations_saved", savedCites.Load())
}
