package citations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4/pgxpool"
)

// CorpusStore is one corpus's citation ledger: the citations_unlinked table
// its detector fills and the citation_links table its linker writes. The MOML
// treatises and the CAP opinions each have one (issue #74), with the same
// columns apart from the citing key, so the two stores differ only in their
// SQL. Everything a linker reads that is not about one corpus -- the whitelist,
// the CAP, FreeLaw, code reporter and English Reports cite strings, the page
// spans, the stubs and the years -- stays on LinkerDBStore.
type CorpusStore struct {
	DB     *pgxpool.Pool
	stream string // the anti-join that delivers the unprocessed citations
	insert string // the INSERT ... unnest that saves a batch of results
}

// NewMOMLCorpusStore returns the store over moml_citations: the citations
// cite-detector-moml finds on the pages of the treatises, and cite-linker's
// results.
func NewMOMLCorpusStore(db *pgxpool.Pool) *CorpusStore {
	return &CorpusStore{
		DB: db,
		stream: `
		SELECT cu.id, cu.moml_treatise, cu.moml_page, cu.raw, cu.volume, cu.reporter_abbr, cu.page, cu.year
		FROM moml_citations.citations_unlinked cu
		WHERE NOT EXISTS (
			SELECT 1 FROM moml_citations.citation_links cl WHERE cl.citation_id = cu.id
		)
		`,
		insert: linksInsertSQL("moml_citations.citation_links"),
	}
}

// linksInsertSQL is the insert every corpus's SaveLinkResults runs, against its
// own citation_links table. Every such table has PRIMARY KEY (citation_id), so
// the ON CONFLICT clause holds for all of them.
func linksInsertSQL(table string) string {
	return `
	INSERT INTO ` + table + `
		(citation_id, status, match_tier, cap_case_id, code_reporter_id, er_case_id, stub_cite, cite_cleaned, cite_normalized, cite_linked)
	SELECT u.citation_id::uuid, u.status, u.match_tier, u.cap_case_id, u.code_reporter_id, u.er_case_id, u.stub_cite, u.cite_cleaned, u.cite_normalized, u.cite_linked
	FROM unnest($1::text[], $2::text[], $3::text[], $4::bigint[], $5::bigint[], $6::text[], $7::text[], $8::text[], $9::text[], $10::text[])
		AS u(citation_id, status, match_tier, cap_case_id, code_reporter_id, er_case_id, stub_cite, cite_cleaned, cite_normalized, cite_linked)
	ON CONFLICT (citation_id) DO NOTHING`
}

// StreamUnprocessedCitations runs a single anti-join over the whole
// citations_unlinked table, streaming every citation not yet in citation_links
// to fn in batches of at most batchSize.
//
// This replaces the old cursor-paginated fetch. That approach ran one
// LIMIT-bounded anti-join per batch; because the inner citation_links index
// scan had no lower bound, each of the ~12,500 batches re-scanned an
// ever-growing prefix of the 62M-row citation_links index to fast-forward to
// the cursor. The total work was quadratic in the table size and dominated the
// 13-hour runtime. One streaming pass scans each index once instead.
//
// The query holds a single connection (and a consistent snapshot) open for the
// duration of the stream, so the set delivered is exactly the citations that
// were unprocessed when the query began — concurrent inserts by the worker
// connections are invisible to it. Callers MUST apply backpressure inside fn;
// the whole table is read as fast as fn accepts batches.
func (s *CorpusStore) StreamUnprocessedCitations(ctx context.Context, batchSize int, fn func([]UnlinkedCitation) error) error {
	rows, err := s.DB.Query(ctx, s.stream)
	if err != nil {
		return fmt.Errorf("streaming unprocessed citations: %w", err)
	}
	defer rows.Close()

	batch := make([]UnlinkedCitation, 0, batchSize)
	for rows.Next() {
		var c UnlinkedCitation
		if err := rows.Scan(&c.ID, &c.MomlTreatise, &c.MomlPage, &c.Raw, &c.Volume, &c.ReporterAbbr, &c.Page, &c.Year); err != nil {
			return fmt.Errorf("scanning unlinked citation: %w", err)
		}
		batch = append(batch, c)
		if len(batch) >= batchSize {
			if err := fn(batch); err != nil {
				return err
			}
			batch = make([]UnlinkedCitation, 0, batchSize)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating unlinked citations: %w", err)
	}
	if len(batch) > 0 {
		if err := fn(batch); err != nil {
			return err
		}
	}
	return nil
}

// SaveLinkResults batch-inserts multiple link results in a single statement.
//
// Rather than build a VALUES list with up to batchSize*10 placeholders (which
// runs into Postgres's 65535-parameter limit at large batch sizes and forces
// the server to parse a huge statement on every batch), it passes one array per
// column and expands them server-side with unnest(). That is a fixed
// 10-parameter statement regardless of batch size, so it parses/plans cheaply
// and keeps the wire payload compact. citation_id is sent as text[] and cast to
// uuid in SQL to avoid relying on driver-side uuid-array encoding.
//
// An empty MatchTier is sent as SQL NULL: the skip statuses reach no tier, and a
// NULL keeps them out of every tier aggregate instead of inventing a bucket for
// them.
func (s *CorpusStore) SaveLinkResults(ctx context.Context, results []*LinkResult) error {
	if len(results) == 0 {
		return nil
	}

	ids := make([]string, len(results))
	statuses := make([]string, len(results))
	tiers := make([]*string, len(results))
	capIDs := make([]*int64, len(results))
	codeIDs := make([]*int64, len(results))
	erIDs := make([]*string, len(results))
	stubs := make([]*string, len(results))
	cleaned := make([]*string, len(results))
	normalized := make([]*string, len(results))
	linked := make([]*string, len(results))
	for i, r := range results {
		ids[i] = r.CitationID.String()
		statuses[i] = r.Status
		if r.MatchTier != "" {
			tiers[i] = &r.MatchTier
		}
		capIDs[i] = r.CAPCaseID
		codeIDs[i] = r.CodeReporterID
		erIDs[i] = r.ERCaseID
		stubs[i] = r.StubCite
		cleaned[i] = r.CiteCleaned
		normalized[i] = r.CiteNormalized
		linked[i] = r.CiteLinked
	}

	_, err := s.DB.Exec(ctx, s.insert, ids, statuses, tiers, capIDs, codeIDs, erIDs, stubs, cleaned, normalized, linked)
	if err != nil {
		return fmt.Errorf("batch saving %d link results: %w", len(results), err)
	}
	return nil
}
