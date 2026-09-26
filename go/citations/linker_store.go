package citations

import (
	"context"
)

// LinkerStore is the lookup tables every linker loads before it links anything:
// the whitelist, the cite strings of every source, the page spans, the stub
// registry and the years. None of them belongs to a corpus, which is why
// cite-linker and cite-linker-cap share one implementation. The corpus's own
// tables, where the citations come from and the results go, are a CorpusStore.
type LinkerStore interface {
	// GetReporterWhitelist loads the full reporter whitelist into memory.
	GetReporterWhitelist(ctx context.Context) (map[string]*WhitelistEntry, error)

	// GetDiffVols loads the volume mapping for reporters with different numbering.
	// The outer key is reporter_standard, inner key is original volume number.
	GetDiffVols(ctx context.Context) (map[string]map[int]*DiffVolEntry, error)

	// LoadCAPCitations loads CAP citations into memory as cite -> case ID.
	// Cites that belong to more than one case are dropped, mirroring
	// freelaw.cite_to_cap, so an ambiguous cite is a miss rather than a link
	// to an arbitrary case.
	LoadCAPCitations(ctx context.Context) (map[string]int64, error)

	// LoadFreelawCites loads the FreeLaw parallel-citation crosswalk
	// (freelaw.cite_to_cap) into memory as cite -> cap_case_id. The linker uses
	// it as a fallback after the exact cap.citations lookup misses.
	LoadFreelawCites(ctx context.Context) (map[string]int64, error)

	// LoadReporterAltAbbrs loads legalhist.reporters_abbreviations into memory as
	// reporter_standard -> list of alternate abbreviations, in a deterministic
	// order. The linker probes the CAP and FreeLaw maps with each alternate
	// spelling after the canonical reporter_standard / reporter_cap forms miss,
	// recovering matches where our reporter string and the other source's
	// disagree. An alternate that is itself a reporter_standard in
	// legalhist.reporters is not loaded: a standard spelling trumps an
	// alternative one (issue #289).
	LoadReporterAltAbbrs(ctx context.Context) (map[string][]string, error)

	// LoadCodeReporterCitations loads code reporter citations into memory as
	// cite -> id, keyed by both the official citation and the individual
	// parallel citations. Cites that belong to more than one row are dropped,
	// as in LoadCAPCitations.
	LoadCodeReporterCitations(ctx context.Context) (map[string]int64, error)

	// LoadEnglishReportsCitations loads all English Reports citations into memory
	// as cite string -> ERCase, keyed by both the E.R. reprint cite and the
	// nominate parallel cite. Unlike LoadCAPCitations, a cite belonging to more
	// than one case is kept as a key and marked ambiguous rather than dropped, so
	// the linker can distinguish a cite it cannot resolve from one it never saw.
	LoadEnglishReportsCitations(ctx context.Context) (map[string]ERCase, error)

	// LoadCAPCaseSpans loads every CAP first-page cite together with the page
	// length of the case it names, for the page-range index that resolves pin
	// cites. Unlike LoadCAPCitations this keeps ambiguous cites: two cases sharing
	// a first page is exactly what the index has to detect in order to refuse the
	// match, so dropping them here would turn a knowable ambiguity into a silent
	// wrong answer.
	LoadCAPCaseSpans(ctx context.Context) ([]CaseSpan[int64], error)

	// LoadERCaseSpans loads every English Reports first-page cite — both er_cite
	// and er_parallel_cite — for the same purpose. Length is always 0: the table
	// records no page range, so spans there can only be bounded by the next cite.
	LoadERCaseSpans(ctx context.Context) ([]CaseSpan[string], error)

	// LoadStubCases loads the keys of legalhist.stub_cases as a set: the cite
	// strings of cases no source holds but that the corpus cites often enough
	// to treat as real (issue #248). The keys take the "{volume}
	// {reporter_standard} {page}" form of cite_cleaned, which is what the linker
	// probes with. An empty set is not an error; it is the state before the
	// registry has been built (make db-stubs).
	LoadStubCases(ctx context.Context) (map[string]struct{}, error)

	// LoadTreatiseYears loads the year each MOML treatise volume was published
	// (moml.volumes.year), keyed by psmid, which is what
	// citations_unlinked.moml_treatise holds. The linker refuses a link to a
	// case decided after that year (issue #319). A volume with no year is left
	// out, so its citations are never refused.
	LoadTreatiseYears(ctx context.Context) (map[string]int, error)

	// LoadCAPCaseYears loads cap.cases.decision_year keyed by case id, for the
	// same test.
	LoadCAPCaseYears(ctx context.Context) (map[int64]int, error)

	// LoadCodeReporterYears loads legalhist.code_reporter.decision_year keyed
	// by id, for the same test.
	LoadCodeReporterYears(ctx context.Context) (map[int64]int, error)

	// LoadERCaseYears loads the year of each English Reports case keyed by id,
	// for the same test: murrell_year, falling back to er_year where Murrell
	// gives none.
	LoadERCaseYears(ctx context.Context) (map[string]int, error)
}
