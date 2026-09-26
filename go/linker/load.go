package linker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/lmullen/legal-modernism/go/citations"
)

// Load reads every lookup table the cascade needs from store and assembles the
// Tables, checking the page-range indexes before returning them. It logs each
// step at INFO, as the drivers always have, because the loads take the first
// minutes of a run and a log that goes quiet there cannot be told from a hang.
// The error names the step that failed.
//
// The lookups are the same for every corpus: what differs between cite-linker
// and cite-linker-cap is only where the citations come from and where the
// results go, which is Run's Source.
func Load(ctx context.Context, store citations.LinkerStore) (*Tables, error) {
	slog.Info("loading reporter whitelist")
	whitelist, err := store.GetReporterWhitelist(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not load reporter whitelist: %w", err)
	}
	slog.Info("loaded reporter whitelist", "entries", len(whitelist))

	slog.Info("loading diff-vols mapping")
	diffvols, err := store.GetDiffVols(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not load diff-vols mapping: %w", err)
	}
	slog.Info("loaded diff-vols mapping", "reporters", len(diffvols))

	slog.Info("loading CAP citations")
	capCites, err := store.LoadCAPCitations(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not load CAP citations: %w", err)
	}
	slog.Info("loaded CAP citations", "entries", len(capCites))

	slog.Info("loading FreeLaw cite crosswalk")
	freelawCites, err := store.LoadFreelawCites(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not load FreeLaw cite crosswalk: %w", err)
	}
	if len(freelawCites) == 0 {
		slog.Warn("FreeLaw cite crosswalk is empty; the FreeLaw fallback will do nothing — refresh the freelaw.cite_to_cap materialized view")
	}
	slog.Info("loaded FreeLaw cite crosswalk", "entries", len(freelawCites))

	slog.Info("loading reporter alternate abbreviations")
	altAbbrs, err := store.LoadReporterAltAbbrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not load reporter alternate abbreviations: %w", err)
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
		return nil, fmt.Errorf("could not load code reporter citations: %w", err)
	}
	slog.Info("loaded code reporter citations", "entries", len(codeCites))

	slog.Info("loading English Reports citations")
	erCites, err := store.LoadEnglishReportsCitations(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not load English Reports citations: %w", err)
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
		return nil, fmt.Errorf("could not load CAP case page spans: %w", err)
	}
	slog.Info("loaded CAP case page spans", "entries", len(capSpans))

	slog.Info("loading English Reports case page spans")
	erSpans, err := store.LoadERCaseSpans(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not load English Reports case page spans: %w", err)
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
		return nil, fmt.Errorf("could not load stub cases: %w", err)
	}
	if len(stubs) == 0 {
		slog.Warn("no stub cases loaded; citations to reporters no source covers stay no_match — run make db-stubs after this run, then truncate and relink")
	}
	slog.Info("loaded stub cases", "entries", len(stubs))

	// The years that refuse an anachronistic link (issue #319). The CAP map is
	// the large one, a year for each of the 6.9M cases.
	slog.Info("loading treatise and case years")
	years, err := LoadYears(ctx, store)
	if err != nil {
		return nil, fmt.Errorf("could not load treatise and case years: %w", err)
	}
	slog.Info("loaded treatise and case years",
		"treatises", len(years.Treatise), "cap_cases", len(years.CAP),
		"code_cases", len(years.Code), "er_cases", len(years.ER))

	// Assemble the lookup tables, which also walks every loaded cite string once
	// to build the reporter/volume indexes a no_match is attributed with, and the
	// page-range indexes that resolve pin cites.
	slog.Info("indexing cite strings by reporter and volume")
	t := NewTables(whitelist, diffvols, capCites, freelawCites, altAbbrs, codeCites, erCites, capSpans, erSpans, stubs, years)
	stats := t.Stats()
	slog.Info("indexed cite strings",
		"us_reporters", stats.USReporters, "us_volumes", stats.USVolumes,
		"uk_reporters", stats.UKReporters, "uk_volumes", stats.UKVolumes)

	slog.Info("indexed case page spans",
		"cap_volumes", stats.CAPVolumes, "cap_spans", stats.CAPSpans,
		"er_volumes", stats.ERVolumes, "er_spans", stats.ERSpans)

	// A malformed span index mislinks silently and at scale, so verify the
	// invariant that has to hold by construction before any citation is linked.
	if err := t.Check(); err != nil {
		return nil, err
	}
	return t, nil
}
