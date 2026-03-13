package main

import (
	"context"
	"fmt"
	"strings"

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
	ProductLink     *string
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

// MomlVolumeURL returns a Gale MOML URL for the volume as a whole.
func (c *CitationDetail) MomlVolumeURL() string {
	if c.ProductLink == nil {
		return ""
	}
	url := *c.ProductLink
	url = strings.Replace(url, "http://link.galegroup.com", "https://link.gale.com", 1)
	url = strings.Replace(url, "?sid=dhxml", "?u=viva_gmu&sid=dhxml", 1)
	return url
}

// MomlPageURL constructs a Gale MOML URL linking to the specific page.
// The pg parameter uses MomlPage (pageid), with the trailing 0 stripped
// and leading 0s removed. E.g., pageid "06870" becomes pg=687.
func (c *CitationDetail) MomlPageURL() string {
	base := c.MomlVolumeURL()
	if base == "" {
		return ""
	}
	if c.MomlPage != "" {
		pg := c.MomlPage
		pg = strings.TrimRight(pg, "0")
		pg = strings.TrimLeft(pg, "0")
		if pg != "" {
			base += "&pg=" + pg
		}
	}
	return base
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
    bi.productlink,
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
LEFT JOIN moml.page mp ON mp.psmid = cu.moml_treatise AND mp.pageid = cu.moml_page
LEFT JOIN moml.page_ocrtext po ON po.psmid = cu.moml_treatise AND po.pageid = cu.moml_page
WHERE cu.id = $1
LIMIT 1
`

// ReporterStandard is a distinct reporter_standard value with its count.
type ReporterStandard struct {
	Name  string
	Count int
}

func getReporterStandards(ctx context.Context, db *pgxpool.Pool) ([]ReporterStandard, error) {
	query := `
	SELECT reporter_standard, count(*) AS n
	FROM legalhist.reporters_citation_to_cap
	WHERE reporter_standard IS NOT NULL
	GROUP BY reporter_standard
	ORDER BY reporter_standard
	`
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying reporter standards: %w", err)
	}
	defer rows.Close()

	var results []ReporterStandard
	for rows.Next() {
		var r ReporterStandard
		if err := rows.Scan(&r.Name, &r.Count); err != nil {
			return nil, fmt.Errorf("scanning reporter standard: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ReporterCite is a raw citation with its linking status, for the reporter check page.
type ReporterCite struct {
	ID     uuid.UUID
	Raw    string
	Status *string
}

// StatusClass returns a CSS class based on the linking status.
func (r *ReporterCite) StatusClass() string {
	if r.Status == nil {
		return "status-unprocessed"
	}
	s := *r.Status
	if strings.HasPrefix(s, "linked") {
		return "status-linked"
	}
	if s == "skipped_no_match" {
		return "status-nomatch"
	}
	if s == "skipped_junk" || s == "skipped_statute" {
		return "status-skip"
	}
	return "status-unprocessed"
}

func getReporterVariants(ctx context.Context, db *pgxpool.Pool, reporterStandard string) ([]string, error) {
	query := `
	SELECT reporter_found
	FROM legalhist.reporters_citation_to_cap
	WHERE reporter_standard = $1
	ORDER BY reporter_found
	`
	rows, err := db.Query(ctx, query, reporterStandard)
	if err != nil {
		return nil, fmt.Errorf("querying variants for reporter %q: %w", reporterStandard, err)
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scanning variant: %w", err)
		}
		results = append(results, v)
	}
	return results, rows.Err()
}

func getCitesForReporter(ctx context.Context, db *pgxpool.Pool, reporterStandard string) ([]ReporterCite, error) {
	query := `
	SELECT cu.id, cu.raw, cl.status
	FROM moml_citations.citations_unlinked cu
	JOIN legalhist.reporters_citation_to_cap wl ON cu.reporter_abbr = wl.reporter_found
	LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
	WHERE wl.reporter_standard = $1
	ORDER BY cu.id
	LIMIT 10000
	`
	rows, err := db.Query(ctx, query, reporterStandard)
	if err != nil {
		return nil, fmt.Errorf("querying cites for reporter %q: %w", reporterStandard, err)
	}
	defer rows.Close()

	var results []ReporterCite
	for rows.Next() {
		var c ReporterCite
		if err := rows.Scan(&c.ID, &c.Raw, &c.Status); err != nil {
			return nil, fmt.Errorf("scanning cite for reporter: %w", err)
		}
		// Strip newlines from raw cite
		c.Raw = strings.ReplaceAll(c.Raw, "\n", " ")
		c.Raw = strings.ReplaceAll(c.Raw, "\r", " ")
		results = append(results, c)
	}
	return results, rows.Err()
}

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
		&c.ProductLink,
		&c.SourcePage,
		&c.OCRText,
	)
	if err != nil {
		return nil, fmt.Errorf("querying citation detail for %s: %w", id, err)
	}
	return &c, nil
}
