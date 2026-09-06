package sources

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// PgxStore is a datastore for sources contained in a PostgreSQL database using
// the pgx driver.
type PgxStore struct {
	DB *pgxpool.Pool
}

// NewPgxStore creates a new datastore backed by the database
func NewPgxStore(db *pgxpool.Pool) *PgxStore {
	return &PgxStore{
		DB: db,
	}
}

// GetDocFromPath is not implemented for this datastore. It will always return an error.
func (p *PgxStore) GetDocFromPath(context.Context, string, string) (*Doc, error) {
	return nil, ErrNotImplemented
}

// GetTreatisePage gets a TreatisePage from the ID of the treatise and the page
func (p *PgxStore) GetTreatisePage(ctx context.Context, treatiseID string, pageID string) (*TreatisePage, error) {
	if treatiseID == "" || pageID == "" {
		return nil, ErrInvalidID
	}

	query := `
	SELECT psmid, pageid, ocrtext FROM moml.page_ocrtext
	WHERE psmid = $1 AND pageid = $2;`

	var dbID, dbTreatiseID, dbText string

	err := p.DB.QueryRow(ctx, query, treatiseID, pageID).Scan(&dbTreatiseID, &dbID, &dbText)
	if err == pgx.ErrNoRows {
		return nil, ErrNoDocument
	}
	if err != nil {
		return nil, fmt.Errorf("problem getting treatise page: %w", err)
	}

	page := NewTreatisePage(dbID, dbTreatiseID, dbText)

	return page, nil
}

// GetAllTreatisePageIDs gets all the IDs (both document and page) for the treatises.
// However, the full text will be empty.
func (p *PgxStore) GetAllTreatisePageIDs(ctx context.Context) ([]*TreatisePage, error) {
	query := `SELECT psmid, pageid FROM moml.page_ocrtext;`
	var pages []*TreatisePage

	rows, err := p.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docID, pageID string
	for rows.Next() {
		err = rows.Scan(&docID, &pageID)
		if err != nil {
			return nil, err
		}
		page := NewTreatisePage(pageID, docID, "")
		pages = append(pages, page)
	}
	// Without this a mid-stream failure returns a short slice and a nil error,
	// so the detector would silently scan part of the corpus and report success
	// (issue #285).
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating treatise page IDs: %w", err)
	}

	return pages, nil
}

// StreamTreatisePages reads every treatise page, text included, in a single
// pass and hands each one to fn.
//
// This replaces fetching all 10.5M page IDs into a slice and then issuing one
// GetTreatisePage query per page: two round trips per page, and about 1.3 GB of
// resident memory for the IDs alone. One streaming read costs neither.
//
// The query holds a single connection and a consistent snapshot open for the
// duration of the stream, so pages added while it runs are not seen. Callers
// MUST apply backpressure inside fn -- the corpus is read as fast as fn accepts
// pages, and it does not fit in memory.
func (p *PgxStore) StreamTreatisePages(ctx context.Context, fn func(*TreatisePage) error) error {
	query := `SELECT psmid, pageid, ocrtext FROM moml.page_ocrtext;`

	rows, err := p.DB.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("streaming treatise pages: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var docID, pageID, ocrText string
		if err := rows.Scan(&docID, &pageID, &ocrText); err != nil {
			return fmt.Errorf("scanning treatise page: %w", err)
		}
		if err := fn(NewTreatisePage(pageID, docID, ocrText)); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating treatise pages: %w", err)
	}
	return nil
}

// CountTreatisePages returns how many pages StreamTreatisePages will deliver.
// It exists only so that --progress can show a total; the detector does not need
// it otherwise, and it costs a full scan of moml.page_ocrtext.
func (p *PgxStore) CountTreatisePages(ctx context.Context) (int64, error) {
	var n int64
	err := p.DB.QueryRow(ctx, `SELECT count(*) FROM moml.page_ocrtext;`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting treatise pages: %w", err)
	}
	return n, nil
}

// GetOCRSubstitutions gets a complete list of OCR substitutions from the
// database, longest mistake first.
//
// The order is load-bearing rather than cosmetic. Several corrections begin with
// another ("Cusl" and "Cuslr", "Wvis" and "Wvisc", "Johns. Cl" and "Johns. Cll"),
// and NewOCRReplacer resolves a position in favour of whichever rule it is given
// first. Sorting here as well as there keeps the slice itself meaningful to any
// other caller, and makes two runs over the same table identical (issue #285).
func (p *PgxStore) GetOCRSubstitutions(ctx context.Context) ([]*OCRSubstitution, error) {
	query := `
	SELECT mistake, correction FROM legalhist.ocr_corrections
	ORDER BY length(mistake) DESC, mistake COLLATE "C";`
	var subs []*OCRSubstitution

	rows, err := p.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		sub := OCRSubstitution{}
		err = rows.Scan(&sub.Mistake, &sub.Correction)
		if err != nil {
			return nil, err
		}
		subs = append(subs, &sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating OCR substitutions: %w", err)
	}

	return subs, nil
}

// GetBatchOfUnprocessedPages get a set number of OCR pages from the database.
// Note that this is limited to the body pages of U.S. treatises.
func (p *PgxStore) GetBatchOfUnprocessedPages(ctx context.Context, batchSize int) ([]*TreatisePage, error) {

	if batchSize < 0 {
		return nil, ErrBatchSize
	}

	query := `
	SELECT po.psmid, po.pageid, po.ocrtext
	FROM moml.page_ocrtext po
	JOIN moml.book_info bi ON po.psmid = bi.psmid
	WHERE NOT EXISTS (
    SELECT 1 
    FROM moml.book_subject bs 
    WHERE bs.psmid = po.psmid 
    AND bs.subject IN ('UK', 'Biography', 'Collected Essays', 'Trials')
	)
	AND NOT EXISTS (
    SELECT 1 
    FROM predictor.requests r 
    WHERE r.psmid = po.psmid 
    AND r.pageid = po.pageid
	)
	LIMIT $1;
	`

	pages := make([]*TreatisePage, 0, batchSize)

	rows, err := p.DB.Query(ctx, query, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docID, pageID, ocrText string
	for rows.Next() {
		err = rows.Scan(&docID, &pageID, &ocrText)
		if err != nil {
			return nil, err
		}
		page := NewTreatisePage(pageID, docID, ocrText)
		pages = append(pages, page)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unprocessed pages: %w", err)
	}

	return pages, nil
}
