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

// cite-detector-cap detects case citations in the text of the opinions in the
// Caselaw Access Project (cap.opinions) for cases decided in or before
// --max-year, and saves them to opinion_citations.citations_unlinked, the CAP
// twin of the table cite-detector-moml fills from the treatises (issue #74).
// The two programs share their detectors, their per-document detection and
// their shape; this one differs in what it streams, where it saves, and the
// year cutoff.

// maxDBConns caps how many connections the detector will open. PostgreSQL is
// configured with max_connections = 100 for every client of the database
// together, so a detector run that sized its pool to a large --workers would
// exhaust the server rather than go faster.
const maxDBConns = 64

func main() {
	showProgress := flag.Bool("progress", false, "show a progress bar (costs one pass over cap.opinions at startup)")
	workers := flag.Int("workers", runtime.NumCPU(), "number of concurrent opinion workers")
	dbConns := flag.Int("db-conns", 0, "maximum database connections (default: workers plus one for the reader, capped at 64)")
	maxYear := flag.Int("max-year", 1920, "detect in the opinions of cases decided in or before this year")
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

	slog.Info("starting the CAP citation detector", "max_year", *maxYear)
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

	// Create the repositories. The detections go to opinion_citations, the
	// CAP twin of moml_citations (issue #74).
	sourcesDB := sources.NewPgxStore(pool)
	citationsDB := citations.NewOpinionDBStore(pool)

	// The detectors, shared with cite-detector-moml so the two corpora are
	// detected under the same semantics. Fatal on failure: continuing without
	// the single-volume or year detectors would detect the whole corpus under
	// different semantics than every previous run.
	detectors, err := citations.LoadFinders(ctx, citationsDB)
	if err != nil {
		slog.Error("could not load the detectors", "error", err)
		os.Exit(1)
	}

	// Both loaders below are fatal. Continuing without the OCR corrections
	// would detect the corpus under different semantics than the treatises, and
	// continuing without the opinions would leave nothing to do -- the stream
	// would be empty and the run would log "done detecting citations" and exit
	// 0 after producing nothing (issue #285).
	//
	// The corrections are the ones built from the treatises' OCR
	// (legalhist.ocr_corrections), applied unchanged so that the two corpora are
	// detected alike; whether CAP's own OCR wants a table of its own is a
	// measurement still to make (issue #74).
	slog.Info("getting OCR corrections")
	ocrSubs, err := sourcesDB.GetOCRSubstitutions(ctx)
	if err != nil {
		slog.Error("error getting OCR substitutions", "error", err)
		os.Exit(1)
	}
	slog.Info("loaded OCR corrections", "num_corrections", len(ocrSubs))
	// Built once and shared by every worker: the replacer is read-only, and
	// rebuilding it per opinion would repeat the sort 1.6M times.
	ocrReplacer := sources.NewOCRReplacer(ocrSubs)

	var pb *progressbar.ProgressBar
	if *showProgress {
		total, err := sourcesDB.CountCAPOpinions(ctx, *maxYear)
		if err != nil {
			slog.Error("error counting CAP opinions", "error", err)
			os.Exit(1)
		}
		pb = progressbar.Default(total)
	}

	// Bounded pipeline, the same shape cite-detector-moml and cite-linker use.
	// One streaming reader (this goroutine, inside StreamCAPOpinions) feeds
	// opinions to a fixed pool of workers through a bounded channel. The channel
	// capacity bounds how many opinions are in flight, so the reader blocks --
	// applying backpressure -- when the workers fall behind, instead of
	// buffering the corpus in memory. An opinion is a few pages long (about 6 KB
	// on average, the longest before 1921 about 140 KB), so even the largest in
	// flight together are a few tens of megabytes.
	opinionCh := make(chan *sources.CAPOpinion, *workers)
	var wg sync.WaitGroup
	var processed, failedOpinions, savedCites atomic.Int64

	slog.Info("detecting citations in the CAP opinions", "workers", *workers, "max_year", *maxYear)

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for opinion := range opinionCh {
				select {
				case <-ctx.Done():
					continue // drain the channel without doing work
				default:
				}

				// Correct the OCR, move the Law Reports' series prefix behind the
				// volume, run every detector and drop the shadows (see
				// citations.DetectDocument). The unit of work is an opinion, a
				// few pages long, which RemoveShadows' quadratic comparison of
				// the citations found in it easily affords.
				kept, dropped := citations.DetectDocument(opinion, detectors, ocrReplacer)
				if dropped > 0 {
					slog.Debug("dropped shadow citations", append(opinion.LogID(), "dropped", dropped)...)
				}

				// One insert per opinion rather than one per citation. Duplicate
				// spans -- which RemoveShadows deliberately keeps, because two
				// abbreviations that are prefixes of one another find the same
				// citation -- are collapsed by SaveCitations on the key of the
				// citations_unlinked_uq unique index before the write; so is the
				// same cite repeated in one opinion, which the per-opinion key
				// makes one row.
				if err := citationsDB.SaveCitations(ctx, kept); err != nil {
					if ctx.Err() != nil {
						slog.Warn("opinion not saved because of shutdown", opinion.LogID()...)
						continue
					}
					failedOpinions.Add(1)
					slog.Error("could not save citations for opinion", append(opinion.LogID(), "citations", len(kept), "error", err)...)
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

	streamErr := sourcesDB.StreamCAPOpinions(ctx, *maxYear, func(opinion *sources.CAPOpinion) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case opinionCh <- opinion:
			return nil
		}
	})
	close(opinionCh)
	wg.Wait()

	// A shutdown signal cancels ctx, which surfaces as an error from whichever
	// query was in flight, so it has to be checked before streamErr: the run was
	// interrupted, not broken. Committed opinions are saved, and re-processing
	// is idempotent thanks to ON CONFLICT DO NOTHING, so the run is simply
	// resubmitted; like cite-detector-moml it has no resume point and rescans
	// from the first opinion.
	if ctx.Err() != nil {
		slog.Warn("interrupted before finishing; committed work is saved, resubmit to resume",
			"opinions_processed", processed.Load(), "citations_saved", savedCites.Load())
		os.Exit(1)
	}

	if streamErr != nil {
		slog.Error("streaming CAP opinions failed", "opinions_processed", processed.Load(), "error", streamErr)
		os.Exit(1)
	}

	// An opinion whose insert failed is left undetected rather than lost, but
	// the run must not report success -- swallowing that would turn a visible
	// failure into a silently partial corpus.
	if n := failedOpinions.Load(); n > 0 {
		slog.Error("finished with unsaved opinions; re-run to pick them up",
			"opinions_processed", processed.Load(), "failed_opinions", n, "citations_saved", savedCites.Load())
		os.Exit(1)
	}

	// The same last line cite-detector-moml logs, which scripts/pipeline.sh
	// looks for; only the count's name differs.
	slog.Info("done detecting citations",
		"opinions_processed", processed.Load(), "citations_saved", savedCites.Load())
}
