package citations

import (
	"context"
)

// LinkerStore is an interface for the data operations needed by the cite-linker.
type LinkerStore interface {
	// GetReporterWhitelist loads the full reporter whitelist into memory.
	GetReporterWhitelist(ctx context.Context) (map[string]*WhitelistEntry, error)

	// GetDiffVols loads the volume mapping for reporters with different numbering.
	// The outer key is reporter_standard, inner key is original volume number.
	GetDiffVols(ctx context.Context) (map[string]map[int]*DiffVolEntry, error)

	// StreamUnprocessedCitations runs a single anti-join over the whole
	// citations_unlinked table and delivers every citation not yet in
	// citation_links to fn in batches of at most batchSize. The full set is read
	// in one streaming pass, so callers must apply their own backpressure inside
	// fn (e.g. a bounded channel) to avoid buffering the entire table in memory.
	StreamUnprocessedCitations(ctx context.Context, batchSize int, fn func([]UnlinkedCitation) error) error

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
	// order. The linker probes the CAP, FreeLaw, and code-reporter maps with each
	// alternate spelling after the canonical reporter_standard / reporter_cap
	// forms miss, recovering matches where our reporter string and the other
	// source's disagree.
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

	// SaveLinkResults batch-inserts multiple link results in a single query.
	SaveLinkResults(ctx context.Context, results []*LinkResult) error

	// ResetUnlinked deletes every citation_links row that was not resolved to a
	// case (every status in UnresolvedStatuses: no_match,
	// skipped_not_whitelisted, skipped_junk, skipped_statute) so the linker
	// re-processes them, preserving only linked_* rows. Returns the number of
	// rows deleted.
	ResetUnlinked(ctx context.Context) (int64, error)
}
