package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/agnivade/levenshtein"
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
	slog.Debug("querying reporter standards")
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
	slog.Debug("fetched reporter standards", "count", len(results))
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
	if s == "no_match" {
		return "status-nomatch"
	}
	if s == "skipped_junk" || s == "skipped_statute" {
		return "status-skip"
	}
	return "status-unprocessed"
}

func getReporterVariants(ctx context.Context, db *pgxpool.Pool, reporterStandard string) ([]string, error) {
	slog.Debug("querying reporter variants", "reporter", reporterStandard)
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
	slog.Debug("fetched reporter variants", "reporter", reporterStandard, "count", len(results))
	return results, rows.Err()
}

func getCitesForReporter(ctx context.Context, db *pgxpool.Pool, reporterStandard string) ([]ReporterCite, error) {
	slog.Debug("querying cites for reporter", "reporter", reporterStandard)
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
	slog.Debug("fetched cites for reporter", "reporter", reporterStandard, "count", len(results))
	return results, rows.Err()
}

// UnwhitelistedReporter is a reporter abbreviation not found in the whitelist,
// with a count of how many citations reference it and potential matches.
type UnwhitelistedReporter struct {
	ReporterAbbr string              `json:"reporterAbbr"`
	Count        int                 `json:"count"`
	Matches      []ReporterMatch     `json:"matches"`
}

// ReporterMatch is a potential reporter_standard match with its CAP info.
type ReporterMatch struct {
	Standard     string `json:"standard"`
	ReporterCap  string `json:"reporterCap"`
	CapDifferent bool   `json:"capDifferent"`
	Score        int    `json:"score"`
}

// capInfo holds the reporter_cap and cap_different for a reporter_standard.
type capInfo struct {
	ReporterCap  string
	CapDifferent bool
}

func getUnwhitelistedReporters(ctx context.Context, db *pgxpool.Pool) ([]UnwhitelistedReporter, error) {
	slog.Debug("querying unwhitelisted reporters")
	query := `
	SELECT cu.reporter_abbr, count(*) AS n
	FROM moml_citations.citations_unlinked cu
	LEFT JOIN legalhist.reporters_citation_to_cap wl
	  ON cu.reporter_abbr = wl.reporter_found
	WHERE wl.reporter_found IS NULL
	GROUP BY cu.reporter_abbr
	ORDER BY n DESC
	LIMIT 250
	`
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying unwhitelisted reporters: %w", err)
	}
	defer rows.Close()

	var results []UnwhitelistedReporter
	for rows.Next() {
		var r UnwhitelistedReporter
		if err := rows.Scan(&r.ReporterAbbr, &r.Count); err != nil {
			return nil, fmt.Errorf("scanning unwhitelisted reporter: %w", err)
		}
		results = append(results, r)
	}
	slog.Debug("fetched unwhitelisted reporters", "count", len(results))
	return results, rows.Err()
}

// getDistinctReporterStandards returns all distinct reporter_standard values.
func getDistinctReporterStandards(ctx context.Context, db *pgxpool.Pool) ([]string, error) {
	slog.Debug("querying distinct reporter standards")
	query := `
	SELECT DISTINCT reporter_standard
	FROM legalhist.reporters_citation_to_cap
	WHERE reporter_standard IS NOT NULL
	ORDER BY reporter_standard
	`
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying distinct reporter standards: %w", err)
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scanning reporter standard: %w", err)
		}
		results = append(results, s)
	}
	slog.Debug("fetched distinct reporter standards", "count", len(results))
	return results, rows.Err()
}

// getCapInfoMap returns a map of reporter_standard → capInfo for standards
// that have a reporter_cap value.
func getCapInfoMap(ctx context.Context, db *pgxpool.Pool) (map[string]capInfo, error) {
	slog.Debug("querying cap info map")
	query := `
	SELECT DISTINCT ON (reporter_standard) reporter_standard, reporter_cap, COALESCE(cap_different, false)
	FROM legalhist.reporters_citation_to_cap
	WHERE reporter_standard IS NOT NULL AND reporter_cap IS NOT NULL AND reporter_cap != ''
	ORDER BY reporter_standard
	`
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying cap info: %w", err)
	}
	defer rows.Close()

	m := make(map[string]capInfo)
	for rows.Next() {
		var std, cap string
		var diff bool
		if err := rows.Scan(&std, &cap, &diff); err != nil {
			return nil, fmt.Errorf("scanning cap info: %w", err)
		}
		m[std] = capInfo{ReporterCap: cap, CapDifferent: diff}
	}
	slog.Debug("fetched cap info map", "count", len(m))
	return m, rows.Err()
}

