package citations

import (
	"context"
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

// SaveCitation save a citation to the database
func (r *DBStore) SaveCitation(ctx context.Context, c *Citation) error {
	query := `
	INSERT INTO
	moml_citations.citations_unlinked (id, moml_treatise, moml_page, raw, volume, reporter_abbr, page, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	ON CONFLICT DO NOTHING;
	`
	_, err := r.DB.Exec(ctx, query, c.ID, c.Source.ParentID(), c.Source.ID(),
		c.Raw, c.Volume, c.ReporterAbbr, c.Page, time.Now())
	return err
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

	return reporters, nil
}
