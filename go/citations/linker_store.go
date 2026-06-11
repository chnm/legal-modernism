package citations

import (
	"context"

	"github.com/google/uuid"
)

// LinkerStore is an interface for the data operations needed by the cite-linker.
type LinkerStore interface {
	// GetReporterWhitelist loads the full reporter whitelist into memory.
	GetReporterWhitelist(ctx context.Context) (map[string]*WhitelistEntry, error)

	// GetDiffVols loads the volume mapping for reporters with different numbering.
	// The outer key is reporter_standard, inner key is original volume number.
	GetDiffVols(ctx context.Context) (map[string]map[int]*DiffVolEntry, error)

	// CountUnprocessedCitations returns the number of citations not yet in citation_links.
	CountUnprocessedCitations(ctx context.Context) (int64, error)

	// GetUnprocessedCitations fetches a batch of citations not yet linked,
	// starting after afterID (use uuid.Nil for the first batch).
	GetUnprocessedCitations(ctx context.Context, afterID uuid.UUID, limit int) ([]UnlinkedCitation, error)

	// LoadCAPCitations loads all CAP citations into memory as cite -> case ID.
	LoadCAPCitations(ctx context.Context) (map[string]int64, error)

	// LoadFreelawCites loads the FreeLaw parallel-citation crosswalk
	// (freelaw.cite_to_cap) into memory as cite -> cap_case_id. The linker uses
	// it as a fallback after the exact cap.citations lookup misses.
	LoadFreelawCites(ctx context.Context) (map[string]int64, error)

	// LoadCodeReporterCitations loads all code reporter citations into memory
	// as official_citation -> id.
	LoadCodeReporterCitations(ctx context.Context) (map[string]int64, error)

	// LoadEnglishReportsCitations loads all English Reports citations into memory
	// as cite string -> case ID.
	LoadEnglishReportsCitations(ctx context.Context) (map[string]string, error)

	// SaveLinkResults batch-inserts multiple link results in a single query.
	SaveLinkResults(ctx context.Context, results []*LinkResult) error

	// ResetUnlinked deletes citation_links rows that were not resolved to a case
	// (status no_match and skipped_not_whitelisted) so the linker re-processes
	// them, while preserving every linked_* row and skipped_junk row. Returns the
	// number of rows deleted.
	ResetUnlinked(ctx context.Context) (int64, error)

	// BatchSkipNonWhitelisted marks all non-whitelisted citations as skipped
	// in a single bulk operation. Returns the number of rows affected.
	BatchSkipNonWhitelisted(ctx context.Context) (int64, error)
}