// normalizeReporter strips periods, commas, and extra whitespace, then lowercases.
func normalizeReporter(s string) string {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// computeMatches finds the best reporter_standard matches for an abbreviation
// using Levenshtein distance on normalized forms.
func computeMatches(abbr string, standards []string, capMap map[string]capInfo) []ReporterMatch {
	normAbbr := normalizeReporter(abbr)
	if normAbbr == "" {
		return nil
	}

	type scored struct {
		standard string
		score    int
	}

	var candidates []scored
	for _, std := range standards {
		normStd := normalizeReporter(std)
		if normStd == "" {
			continue
		}

		var score int

		// Exact normalized match
		if normAbbr == normStd {
			score = 100
		} else if strings.HasPrefix(normAbbr, normStd) || strings.HasPrefix(normStd, normAbbr) {
			// Prefix match
			score = 90
		} else {
			// Levenshtein distance
			dist := levenshtein.ComputeDistance(normAbbr, normStd)
			maxLen := max(len(normAbbr), len(normStd))
			score = int((1.0 - float64(dist)/float64(maxLen)) * 100)
		}

		if score >= 30 {
			candidates = append(candidates, scored{standard: std, score: score})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].standard < candidates[j].standard
	})

	// Cap at 20 matches
	if len(candidates) > 20 {
		candidates = candidates[:20]
	}

	matches := make([]ReporterMatch, len(candidates))
	for i, c := range candidates {
		m := ReporterMatch{
			Standard: c.standard,
			Score:    c.score,
		}
		if info, ok := capMap[c.standard]; ok {
			m.ReporterCap = info.ReporterCap
			m.CapDifferent = info.CapDifferent
		}
		matches[i] = m
	}
	return matches
}

// ReporterStats holds linked and no-match counts for a single reporter_standard.
type ReporterStats struct {
	Reporter    string `json:"reporter"`
	Linked      int    `json:"linked"`
	NoMatch     int    `json:"noMatch"`
	Unprocessed int    `json:"unprocessed"`
	UK          bool   `json:"uk"`
}

// DashboardData holds aggregated linking status data for the dashboard.
type DashboardData struct {
	LinkedCAP             int
	LinkedEnglishReports  int
	LinkedCodeReporter    int
	SkippedNotWhiteListed int
	NoMatch               int
	SkippedJunk           int
	SkippedStatute        int
	TotalRawCites         int
	Reporters             []ReporterStats `json:"Reporters,omitempty"`
}

// TotalLinked returns the sum of all linked statuses.
func (d *DashboardData) TotalLinked() int {
	return d.LinkedCAP + d.LinkedEnglishReports + d.LinkedCodeReporter
}

func getDashboardData(ctx context.Context, db *pgxpool.Pool) (*DashboardData, error) {
	d := &DashboardData{}

	// Get counts by status from the view
	slog.Debug("querying citation links status view")
	rows, err := db.Query(ctx, `SELECT status, n FROM moml_citations.citation_links_status`)
	if err != nil {
		return nil, fmt.Errorf("querying citation links status: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("scanning citation links status: %w", err)
		}
		slog.Debug("citation links status row", "status", status, "n", n)
		switch status {
		case "linked_cap":
			d.LinkedCAP = n
		case "linked_english_reports":
			d.LinkedEnglishReports = n
		case "linked_code_reporter":
			d.LinkedCodeReporter = n
		case "skipped_not_whitelisted":
			d.SkippedNotWhiteListed = n
		case "no_match":
			d.NoMatch = n
		case "skipped_junk":
			d.SkippedJunk = n
		case "skipped_statute":
			d.SkippedStatute = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating citation links status: %w", err)
	}
	slog.Debug("finished querying citation links status view")

	// Get total raw citations count
	slog.Debug("counting total raw citations")
	err = db.QueryRow(ctx, `SELECT count(*) FROM moml_citations.citations_unlinked`).Scan(&d.TotalRawCites)
	if err != nil {
		return nil, fmt.Errorf("counting raw citations: %w", err)
	}
	// Get per-reporter linking stats
	slog.Debug("querying per-reporter linking stats")
	reporterRows, err := db.Query(ctx, `
		SELECT
			wl.reporter_standard,
			count(*) FILTER (WHERE cl.status LIKE 'linked%') AS linked,
			count(*) FILTER (WHERE cl.status = 'no_match') AS no_match,
			count(*) FILTER (WHERE cl.status IS NULL) AS unprocessed,
			bool_or(wl.uk) AS uk
		FROM moml_citations.citations_unlinked cu
		JOIN legalhist.reporters_citation_to_cap wl ON cu.reporter_abbr = wl.reporter_found
		LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
		WHERE wl.reporter_standard IS NOT NULL
		  AND wl.statute = false
		  AND wl.junk = false
		GROUP BY wl.reporter_standard
		ORDER BY count(*) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying reporter stats: %w", err)
	}
	defer reporterRows.Close()

	for reporterRows.Next() {
		var r ReporterStats
		if err := reporterRows.Scan(&r.Reporter, &r.Linked, &r.NoMatch, &r.Unprocessed, &r.UK); err != nil {
			return nil, fmt.Errorf("scanning reporter stats: %w", err)
		}
		d.Reporters = append(d.Reporters, r)
	}
	if err := reporterRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating reporter stats: %w", err)
	}
	slog.Debug("fetched reporter stats", "count", len(d.Reporters))

	slog.Debug("dashboard data complete",
		"linked_cap", d.LinkedCAP,
		"linked_english_reports", d.LinkedEnglishReports,
		"linked_code_reporter", d.LinkedCodeReporter,
		"skipped_not_whitelisted", d.SkippedNotWhiteListed,
		"no_match", d.NoMatch,
		"skipped_junk", d.SkippedJunk,
		"skipped_statutes", d.SkippedStatute,
		"total_raw_cites", d.TotalRawCites,
		"reporters", len(d.Reporters),
	)

	return d, nil
}

func getCitationDetail(ctx context.Context, db *pgxpool.Pool, id uuid.UUID) (*CitationDetail, error) {
	slog.Debug("querying citation detail", "id", id)
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
	slog.Debug("fetched citation detail", "id", id, "status", c.Status, "reporter_abbr", c.ReporterAbbr)
	return &c, nil
}
