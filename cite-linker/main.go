package main

import (
	"context"
	"fmt"
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

	// Assemble the lookup tables, which also walks every loaded cite string once
	// to build the reporter/volume indexes a no_match is attributed with, and the
	// page-range indexes that resolve pin cites.
	slog.Info("indexing cite strings by reporter and volume")
	tables := newLinkTables(whitelist, diffvols, capCites, freelawCites, altAbbrs, codeCites, erCites, capSpans, erSpans, stubs)
	slog.Info("indexed cite strings",
		"us_reporters", len(tables.us.reporters), "us_volumes", len(tables.us.volumes),
		"uk_reporters", len(tables.uk.reporters), "uk_volumes", len(tables.uk.volumes))

	// The span arrays are large and fully consumed by the indexes; drop the
	// references so the 7M-element CAP slice can be collected before linking
	// starts rather than sitting alongside the maps for the whole run.
	capSpans, erSpans = nil, nil

	capVols, capSpanCount := tables.capRanges.size()
	erVols, erSpanCount := tables.erRanges.size()
	slog.Info("indexed case page spans",
		"cap_volumes", capVols, "cap_spans", capSpanCount,
		"er_volumes", erVols, "er_spans", erSpanCount)

	// A malformed span index mislinks silently and at scale, so verify the
	// invariant that has to hold by construction before any citation is linked.
	if err := tables.capRanges.checkSelfConsistency(); err != nil {
		exitStartupError("page span index is inconsistent", err, "index", "cap")
	}
	if err := tables.erRanges.checkSelfConsistency(); err != nil {
		exitStartupError("page span index is inconsistent", err, "index", "er")
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
					r := linkCitation(&batch[j], tables)
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

// linkTables holds every in-memory lookup table the cascade reads, together with
// the derived indexes that let a failure report which tier it reached. It is
// built once at startup and never written to afterwards, which is what makes it
// safe to share across the insert workers.
type linkTables struct {
	whitelist    map[string]*citations.WhitelistEntry
	diffvols     map[string]map[int]*citations.DiffVolEntry
	capCites     map[string]int64
	freelawCites map[string]int64
	altAbbrs     map[string][]string
	codeCites    map[string]int64
	erCites      map[string]citations.ERCase

	// us indexes the three maps the US cascade probes as a unit; uk indexes the
	// English Reports. Building them walks every cite string once, which is why
	// it happens here rather than per citation.
	us *citeIndex
	uk *citeIndex

	// capRanges and erRanges resolve pin cites — citations to an interior page of
	// a case — after the exact cascade has missed. Either may be nil, in which
	// case range matching is simply skipped.
	capRanges *rangeIndex[int64]
	erRanges  *rangeIndex[string]

	// stubs is the registry of cases no source holds (legalhist.stub_cases),
	// probed last and only for a citation whose reporter no source knows. It
	// is deliberately kept out of the us/uk cite indexes: a stub is evidence
	// that a case exists, not a source that holds it, and counting it as a
	// reached reporter would make reporter_absent -- the very condition a stub
	// depends on -- impossible to report. May be nil.
	stubs stubIndex
}

func newLinkTables(
	whitelist map[string]*citations.WhitelistEntry,
	diffvols map[string]map[int]*citations.DiffVolEntry,
	capCites map[string]int64,
	freelawCites map[string]int64,
	altAbbrs map[string][]string,
	codeCites map[string]int64,
	erCites map[string]citations.ERCase,
	capSpans []citations.CaseSpan[int64],
	erSpans []citations.CaseSpan[string],
	stubs map[string]struct{},
) *linkTables {
	return &linkTables{
		whitelist:    whitelist,
		diffvols:     diffvols,
		capCites:     capCites,
		freelawCites: freelawCites,
		altAbbrs:     altAbbrs,
		codeCites:    codeCites,
		erCites:      erCites,
		us:           newCiteIndex(capCites, freelawCites, codeCites),
		uk:           newCiteIndex(erCites),
		capRanges:    newRangeIndex(capSpans),
		erRanges:     newRangeIndex(erSpans),
		stubs:        stubIndex(stubs),
	}
}

// linkCitation processes a single citation through the linking pipeline.
// All lookups are in-memory map accesses — no database queries.
func linkCitation(c *citations.UnlinkedCitation, t *linkTables) *citations.LinkResult {
	result := &citations.LinkResult{CitationID: c.ID}

	// Step 1: whitelist check. None of the skips records a tier: the status is
	// already the whole explanation, and there was no cascade to reach a tier in.
	entry, ok := t.whitelist[c.ReporterAbbr]
	if !ok {
		result.Status = citations.StatusSkippedNotWhitelisted
		return result
	}
	if entry.Junk {
		result.Status = citations.StatusSkippedJunk
		return result
	}
	// A regnal-year statute ("13 Eliz. c. 5") is a real citation but not to a
	// case, so no source could hold it; skipping it here keeps it out of
	// no_match, where it would read as a coverage gap (issue #246). Checked
	// before routing because the statute rows carry a jurisdiction too.
	if entry.Statute {
		result.Status = citations.StatusSkippedStatute
		return result
	}

	// Past this point entry.ReporterStandard is never nil: non-junk whitelist
	// rows always have a standard reporter, enforced by the
	// chk_whitelist_nonjunk_has_standard constraint. A violation panics at the
	// derefs below rather than silently producing no_match.

	// Step 2: route by UK flag
	if entry.UK {
		return linkEnglishReports(c, entry, t, result)
	}
	return linkCAPThenCode(c, entry, t, result)
}

// linkCAPThenCode tries CAP first, then the FreeLaw parallel-citation crosswalk
// (which also resolves to a CAP case), then both again under alternate reporter
// spellings, then the Code Reporter, all using in-memory maps. The alternate spellings probe per map, not per alt: CAP is
// exhausted across every alternate before FreeLaw is consulted, because the
// source ranking is meaningful (CAP's own citation index over the FreeLaw
// cluster crosswalk, matching the direct-probe order) while the position of an
// alt in its list is not.
func linkCAPThenCode(
	c *citations.UnlinkedCitation,
	entry *citations.WhitelistEntry,
	t *linkTables,
	result *citations.LinkResult,
) *citations.LinkResult {

	citeCleaned := buildStandardCite(c, entry)
	citeNormalized := buildCAPCite(c, entry, t.diffvols)
	result.CiteCleaned = &citeCleaned
	result.CiteNormalized = &citeNormalized

	// The cite strings built from this citation's own reporter, in the order they
	// are probed, so a no_match can be attributed to the tier the cascade
	// actually reached rather than to a second, drifting reimplementation of
	// which forms get tried.
	//
	// Alternate spellings are deliberately NOT collected here, though they are
	// probed against the exact maps below. Two reasons, and either alone is
	// enough (issue #290):
	//
	// buildAltCites bypasses buildCAPCite's reporter_cap and diffvols handling,
	// because an alternate is the other source's own spelling and remapping its
	// volume would be wrong. That makes an alternate's volume number
	// untranslated, which is harmless for an exact probe -- a miss costs
	// nothing -- but not for containment, for exactly the reason diffvolsMissing
	// already suppresses range matching: the wrong volume of the right reporter
	// is densely populated, so containment would confidently return a case from
	// it.
	//
	// And an alternate may not name this reporter at all. 76 rows in
	// legalhist.reporters_abbreviations carry an alt_abbr that is itself another
	// reporter's reporter_standard (issue #289), so a probe under one asks the
	// index about a different reporter. Feeding those to the range index turned
	// them into links: CAP holds no official or nominative cite under "Am. Dec.",
	// "P." or "Paine", so the span index has no key for them at all, and every
	// one of their 281,132 cap_page_interior links came from an alternate. The
	// same probes reaching citeIndex made the failure tiers claim a reporter and
	// volume that belong to some other reporter.
	//
	// Keeping the alternates out also makes a page-interior link auditable
	// after the fact, which it was not: CiteLinked is nil on those rows, but the
	// link can now only have come from cite_cleaned or cite_normalized, and both
	// are recorded.
	probes := make([]string, 0, 4)

	// The standard-form strings alone, for the stub registry, which is keyed on
	// cite_cleaned: a CAP-spelled or volume-translated form is a different
	// reporter's string as far as the registry is concerned.
	standard := make([]string, 0, 2)

	// Run the whole cascade for the form we detected before trying the volume
	// variant, so an existing link can never be rewired: the variant only ever
	// turns a no_match into a link.
	for _, f := range volumeForms(c, entry) {
		cleaned := buildStandardCite(f, entry)
		normalized := buildCAPCite(f, entry, t.diffvols)
		probes = append(probes, normalized, cleaned)
		standard = append(standard, cleaned)

		// Try CAP with the normalized cite
		if caseID, ok := t.capCites[normalized]; ok {
			result.Status = citations.StatusLinkedCAP
			result.MatchTier = citations.TierCAPDirect
			result.CAPCaseID = &caseID
			result.CiteLinked = &normalized
			return result
		}

		// Fall back to the FreeLaw crosswalk: if any parallel form of this decision
		// is in our CAP data, this reaches the CAP case from the form we detected.
		// The result is still a CAP link (status linked_cap), distinguished from a
		// direct hit only by the tier.
		if caseID, ok := t.freelawCites[normalized]; ok {
			result.Status = citations.StatusLinkedCAP
			result.MatchTier = citations.TierCAPFreelaw
			result.CAPCaseID = &caseID
			result.CiteLinked = &normalized
			return result
		}

		// Fall back to alternate reporter spellings: the same decision may be in
		// CAP or the FreeLaw crosswalk under a spelling that differs from our
		// reporter_standard/reporter_cap. Probe each known alternate spelling for
		// this reporter (keyed by the canonical reporter_standard, like diffvols)
		// against CAP first, then all of them against FreeLaw. A hit links to the
		// CAP case (status linked_cap).
		altCites := buildAltCites(f, t.altAbbrs[*entry.ReporterStandard])
		for i := range altCites {
			if caseID, ok := t.capCites[altCites[i]]; ok {
				result.Status = citations.StatusLinkedCAP
				result.MatchTier = citations.TierCAPAltSpelling
				result.CAPCaseID = &caseID
				result.CiteLinked = &altCites[i]
				return result
			}
		}
		for i := range altCites {
			if caseID, ok := t.freelawCites[altCites[i]]; ok {
				result.Status = citations.StatusLinkedCAP
				result.MatchTier = citations.TierCAPFreelawAltSpelling
				result.CAPCaseID = &caseID
				result.CiteLinked = &altCites[i]
				return result
			}
		}

		// Try Code Reporter with the cleaned cite. There is deliberately no
		// alternate-spelling probe here: it never produced a link in the whole
		// history of the table, and legalhist.code_reporter holds 633 rows of
		// one New York series, so an alternate reporter spelling has nothing to
		// reach (issue #292).
		if codeID, ok := t.codeCites[cleaned]; ok {
			result.Status = citations.StatusLinkedCodeReporter
			result.MatchTier = citations.TierCodeDirect
			result.CodeReporterID = &codeID
			result.CiteLinked = &cleaned
			return result
		}
	}

	// Every exact form missed. Before giving up, try page-range matching: the
	// citation may be a pin cite to an interior page of a case, which no
	// first-page cite string can ever equal. This runs last so it can only turn a
	// no_match into a link, never rewire one the exact cascade already made.
	//
	// Skipped when diffvols is missing, for the same reason usTier reports that
	// tier ahead of the volume and page ones: the reporter renumbers in CAP and no
	// reporters_diffvols row covers this volume, so every probe carries a volume
	// number known to be untranslated. An exact miss on such a probe is harmless,
	// but a range hit is not — the wrong volume of the right reporter is densely
	// populated, so containment would confidently return a case from it. Measured
	// over the current no_match pool this suppresses 45,077 otherwise-plausible
	// links that would all have been fabricated.
	missingDiffvols := diffvolsMissing(c, entry, t.diffvols)
	span := rangeMiss
	if t.capRanges != nil && !missingDiffvols {
		caseID, outcome := t.capRanges.probe(probes)
		if outcome == rangeHit {
			result.Status = citations.StatusLinkedCAP
			result.MatchTier = citations.TierCAPPageInterior
			result.CAPCaseID = &caseID
			// No cite string matched, so there is nothing to record as the cite
			// that linked; CiteLinked stays nil and the tier is what identifies
			// how this row was made.
			return result
		}
		// A refusal is not a link but is still a finding, so it is carried to
		// usTier to sharpen the page step rather than dropped.
		span = outcome
	}

	// Last of all, the stub registry (issue #248), and only when the failure
	// would be reporter_absent: no probed spelling of this reporter is in any US
	// source, so there is no case this could have been and nothing a stub can
	// outrank. A deeper tier means a source does hold the reporter, and its
	// misses are coverage gaps or pin cites, which the registry is built to
	// exclude -- checking the tier here rather than trusting the registry keeps
	// that true even when the registry is stale.
	tier := usTier(probes, t.us, missingDiffvols, span)
	if tier == citations.TierUSReporterAbsent {
		if cite, ok := t.stubs.lookup(standard); ok {
			return linkStub(result, cite)
		}
	}

	result.Status = citations.StatusNoMatch
	result.MatchTier = tier
	return result
}

// diffvolsMissing reports whether this citation's reporter renumbers its volumes
// in CAP but no legalhist.reporters_diffvols row covers the cited volume — the
// case where buildCAPCite has to fall back to the untranslated volume number, so
// every probe built from it is a guess. A volume-less citation is not counted:
// there is no volume to translate, and buildCAPCite does not consult diffvols for
// one either.
func diffvolsMissing(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry, diffvols map[string]map[int]*citations.DiffVolEntry) bool {
	if !entry.CAPDifferent || c.Volume == nil {
		return false
	}
	_, ok := diffvols[*entry.ReporterStandard][*c.Volume]
	return !ok
}

// linkEnglishReports tries to link a UK citation to the English Reports
// using an in-memory map.
func linkEnglishReports(
	c *citations.UnlinkedCitation,
	entry *citations.WhitelistEntry,
	t *linkTables,
	result *citations.LinkResult,
) *citations.LinkResult {
	citeCleaned := buildStandardCite(c, entry)
	result.CiteCleaned = &citeCleaned
	result.CiteNormalized = &citeCleaned

	probes := make([]string, 0, 2)

	// Set when a probe matches a cite string that several English Reports cases
	// share. Recorded rather than returned on, because it is not a reason to stop
	// probing: on a single-volume reporter one volume form can collide while the
	// other resolves cleanly, and a real link is better evidence than a refusal.
	// Only after every form has missed does it decide the tier.
	ambiguous := false

	// The English Reports are inconsistent about the redundant volume on
	// single-volume nominate reporters: most are stored bare ("Cro Eliz 1") but
	// some carry it ("1 Vern 1"), so try both forms.
	for _, f := range volumeForms(c, entry) {
		cite := buildStandardCite(f, entry)
		probes = append(probes, cite)
		er, ok := t.erCites[cite]
		if !ok {
			continue
		}
		if er.Ambiguous {
			ambiguous = true
			continue
		}
		result.Status = citations.StatusLinkedEnglishReports
		result.MatchTier = citations.TierERDirect
		result.ERCaseID = &er.ID
		result.CiteLinked = &cite
		return result
	}

	// As on the US route, fall back to page-range matching for pin cites. The
	// English Reports record no page ranges of their own, so spans here are
	// bounded only by the next cite and maxSpanPages.
	span := rangeMiss
	if t.erRanges != nil {
		erID, outcome := t.erRanges.probe(probes)
		if outcome == rangeHit {
			result.Status = citations.StatusLinkedEnglishReports
			result.MatchTier = citations.TierERPageInterior
			result.ERCaseID = &erID
			return result
		}
		span = outcome
	}

	// The stub registry, under the same gate as the US route: the probes here
	// are already the standard forms the registry is keyed on.
	tier := ukTier(probes, t.uk, ambiguous, span)
	if tier == citations.TierUKReporterAbsent {
		if cite, ok := t.stubs.lookup(probes); ok {
			return linkStub(result, cite)
		}
	}

	result.Status = citations.StatusNoMatch
	result.MatchTier = tier
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

// buildAltCites constructs the alternate-spelling cite strings for a citation
// form, one per alternate abbreviation, in order; nil when there are none.
// Volume-nil is handled the same way as buildStandardCite. The alternates
// deliberately bypass buildCAPCite's reporter_cap/diffvols handling: they are
// the other source's own spellings, so remapping their volumes would be wrong.
func buildAltCites(c *citations.UnlinkedCitation, alts []string) []string {
	if len(alts) == 0 {
		return nil
	}
	altCites := make([]string, len(alts))
	for i, alt := range alts {
		if c.Volume == nil {
			altCites[i] = fmt.Sprintf("%s %d", alt, c.Page)
		} else {
			altCites[i] = fmt.Sprintf("%d %s %d", *c.Volume, alt, c.Page)
		}
	}
	return altCites
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
