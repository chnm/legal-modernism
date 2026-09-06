package citations

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

// DBStore is a database store for citation objects
type DBStore struct {
	DB *pgxpool.Pool
}

// NewDBStore returns an citation repo using PostgreSQL with the pgx native interface.
func NewDBStore(db *pgxpool.Pool) *DBStore {
	return &DBStore{
		DB: db,
	}
}

// SaveCitation saves a single citation to the database.
func (r *DBStore) SaveCitation(ctx context.Context, c *Citation) error {
	return r.SaveCitations(ctx, []*Citation{c})
}

// SaveCitations inserts a page's worth of citations in one statement.
//
// Rather than build a VALUES list, which would grow the statement with the
// batch and run into Postgres's 65535-parameter limit, it passes one array per
// column and expands them server-side with unnest(), the same shape
// SaveLinkResults uses. That is a fixed 8-parameter statement whatever the
// batch size. The id is sent as text[] and cast to uuid in SQL rather than
// relying on driver-side uuid-array encoding, and created_at is a single scalar
// because every citation in a batch comes off the same page at the same moment.
//
// Duplicates are collapsed twice over. citationKey drops them inside the batch,
// on exactly the columns of the citations_unlinked_uq unique index, and the
// bare ON CONFLICT DO NOTHING -- no conflict target, so it covers the primary
// key and that index alike -- drops any that collide with rows already in the
// table. Both are needed: detectors legitimately produce identical spans for one
// citation (two abbreviations that are prefixes of one another), and a re-run
// over a page already scanned must not fail.
func (r *DBStore) SaveCitations(ctx context.Context, cites []*Citation) error {
	if len(cites) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(cites))
	ids := make([]string, 0, len(cites))
	treatises := make([]string, 0, len(cites))
	pages := make([]string, 0, len(cites))
	raws := make([]string, 0, len(cites))
	volumes := make([]*int32, 0, len(cites))
	abbrs := make([]string, 0, len(cites))
	pageNums := make([]int32, 0, len(cites))

	for _, c := range cites {
		k := citationKey(c)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}

		ids = append(ids, c.ID.String())
		treatises = append(treatises, c.Source.ParentID())
		pages = append(pages, c.Source.ID())
		raws = append(raws, c.Raw)
		if c.Volume == nil {
			volumes = append(volumes, nil)
		} else {
			v := int32(*c.Volume)
			volumes = append(volumes, &v)
		}
		abbrs = append(abbrs, c.ReporterAbbr)
		pageNums = append(pageNums, int32(c.Page))
	}

	query := `
	INSERT INTO moml_citations.citations_unlinked
		(id, moml_treatise, moml_page, raw, volume, reporter_abbr, page, created_at)
	SELECT u.id::uuid, u.moml_treatise, u.moml_page, u.raw, u.volume, u.reporter_abbr, u.page, $8
	FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::int4[], $6::text[], $7::int4[])
		AS u(id, moml_treatise, moml_page, raw, volume, reporter_abbr, page)
	ON CONFLICT DO NOTHING;
	`
	_, err := r.DB.Exec(ctx, query, ids, treatises, pages, raws, volumes, abbrs, pageNums, time.Now())
	if err != nil {
		return fmt.Errorf("batch saving %d citations: %w", len(ids), err)
	}
	return nil
}

// citationKey is the key of the citations_unlinked_uq unique index --
// (moml_treatise, moml_page, COALESCE(volume, -1), reporter_abbr, page) -- so
// that dropping duplicates in Go removes exactly the rows the database would
// have refused. The NUL separator cannot occur in any part, so no two distinct
// citations can share a key.
func citationKey(c *Citation) string {
	vol := "-1"
	if c.Volume != nil {
		vol = strconv.Itoa(*c.Volume)
	}
	return strings.Join([]string{
		c.Source.ParentID(), c.Source.ID(), vol, c.ReporterAbbr, strconv.Itoa(c.Page),
	}, "\x00")
}

// GetSingleVolReporterAbbrs returns one row per (reporter_standard, abbreviation)
// pair for every single-volume reporter, covering both the canonical
// reporter_standard form and every alt_abbr in legalhist.reporters_abbreviations.
// Pairing each abbreviation with its canonical reporter_standard lets the
// detector normalize the saved reporter_abbr to the canonical form regardless
// of which spelling appeared in the OCR.
func (r *DBStore) GetSingleVolReporterAbbrs(ctx context.Context) ([]SingleVolReporter, error) {
	query := `
	SELECT r.reporter_standard, r.reporter_standard AS abbr
	  FROM legalhist.reporters r
	 WHERE r.single_vol = true
	UNION
	SELECT r.reporter_standard, ra.alt_abbr
	  FROM legalhist.reporters r
	  JOIN legalhist.reporters_abbreviations ra
	    ON ra.reporter_standard = r.reporter_standard
	 WHERE r.single_vol = true
	   AND ra.alt_abbr IS NOT NULL;
	`
	var reporters []SingleVolReporter

	rows, err := r.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sv SingleVolReporter
		if err := rows.Scan(&sv.Standard, &sv.Abbr); err != nil {
			return nil, err
		}
		reporters = append(reporters, sv)
	}
	// This slice gates every single-volume detector, so a mid-stream failure
	// that returned a short list and a nil error would quietly scan the corpus
	// with only some of them (issue #285).
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating single volume reporters: %w", err)
	}

	return reporters, nil
}
