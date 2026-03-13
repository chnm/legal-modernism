package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
)

// CitationDetail holds all the information needed to display a citation.
type CitationDetail struct {
	// Detected citation
	ID           uuid.UUID
	Raw          string
	Volume       *int
	ReporterAbbr string
	Page         int
	MomlTreatise string
	MomlPage     string

	// Linking result
	Status         *string
	CiteCleaned    *string
	CiteNormalized *string
	CiteLinked     *string

	// CAP case info
	CAPCaseName   *string
	CAPCaseAbbr   *string
	CAPYear       *int
	CAPVolume     *string
	CAPURL        *string
	CAPReporter   *string
	CAPCourt      *string
	CAPJurisdiction *string

	// Code reporter case info
	CodeName *string
	CodeCite *string
	CodeYear *int

	// English reports case info
	ERName  *string
	ERCite  *string
	ERYear  *int
	ERCourt *string

	// MOML source info
	BibliographicID *string
	PubYear         *int
	BookTitle       *string
	Author          *string
	SourcePage      *string
	OCRText         *string
}

// HasLink returns true if the citation was linked to any case.
func (c *CitationDetail) HasLink() bool {
	return c.Status != nil && (*c.Status == "linked_cap" || *c.Status == "linked_code_reporter" || *c.Status == "linked_english_reports")
}

// IsCAP returns true if linked to a CAP case.
func (c *CitationDetail) IsCAP() bool {
	return c.Status != nil && *c.Status == "linked_cap"
}

// IsCodeReporter returns true if linked to a code reporter case.
func (c *CitationDetail) IsCodeReporter() bool {
	return c.Status != nil && *c.Status == "linked_code_reporter"
}

// IsEnglishReports returns true if linked to an English Reports case.
func (c *CitationDetail) IsEnglishReports() bool {
	return c.Status != nil && *c.Status == "linked_english_reports"
}

const citationDetailQuery = `
SELECT
    cu.id,
    cu.raw,
    cu.volume,
    cu.reporter_abbr,
    cu.page,
    cu.moml_treatise,
    cu.moml_page,
    cl.status,
    cl.cite_cleaned,
    cl.cite_normalized,
    cl.cite_linked,
    cc.name,
    cc.name_abbreviation,
    cc.decision_year,
    cc.volume,
    cc.frontend_url,
    cr_rep.full_name,
    ct.name,
    j.name,
    code.name,
    code.official_citation,
    code.decision_year,
    er.er_name,
    er.er_cite,
    er.er_year,
    er.court,
    bi.bibliographicid,
    bi.year,
    bc.displaytitle,
    bc.author_composed,
    mp.sourcepage,
    po.ocrtext
FROM moml_citations.citations_unlinked cu
LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
LEFT JOIN cap.cases cc ON cc.id = cl.cap_case_id
LEFT JOIN cap.reporters cr_rep ON cr_rep.id = cc.reporter
LEFT JOIN cap.courts ct ON ct.id = cc.court
LEFT JOIN cap.jurisdictions j ON j.id = cc.jurisdiction
LEFT JOIN legalhist.code_reporter code ON code.id = cl.code_reporter_id
LEFT JOIN english_reports.cases er ON er.id = cl.er_case_id
LEFT JOIN moml.book_info bi ON bi.psmid = cu.moml_treatise
LEFT JOIN moml.book_citation bc ON bc.psmid = cu.moml_treatise
LEFT JOIN moml.page mp ON mp.pageid = cu.moml_page
LEFT JOIN moml.page_ocrtext po ON po.psmid = cu.moml_treatise AND po.pageid = cu.moml_page
WHERE cu.id = $1
LIMIT 1
`

func getCitationDetail(ctx context.Context, db *pgxpool.Pool, id uuid.UUID) (*CitationDetail, error) {
	var c CitationDetail
	err := db.QueryRow(ctx, citationDetailQuery, id).Scan(
		&c.ID,
		&c.Raw,
		&c.Volume,
		&c.ReporterAbbr,
		&c.Page,
		&c.MomlTreatise,
		&c.MomlPage,
		&c.Status,
		&c.CiteCleaned,
		&c.CiteNormalized,
		&c.CiteLinked,
		&c.CAPCaseName,
		&c.CAPCaseAbbr,
		&c.CAPYear,
		&c.CAPVolume,
		&c.CAPURL,
		&c.CAPReporter,
		&c.CAPCourt,
		&c.CAPJurisdiction,
		&c.CodeName,
		&c.CodeCite,
		&c.CodeYear,
		&c.ERName,
		&c.ERCite,
		&c.ERYear,
		&c.ERCourt,
		&c.BibliographicID,
		&c.PubYear,
		&c.BookTitle,
		&c.Author,
		&c.SourcePage,
		&c.OCRText,
	)
	if err != nil {
		return nil, fmt.Errorf("querying citation detail for %s: %w", id, err)
	}
	return &c, nil
}
