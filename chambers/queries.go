package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
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
	CAPCaseName     *string
	CAPCaseAbbr     *string
	CAPYear         *int
	CAPVolume       *string
	CAPURL          *string
	CAPReporter     *string
	CAPCourt        *string
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

// momlVolumeURL builds a Gale MOML volume URL from the stored productlink,
// rewriting the legacy link.galegroup.com host to the given host and inserting
// the optional Gale user query fragment (e.g. "u=viva_gmu&") before the sid
// parameter. host and userParam select which institution's proxy the link
// routes through.
func (c *CitationDetail) momlVolumeURL(host, userParam string) string {
	if c.ProductLink == nil {
		return ""
	}
	url := *c.ProductLink
	url = strings.Replace(url, "http://link.galegroup.com", host, 1)
	url = strings.Replace(url, "?sid=dhxml", "?"+userParam+"sid=dhxml", 1)
	return url
}

// MomlVolumeURL returns a Gale MOML URL for the volume as a whole, routed
// through GMU's library proxy.
func (c *CitationDetail) MomlVolumeURL() string {
	return c.momlVolumeURL("https://link.gale.com", "u=viva_gmu&")
}

// MomlVolumeURLColumbia returns a Gale MOML URL for the volume as a whole,
// routed through Columbia University's EZproxy.
func (c *CitationDetail) MomlVolumeURLColumbia() string {
	return c.momlVolumeURL("https://go-gale-com.ezproxy.cul.columbia.edu", "")
}

