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

	// LookupCAPCite looks up a normalized citation in cap.citations and returns
	// the case ID, or nil if not found.
	LookupCAPCite(ctx context.Context, cite string) (*int64, error)

	// LookupCodeReporter looks up a citation in legalhist.code_reporter and
	// returns the row ID, or nil if not found.
	LookupCodeReporter(ctx context.Context, cite string) (*int64, error)

	// LookupEnglishReports looks up a citation in english_reports.cases and
	// returns the case ID, or nil if not found.
	LookupEnglishReports(ctx context.Context, cite string) (*string, error)

	// SaveLinkResult saves the outcome of linking a single citation.
	SaveLinkResult(ctx context.Context, r *LinkResult) error

	// BatchSkipNonWhitelisted marks all non-whitelisted citations as skipped
	// in a single bulk operation. Returns the number of rows affected.
	BatchSkipNonWhitelisted(ctx context.Context) (int64, error)
}
