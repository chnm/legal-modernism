package citations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// ErrNoCorpus is returned by a CorpusStore built as a bare literal rather than
// by NewMOMLCorpusStore or NewOpinionCorpusStore: such a store knows no tables,
// and without this it would stream nothing and report success.
var ErrNoCorpus = errors.New("citations: corpus store built without a corpus; use NewMOMLCorpusStore or NewOpinionCorpusStore")

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
	// scan reads one streamed row into a citation, including its SourceYear.
	// Each corpus keys the citing document differently (a psmid, a case id),
	// so the row's shape and where the year comes from are the store's to
	// know; the cascade sees only the year.
	scan func(rows pgx.Rows, c *UnlinkedCitation) error
	// prepare is what Prepare runs once before the stream: loading what scan
	// needs, or checking that the ledger's tables exist. May be nil.
	prepare  func(ctx context.Context) error
	prepared bool
}

// Prepare readies the store for a run: the MOML store loads the volume years
// its scan dates citations by, and logs how many. A driver calls it after the
// lookup tables load and before linking starts, so that whatever the ledger
// needs is loaded and logged with the other tables and a failure is a startup
// failure rather than a stream failure minutes in. StreamUnprocessedCitations
// calls it itself if no one has, so a store is never streamed unprepared. A
// store is for one run, on one goroutine at a time.
func (s *CorpusStore) Prepare(ctx context.Context) error {
	if s.stream == "" || s.scan == nil {
		return ErrNoCorpus
	}
	if s.prepared {
		return nil
	}
	if s.prepare != nil {
		if err := s.prepare(ctx); err != nil {
			return err
		}
	}
	s.prepared = true
	return nil
}

// NewMOMLCorpusStore returns the store over moml_citations: the citations
// cite-detector-moml finds on the pages of the treatises, and cite-linker's
// results.
//
// The citing document's year comes from moml.volumes.year by psmid, the
// volume the page belongs to, looked up at scan time in a map the store loads
// before it streams (about 25K volumes). Looking it up here rather than joining
// moml.volumes into the stream keeps the anti-join's SQL, and so its plan,
// exactly what it was; a volume with no year, or none in the table, leaves
// SourceYear nil, which is what left the psmid out of the map before.
func NewMOMLCorpusStore(db *pgxpool.Pool) *CorpusStore {
	var years map[string]int
	s := &CorpusStore{
		DB: db,
		stream: `
		SELECT cu.id, cu.moml_treatise, cu.raw, cu.volume, cu.reporter_abbr, cu.page, cu.year
		FROM moml_citations.citations_unlinked cu
		WHERE NOT EXISTS (
			SELECT 1 FROM moml_citations.citation_links cl WHERE cl.citation_id = cu.id
		)
		`,
		insert: linksInsertSQL("moml_citations.citation_links"),
	}
	s.prepare = func(ctx context.Context) error {
		slog.Info("loading treatise years")
		var err error
		years, err = loadYears[string](ctx, db, "treatise years",
			`SELECT psmid, year FROM moml.volumes WHERE year IS NOT NULL`, 25_000)
		if err != nil {
			return err
		}
		// The count is the one number that shows the anachronism gate has its
		// input: with no years loaded no MOML link is ever refused.
		if len(years) == 0 {
			slog.Warn("no treatise years loaded; no link will be refused as anachronistic (issue #319)")
		}
		slog.Info("loaded treatise years", "treatises", len(years))
		return nil
	}
	s.scan = func(rows pgx.Rows, c *UnlinkedCitation) error {
		var psmid string
		if err := rows.Scan(&c.ID, &psmid, &c.Raw, &c.Volume, &c.ReporterAbbr, &c.Page, &c.Year); err != nil {
			return err
		}
		if y, ok := years[psmid]; ok {
			c.SourceYear = &y
		}
		return nil
	}
	return s
}

// NewOpinionCorpusStore returns the store over opinion_citations: the
// citations cite-detector-cap finds in the text of CAP opinions, and
// cite-linker-cap's results (issue #74). The citing document is the case the
// opinion belongs to, so its year is cap.cases.decision_year, joined into the
// stream; a citation whose case has no year, or no row, streams with a nil
// SourceYear and is never refused as anachronistic. A case is of the same
// year as itself, so an opinion's citation of its own case links.
func NewOpinionCorpusStore(db *pgxpool.Pool) *CorpusStore {
	return &CorpusStore{
		DB: db,
		stream: `
		SELECT cu.id, cu.raw, cu.volume, cu.reporter_abbr, cu.page, cu.year, k.decision_year
		FROM opinion_citations.citations_unlinked cu
		LEFT JOIN cap.cases k ON k.id = cu.cap_case
		WHERE NOT EXISTS (
			SELECT 1 FROM opinion_citations.citation_links cl WHERE cl.citation_id = cu.id
		)
		`,
		insert: linksInsertSQL("opinion_citations.citation_links"),
		scan: func(rows pgx.Rows, c *UnlinkedCitation) error {
			return rows.Scan(&c.ID, &c.Raw, &c.Volume, &c.ReporterAbbr, &c.Page, &c.Year, &c.SourceYear)
		},
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
	if err := s.Prepare(ctx); err != nil {
		return fmt.Errorf("preparing to stream unprocessed citations: %w", err)
	}
	rows, err := s.DB.Query(ctx, s.stream)
	if err != nil {
		return fmt.Errorf("streaming unprocessed citations: %w", err)
	}
	defer rows.Close()

	batch := make([]UnlinkedCitation, 0, batchSize)
	for rows.Next() {
		var c UnlinkedCitation
		if err := s.scan(rows, &c); err != nil {
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
	if s.insert == "" {
		return ErrNoCorpus
	}
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