// momlPageURL appends the page (pg) parameter to a Gale MOML volume URL.
// The pg parameter uses MomlPage (pageid), with the trailing 0 stripped
// and leading 0s removed. E.g., pageid "06870" becomes pg=687.
func (c *CitationDetail) momlPageURL(base string) string {
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

// MomlPageURL constructs a Gale MOML URL linking to the specific page, routed
// through GMU's library proxy.
func (c *CitationDetail) MomlPageURL() string {
	return c.momlPageURL(c.MomlVolumeURL())
}

// MomlPageURLColumbia constructs a Gale MOML URL linking to the specific page,
// routed through Columbia University's EZproxy.
func (c *CitationDetail) MomlPageURLColumbia() string {
	return c.momlPageURL(c.MomlVolumeURLColumbia())
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
	SELECT r.reporter_standard, count(wl.reporter_found) AS n
	FROM legalhist.reporters r
	LEFT JOIN legalhist.whitelist wl ON wl.reporter_standard = r.reporter_standard
	GROUP BY r.reporter_standard
	ORDER BY r.reporter_standard
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

// statusClass maps a linking status to a CSS class.
func statusClass(status *string) string {
	if status == nil {
		return "status-unprocessed"
	}
	s := *status
	if strings.HasPrefix(s, "linked") {
		return "status-linked"
	}
	if s == "no_match" {
		return "status-nomatch"
	}
	if s == "skipped_junk" {
		return "status-skip"
	}
	return "status-unprocessed"
}

// StatusClass returns a CSS class based on the linking status.
func (r *ReporterCite) StatusClass() string {
	return statusClass(r.Status)
}

func getReporterVariants(ctx context.Context, db *pgxpool.Pool, reporterStandard string) ([]string, error) {
	slog.Debug("querying reporter variants", "reporter", reporterStandard)
	query := `
	SELECT reporter_found
	FROM legalhist.whitelist
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
	JOIN legalhist.whitelist wl ON cu.reporter_abbr = wl.reporter_found
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
	ReporterAbbr string          `json:"reporterAbbr"`
	Count        int             `json:"count"`
	Matches      []ReporterMatch `json:"matches"`
}

// ReporterMatch is a potential reporter_standard match with its CAP info.
type ReporterMatch struct {
	Standard    string `json:"standard"`
	ReporterCap string `json:"reporterCap"`
	Score       int    `json:"score"`
}

func getUnwhitelistedReporters(ctx context.Context, db *pgxpool.Pool) ([]UnwhitelistedReporter, error) {
	slog.Debug("querying unwhitelisted reporters")
	query := `
	SELECT cu.reporter_abbr, count(*) AS n
	FROM moml_citations.citations_unlinked cu
	LEFT JOIN legalhist.whitelist wl
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

// getDistinctReporterStandards returns the canonical list of reporter_standard
// values from legalhist.reporters.
func getDistinctReporterStandards(ctx context.Context, db *pgxpool.Pool) ([]string, error) {
	slog.Debug("querying distinct reporter standards")
	query := `
	SELECT reporter_standard
	FROM legalhist.reporters
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

// getCapInfoMap returns a map of reporter_standard → reporter_cap for standards
// that have a non-empty reporter_cap value.
func getCapInfoMap(ctx context.Context, db *pgxpool.Pool) (map[string]string, error) {
	slog.Debug("querying cap info map")
	query := `
	SELECT reporter_standard, reporter_cap
	FROM legalhist.reporters
	WHERE reporter_cap IS NOT NULL AND reporter_cap != ''
	`
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying cap info: %w", err)
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var std, cap string
		if err := rows.Scan(&std, &cap); err != nil {
			return nil, fmt.Errorf("scanning cap info: %w", err)
		}
		m[std] = cap
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
func computeMatches(abbr string, standards []string, capMap map[string]string) []ReporterMatch {
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
		if cap, ok := capMap[c.standard]; ok {
			m.ReporterCap = cap
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
	TotalRawCites         int
	Reporters             []ReporterStats `json:"Reporters,omitempty"`
}

// TotalLinked returns the sum of all linked statuses.
func (d *DashboardData) TotalLinked() int {
	return d.LinkedCAP + d.LinkedEnglishReports + d.LinkedCodeReporter
}

func getDashboardData(ctx context.Context, db *pgxpool.Pool) (*DashboardData, error) {
	d := &DashboardData{}

	// Get summary metrics (total raw cites and per-status counts) from the
	// precomputed materialized view. The view is refreshed by the cite-linker;
	// reading it here is a small indexed scan instead of aggregating the ~62M-row
	// citations_unlinked and citation_links tables on every request.
	slog.Debug("querying linking dashboard summary view")
	rows, err := db.Query(ctx, `SELECT metric, n FROM moml_citations.linking_dashboard_summary`)
	if err != nil {
		return nil, fmt.Errorf("querying dashboard summary: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var metric string
		var n int
		if err := rows.Scan(&metric, &n); err != nil {
			return nil, fmt.Errorf("scanning dashboard summary: %w", err)
		}
		slog.Debug("dashboard summary row", "metric", metric, "n", n)
		switch metric {
		case "total_raw_cites":
			d.TotalRawCites = n
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
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating dashboard summary: %w", err)
	}
	slog.Debug("finished querying linking dashboard summary view")

	// Get per-reporter linking stats from the precomputed materialized view,
	// ordered by total citations descending (linked + no_match + unprocessed).
	slog.Debug("querying per-reporter linking stats")
	reporterRows, err := db.Query(ctx, `
		SELECT reporter_standard, linked, no_match, unprocessed, uk
		FROM moml_citations.linking_dashboard_reporters
		ORDER BY linked + no_match + unprocessed DESC
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
		"total_raw_cites", d.TotalRawCites,
		"reporters", len(d.Reporters),
	)

	return d, nil
}

// UnmatchedCitation is one aggregated row from
// moml_citations.citations_unmatched_top: a distinct (volume, reporter_standard,
// page) citation that still needs to be linked, with the count n of raw
// citations it aggregates.
type UnmatchedCitation struct {
	Volume           *int
	ReporterStandard *string
	Page             int
	N                int
}

// VolumeDisplay returns the volume for display, or an em dash for single-volume
// reporters (NULL volume).
func (u *UnmatchedCitation) VolumeDisplay() string {
	if u.Volume == nil {
		return "—"
	}
	return strconv.Itoa(*u.Volume)
}

// ReporterDisplay returns the standard reporter, or a placeholder when it is
// NULL (a non-junk whitelist entry with no standard assigned).
func (u *UnmatchedCitation) ReporterDisplay() string {
	if u.ReporterStandard == nil {
		return "(no standard)"
	}
	return *u.ReporterStandard
}

// Cite renders the aggregated citation as it would appear, e.g. "4 Wil. 877".
func (u *UnmatchedCitation) Cite() string {
	if u.Volume == nil {
		return fmt.Sprintf("%s %d", u.ReporterDisplay(), u.Page)
	}
	return fmt.Sprintf("%d %s %d", *u.Volume, u.ReporterDisplay(), u.Page)
}

// DetailURL builds the URL to the reverse-aggregation page for this citation.
// Volume and reporter are omitted when NULL so the handler treats them as NULL.
func (u *UnmatchedCitation) DetailURL() string {
	v := url.Values{}
	v.Set("page", strconv.Itoa(u.Page))
	if u.Volume != nil {
		v.Set("volume", strconv.Itoa(*u.Volume))
	}
	if u.ReporterStandard != nil {
		v.Set("reporter", *u.ReporterStandard)
	}
	return "/unmatched/cites?" + v.Encode()
}

// unmatchedFilters maps a filter key to its SQL predicate on the view. Values
// are fixed literals (never user input) so they are safe to interpolate.
var unmatchedFilters = map[string]string{
	"multi":  "volume IS NOT NULL",
	"single": "volume IS NULL",
	"all":    "true",
}

// NormalizeUnmatchedFilter returns filter if it is a known key, else "multi".
func NormalizeUnmatchedFilter(filter string) string {
	if _, ok := unmatchedFilters[filter]; ok {
		return filter
	}
	return "multi"
}

func getTopUnmatched(ctx context.Context, db *pgxpool.Pool, filter string) ([]UnmatchedCitation, error) {
	pred := unmatchedFilters[NormalizeUnmatchedFilter(filter)]
	slog.Debug("querying top unmatched citations", "filter", filter)
	query := `
	SELECT volume, reporter_standard, page, n
	FROM moml_citations.citations_unmatched_top
	WHERE ` + pred + `
	ORDER BY n DESC, reporter_standard, volume, page
	LIMIT 1000
	`
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying top unmatched citations: %w", err)
	}
	defer rows.Close()

	var results []UnmatchedCitation
	for rows.Next() {
		var u UnmatchedCitation
		if err := rows.Scan(&u.Volume, &u.ReporterStandard, &u.Page, &u.N); err != nil {
			return nil, fmt.Errorf("scanning unmatched citation: %w", err)
		}
		results = append(results, u)
	}
	slog.Debug("fetched top unmatched citations", "filter", filter, "count", len(results))
	return results, rows.Err()
}

// UnmatchedSummary holds aggregate counts over the unmatched-citations view.
type UnmatchedSummary struct {
	Groups int   // number of grouped (distinct) citations
	Cites  int64 // total raw unmatched citations across those groups (sum of n)
}

func getUnmatchedSummary(ctx context.Context, db *pgxpool.Pool, filter string) (UnmatchedSummary, error) {
	pred := unmatchedFilters[NormalizeUnmatchedFilter(filter)]
	slog.Debug("querying unmatched summary", "filter", filter)
	var s UnmatchedSummary
	err := db.QueryRow(ctx, `
		SELECT count(*), COALESCE(sum(n), 0)
		FROM moml_citations.citations_unmatched_top
		WHERE `+pred).Scan(&s.Groups, &s.Cites)
	if err != nil {
		return s, fmt.Errorf("querying unmatched summary: %w", err)
	}
	slog.Debug("fetched unmatched summary", "filter", filter, "groups", s.Groups, "cites", s.Cites)
	return s, nil
}

// UnmatchedCite is a single raw citation occurrence on the reverse-aggregation
// page, with the MOML treatise and page where it was found.
type UnmatchedCite struct {
	ID       uuid.UUID
	Raw      string
	Status   *string
	Treatise string // MOML treatise short (display) title
	Page     string // printed source page, or the MOML page id as a fallback
}

// StatusClass returns a CSS class based on the linking status.
func (c *UnmatchedCite) StatusClass() string {
	return statusClass(c.Status)
}

// unmatchedCitesLimit caps how many raw citations the reverse-aggregation page
// renders, matching the reporter-cites page. Groups larger than this are
// truncated for display; the true total is reported separately.
const unmatchedCitesLimit = 10000

// getUnmatchedCites reverses the aggregation: it returns the raw, still-to-be-
// linked citations that the view aggregated under (volume, reporter_standard,
// page), capped at unmatchedCitesLimit, plus the true total (the view's n).
// Each occurrence carries the MOML treatise title and page where it was found,
// since the raw string is often identical across many treatises. volume and
// reporter may be nil to match NULL values. The unmatched predicate matches
// the view, so total equals the row's n.
func getUnmatchedCites(ctx context.Context, db *pgxpool.Pool, volume *int, reporter *string, page int) ([]UnmatchedCite, int, error) {
	slog.Debug("querying cites for unmatched citation", "page", page)
	query := `
	SELECT cu.id, cu.raw, cl.status,
	       COALESCE(bc.displaytitle, '') AS treatise,
	       COALESCE(NULLIF(mp.sourcepage, ''), cu.moml_page) AS found_page,
	       count(*) OVER() AS total
	FROM moml_citations.citations_unlinked cu
	JOIN legalhist.whitelist wl
	  ON cu.reporter_abbr = wl.reporter_found AND wl.junk = false
	LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
	LEFT JOIN moml.book_citation bc ON bc.psmid = cu.moml_treatise
	LEFT JOIN moml.page mp ON mp.psmid = cu.moml_treatise AND mp.pageid = cu.moml_page
	WHERE wl.reporter_standard IS NOT DISTINCT FROM $1::text
	  AND cu.volume IS NOT DISTINCT FROM $2::int
	  AND cu.page = $3
	  AND (cl.citation_id IS NULL OR cl.status = 'no_match')
	ORDER BY bc.displaytitle NULLS LAST, mp.sourcepage, cu.id
	LIMIT $4
	`
	rows, err := db.Query(ctx, query, reporter, volume, page, unmatchedCitesLimit)
	if err != nil {
		return nil, 0, fmt.Errorf("querying cites for unmatched citation: %w", err)
	}
	defer rows.Close()

	var results []UnmatchedCite
	var total int
	for rows.Next() {
		var c UnmatchedCite
		if err := rows.Scan(&c.ID, &c.Raw, &c.Status, &c.Treatise, &c.Page, &total); err != nil {
			return nil, 0, fmt.Errorf("scanning unmatched cite: %w", err)
		}
		c.Raw = strings.ReplaceAll(c.Raw, "\n", " ")
		c.Raw = strings.ReplaceAll(c.Raw, "\r", " ")
		results = append(results, c)
	}
	slog.Debug("fetched cites for unmatched citation", "page", page, "shown", len(results), "total", total)
	return results, total, rows.Err()
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
