package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

func dbConnect() (*pgxpool.Pool, error) {
	connstr := os.Getenv("LAW_DBSTR")
	if connstr == "" {
		return nil, errors.New("database connection string is not set")
	}
	timeout, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	db, err := pgxpool.Connect(timeout, connstr)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (c Case) Save(ctx context.Context, db *pgxpool.Pool) error {
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	tx, err := db.Begin(timeout)
	if err != nil {
		return err
	}

	queryCase := `
	INSERT INTO cap.cases
		(id, name, name_abbreviation, decision_year, decision_date, decision_date_raw, docket_number,
		first_page_raw, last_page_raw, first_page, last_page, volume, reporter, court,
		jurisdiction, url, frontend_url, frontend_pdf_url, analysis, last_updated,
		provenance, imported, judges, parties, attorneys)
	VALUES
		($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
		$18, $19, $20, $21, $22, $23, $24, $25)
		ON CONFLICT DO NOTHING;
	`

	queryCourt := `
	INSERT INTO cap.courts
		(id, name, name_abbreviation, slug, url)
	VALUES
		($1, $2, $3, $4, $5)
	ON CONFLICT DO NOTHING;
	`

	queryJurisdiction := `
	INSERT INTO cap.jurisdictions
		(id, name, name_long, slug, whitelisted, url)
	VALUES
		($1, $2, $3, $4, $5, $6)
	ON CONFLICT DO NOTHING;
	`

	citationQuery := `
	INSERT INTO cap.citations (cite, type, "case")
	VALUES ($1, $2, $3);
	`

	opinionQuery := `
	INSERT INTO cap.opinions ("case", type, author, text)
	VALUES ($1, $2, $3, $4);
	`

	_, err = tx.Exec(timeout, queryCourt, c.Court.ID, c.Court.Name, c.Court.NameAbbreviation,
		c.Court.Slug, c.Court.URL)
	if err != nil {
		tx.Rollback(timeout)
		return fmt.Errorf("error saving court: %w", err)
	}

	_, err = tx.Exec(timeout, queryJurisdiction, c.Jurisdiction.ID, c.Jurisdiction.Name,
		c.Jurisdiction.NameLong, c.Jurisdiction.Slug, c.Jurisdiction.Whitelisted,
		c.Jurisdiction.URL)
	if err != nil {
		tx.Rollback(timeout)
		return fmt.Errorf("error saving jurisdiction: %w", err)
	}

	_, err = tx.Exec(timeout, queryCase, c.ID, c.Name, c.NameAbbreviation, c.Year(), c.DecisionDate(),
		c.DecisionDateRaw, c.DocketNumber, c.FirstPageRaw, c.LastPageRaw, c.FirstPage(),
		c.LastPage(), c.Volume(), c.Reporter.ID, c.Court.ID, c.Jurisdiction.ID,
		c.URL, c.FrontEndURL, c.FrontEndPDFURL, c.Analysis, c.LastUpdated, c.Provenance,
		c.Imported(), c.Casebody.Data.Judges, c.Casebody.Data.Parties, c.Casebody.Data.Attorneys)
	if err != nil {
		tx.Rollback(timeout)
		return fmt.Errorf("error saving case: %w", err)
	}

	for _, cite := range c.Citations {
		_, err := tx.Exec(timeout, citationQuery, cite.Cite, cite.Type, c.ID)
		if err != nil {
			tx.Rollback(timeout)
			return fmt.Errorf("error saving citation: %w", err)
		}
	}

	for _, opinion := range c.Casebody.Data.Opinions {
		_, err := tx.Exec(timeout, opinionQuery, c.ID, opinion.Type, opinion.Author, opinion.Text)
		if err != nil {
			tx.Rollback(timeout)
			return fmt.Errorf("error saving opinion: %w", err)
		}
	}

	err = tx.Commit(timeout)
	if err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}
