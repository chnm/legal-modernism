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

// StreamCAPOpinions reads every opinion of a CAP case decided in or before
// maxYear, text included, in a single pass and hands each one to fn. It is
// StreamTreatisePages for the CAP corpus (issue #74), with the same
// discipline: one connection and one snapshot held for the duration, so
// callers MUST apply backpressure inside fn.
//
// The year cutoff lives here and nowhere else: nothing in the tables the
// detections go to records it, and the citing case's year is a join away
// (cap.cases.decision_year). cap.opinions has no index, and needs none for
// this: the query is one pass over its heap, with a lookup on cases_pkey for
// each row's year, and the text is fetched from TOAST only for the rows that
// pass the cutoff.
func (p *PgxStore) StreamCAPOpinions(ctx context.Context, maxYear int, fn func(*CAPOpinion) error) error {
	query := `
	SELECT o.id, o."case", o.type, o.text
	FROM cap.opinions o
	JOIN cap.cases c ON c.id = o."case"
	WHERE c.decision_year <= $1;`

	rows, err := p.DB.Query(ctx, query, maxYear)
	if err != nil {
		return fmt.Errorf("streaming CAP opinions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var opinionID, caseID int64
		var typ, text string
		if err := rows.Scan(&opinionID, &caseID, &typ, &text); err != nil {
			return fmt.Errorf("scanning CAP opinion: %w", err)
		}
		if err := fn(NewCAPOpinion(opinionID, caseID, typ, text)); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating CAP opinions: %w", err)
	}
	return nil
}

// CountCAPOpinions returns how many opinions StreamCAPOpinions will deliver for
// maxYear, with the same join and filter so that the two agree. It exists only
// so that --progress can show a total, and costs a pass over cap.opinions.
func (p *PgxStore) CountCAPOpinions(ctx context.Context, maxYear int) (int64, error) {
	query := `
	SELECT count(*)
	FROM cap.opinions o
	JOIN cap.cases c ON c.id = o."case"
	WHERE c.decision_year <= $1;`

	var n int64
	if err := p.DB.QueryRow(ctx, query, maxYear).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting CAP opinions: %w", err)
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
