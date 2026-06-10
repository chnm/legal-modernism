package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// cleanRaw collapses the embedded newlines that detected citations often carry
// (OCR line breaks) into spaces, matching how the reporter/unmatched pages
// display raw citation text.
func cleanRaw(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

// isLinked reports whether a link status counts as "linked" (resolved to a
// case in any source). Anything else — no_match, skipped_*, or no row — is
// "not linked".
func isLinked(status *string) bool {
	return status != nil && strings.HasPrefix(*status, "linked_")
}

// ---------------------------------------------------------------------------
// Treatises: browse list
// ---------------------------------------------------------------------------

// TreatiseListItem is one U.S. treatise work (bibliographicid) on the
// /treatises list, with its summed citation counts across all its volumes.
type TreatiseListItem struct {
	BiblioID  string
	Title     string
	Author    *string
	Year      *int
	Vols      int
	N         int
	Linked    int
	NotLinked int
}

// DetailURL links to the work's detail page.
func (t *TreatiseListItem) DetailURL() string {
	return "/treatise?id=" + url.QueryEscape(t.BiblioID)
}

// LinkedPct is the share of the work's citations that are linked, as a rounded
// percentage (0 when the work has no citations). Every citation is either
// linked or not, so LinkedPct + NotLinkedPct == 100 whenever N > 0.
func (t *TreatiseListItem) LinkedPct() int {
	if t.N == 0 {
		return 0
	}
	return (t.Linked*100 + t.N/2) / t.N
}

// NotLinkedPct is the share of the work's citations that are not linked.
func (t *TreatiseListItem) NotLinkedPct() int {
	if t.N == 0 {
		return 0
	}
	return 100 - t.LinkedPct()
}

// treatiseSorts maps a sort key to its ORDER BY expression. Values are fixed
// literals (never user input) so they are safe to interpolate.
var treatiseSorts = map[string]string{
	"cites": "COALESCE(c.n, 0) DESC, t.title",
	"year":  "t.year NULLS LAST, t.title",
	"title": "t.title, t.year",
}

// NormalizeTreatiseSort returns sort if known, else "cites".
func NormalizeTreatiseSort(sort string) string {
	if _, ok := treatiseSorts[sort]; ok {
		return sort
	}
	return "cites"
}

// getUSTreatises returns a page of U.S. treatise works, optionally filtered by a
// title substring, sorted by sort ("cites"|"year"|"title"). Citation totals are
// summed from the treatise_citation_counts materialized view over each work's
// volumes; the view is empty until refreshed, in which case counts read zero.
func getUSTreatises(ctx context.Context, db *pgxpool.Pool, q, sort string, limit, offset int) ([]TreatiseListItem, error) {
	order := treatiseSorts[NormalizeTreatiseSort(sort)]
	slog.Debug("querying US treatises", "q", q, "sort", sort, "limit", limit, "offset", offset)
	query := `
	WITH t AS (
		SELECT bibliographicid, year, title, vols, psmid
		FROM moml.us_treatises
		WHERE ($1 = '' OR title ILIKE '%' || $1 || '%')
	),
	c AS (
		SELECT t.bibliographicid AS id,
		       sum(tcc.n)          AS n,
		       sum(tcc.linked)     AS linked,
		       sum(tcc.not_linked) AS not_linked
		FROM t
		JOIN moml_citations.treatise_citation_counts tcc
		  ON tcc.moml_treatise = ANY(t.psmid)
		GROUP BY t.bibliographicid
	)
	SELECT t.bibliographicid, t.title, t.year, t.vols,
	       (SELECT bc.author_composed FROM moml.book_citation bc
	         WHERE bc.psmid = t.psmid[1]) AS author,
	       COALESCE(c.n, 0), COALESCE(c.linked, 0), COALESCE(c.not_linked, 0)
	FROM t LEFT JOIN c ON c.id = t.bibliographicid
	ORDER BY ` + order + `
	LIMIT $2 OFFSET $3
	`
	rows, err := db.Query(ctx, query, q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying US treatises: %w", err)
	}
	defer rows.Close()

	var results []TreatiseListItem
	for rows.Next() {
		var t TreatiseListItem
		if err := rows.Scan(&t.BiblioID, &t.Title, &t.Year, &t.Vols, &t.Author, &t.N, &t.Linked, &t.NotLinked); err != nil {
			return nil, fmt.Errorf("scanning treatise: %w", err)
		}
		results = append(results, t)
	}
	slog.Debug("fetched US treatises", "count", len(results))
	return results, rows.Err()
}

// ---------------------------------------------------------------------------
// Treatises: work detail (volumes -> pages)
// ---------------------------------------------------------------------------

// TreatiseDetail is a treatise work with its volumes and the pages that carry
// citations.
type TreatiseDetail struct {
	BiblioID string
	Title    string
	Author   *string
	Year     *int
	Subjects []string
	Volumes  []TreatiseVolume
}

// Vols is the number of volumes in the work.
func (t *TreatiseDetail) Vols() int { return len(t.Volumes) }

// MultiVolume reports whether the work has more than one volume.
func (t *TreatiseDetail) MultiVolume() bool { return len(t.Volumes) > 1 }

// TreatiseVolume is one physical volume (psmid) of a work and its pages that
// have detected citations.
type TreatiseVolume struct {
	PSMID       string
	VolumeLabel string
	Title       string
	Pages       []TreatisePageRow
}

// TreatisePageRow is one page of a treatise volume with citation counts.
type TreatisePageRow struct {
	PSMID      string
	PageID     string
	SourcePage string
	Cites      int
	Linked     int
	NotLinked  int
}

// URL links to the page's citation detail.
func (p *TreatisePageRow) URL() string {
	v := url.Values{}
	v.Set("psmid", p.PSMID)
	v.Set("pageid", p.PageID)
	return "/treatise/page?" + v.Encode()
}

// LinkedPct is the share of the page's citations that are linked, as a rounded
// percentage. LinkedPct + NotLinkedPct == 100 whenever the page has citations.
func (p *TreatisePageRow) LinkedPct() int {
	if p.Cites == 0 {
		return 0
	}
	return (p.Linked*100 + p.Cites/2) / p.Cites
}

// NotLinkedPct is the share of the page's citations that are not linked.
func (p *TreatisePageRow) NotLinkedPct() int {
	if p.Cites == 0 {
		return 0
	}
	return 100 - p.LinkedPct()
}

// getTreatiseDetail returns one work's volumes and citation-bearing pages. It
// runs three small queries (volumes, subjects, pages) keyed off the work's
// bibliographicid; the page query uses the citations_unlinked index on
// moml_treatise, so it stays fast and does not depend on any materialized view.
// Returns (nil, nil) when the bibliographicid matches no volumes.
func getTreatiseDetail(ctx context.Context, db *pgxpool.Pool, biblioID string) (*TreatiseDetail, error) {
	slog.Debug("querying treatise detail", "biblioid", biblioID)

	// Volumes, ordered by psmid so multi-volume works read v.1, v.2, ...
	volRows, err := db.Query(ctx, `
		SELECT bi.psmid, bi.year, bc.currentvolume, bc.displaytitle, bc.author_composed
		FROM moml.book_info bi
		LEFT JOIN moml.book_citation bc ON bc.psmid = bi.psmid
		WHERE bi.bibliographicid = $1
		ORDER BY bi.psmid
	`, biblioID)
	if err != nil {
		return nil, fmt.Errorf("querying treatise volumes: %w", err)
	}
	defer volRows.Close()

	d := &TreatiseDetail{BiblioID: biblioID}
	var psmids []string
	volIndex := make(map[string]*TreatiseVolume)
	for volRows.Next() {
		var psmid string
		var year *int
		var curVol, title, author *string
		if err := volRows.Scan(&psmid, &year, &curVol, &title, &author); err != nil {
			return nil, fmt.Errorf("scanning treatise volume: %w", err)
		}
		v := TreatiseVolume{PSMID: psmid}
		if curVol != nil {
			v.VolumeLabel = *curVol
		}
		if title != nil {
			v.Title = *title
			if d.Title == "" {
				d.Title = *title
			}
		}
		if author != nil && *author != "" && d.Author == nil {
			d.Author = author
		}
		if year != nil && (d.Year == nil || *year < *d.Year) {
			d.Year = year
		}
		d.Volumes = append(d.Volumes, v)
		volIndex[psmid] = &d.Volumes[len(d.Volumes)-1]
		psmids = append(psmids, psmid)
	}
	if err := volRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating treatise volumes: %w", err)
	}
	if len(psmids) == 0 {
		return nil, nil
	}

	// Subjects across the work's volumes.
	subjRows, err := db.Query(ctx, `
		SELECT DISTINCT subject FROM moml.book_subject
		WHERE psmid = ANY($1::text[]) AND subject IS NOT NULL AND subject <> ''
		ORDER BY subject
	`, psmids)
	if err != nil {
		return nil, fmt.Errorf("querying treatise subjects: %w", err)
	}
	defer subjRows.Close()
	for subjRows.Next() {
		var s string
		if err := subjRows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scanning subject: %w", err)
		}
		d.Subjects = append(d.Subjects, s)
	}
	if err := subjRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating subjects: %w", err)
	}

	// Pages that have citations, with linked/not-linked counts, grouped into
	// their volume in Go. Index scan on citations_unlinked.moml_treatise.
	pageRows, err := db.Query(ctx, `
		SELECT cu.moml_treatise, cu.moml_page,
		       COALESCE(NULLIF(mp.sourcepage, ''), cu.moml_page) AS sourcepage,
		       count(*) AS cites,
		       count(*) FILTER (WHERE cl.status LIKE 'linked_%') AS linked,
		       count(*) FILTER (WHERE cl.status IS NULL OR cl.status NOT LIKE 'linked_%') AS not_linked
		FROM moml_citations.citations_unlinked cu
		LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
		LEFT JOIN moml.page mp ON mp.psmid = cu.moml_treatise AND mp.pageid = cu.moml_page
		WHERE cu.moml_treatise = ANY($1::text[])
		GROUP BY cu.moml_treatise, cu.moml_page, sourcepage
		ORDER BY cu.moml_treatise, cu.moml_page
	`, psmids)
	if err != nil {
		return nil, fmt.Errorf("querying treatise pages: %w", err)
	}
	defer pageRows.Close()
	for pageRows.Next() {
		var p TreatisePageRow
		if err := pageRows.Scan(&p.PSMID, &p.PageID, &p.SourcePage, &p.Cites, &p.Linked, &p.NotLinked); err != nil {
			return nil, fmt.Errorf("scanning treatise page: %w", err)
		}
		if v, ok := volIndex[p.PSMID]; ok {
			v.Pages = append(v.Pages, p)
		}
	}
	if err := pageRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating treatise pages: %w", err)
	}

	slog.Debug("fetched treatise detail", "biblioid", biblioID, "volumes", len(d.Volumes))
	return d, nil
}

// ---------------------------------------------------------------------------
// Treatise page: the citations detected on one page
// ---------------------------------------------------------------------------

// TreatisePageView is one treatise page with the citations detected on it.
type TreatisePageView struct {
	PSMID      string
	PageID     string
	BiblioID   *string
	Title      *string
	SourcePage string
	Citations  []PageCitation
}

// TreatiseURL links back to the parent work, when known.
func (v *TreatisePageView) TreatiseURL() string {
	if v.BiblioID == nil {
		return ""
	}
	return "/treatise?id=" + url.QueryEscape(*v.BiblioID)
}

// LinkedCount returns how many of the page's citations are linked.
func (v *TreatisePageView) LinkedCount() int {
	n := 0
	for i := range v.Citations {
		if v.Citations[i].IsLinked() {
			n++
		}
	}
	return n
}

// PageCitation is one detected citation on a treatise page, with its link
// status and, when linked, the case it resolved to.
type PageCitation struct {
	ID       uuid.UUID
	Raw      string
	Status   *string
	Source   *string // "cap" | "er" | "code" when linked
	CaseID   *string
	CaseName *string
}

// StatusClass reuses the shared linked/not-linked colour classes.
func (c *PageCitation) StatusClass() string { return statusClass(c.Status) }

// IsLinked reports whether the citation resolved to a case.
func (c *PageCitation) IsLinked() bool { return isLinked(c.Status) }

// CiteURL links to the existing per-citation detail page.
func (c *PageCitation) CiteURL() string { return "/cite?id=" + c.ID.String() }

// CaseURL links to the case detail page, when linked.
func (c *PageCitation) CaseURL() string {
	if c.Source == nil || c.CaseID == nil {
		return ""
	}
	v := url.Values{}
	v.Set("source", *c.Source)
	v.Set("id", *c.CaseID)
	return "/case?" + v.Encode()
}

// getTreatisePage returns one page's header and the citations on it. Returns
// (nil, nil) when the (psmid, pageid) has no citations and no page record.
func getTreatisePage(ctx context.Context, db *pgxpool.Pool, psmid, pageid string) (*TreatisePageView, error) {
	slog.Debug("querying treatise page", "psmid", psmid, "pageid", pageid)
	view := &TreatisePageView{PSMID: psmid, PageID: pageid, SourcePage: pageid}

	// Header: treatise title, bibliographicid, source page number.
	var title *string
	var biblioID *string
	var sourcePage *string
	err := db.QueryRow(ctx, `
		SELECT bc.displaytitle, bi.bibliographicid,
		       (SELECT NULLIF(mp.sourcepage, '') FROM moml.page mp
		         WHERE mp.psmid = $1 AND mp.pageid = $2)
		FROM moml.book_info bi
		LEFT JOIN moml.book_citation bc ON bc.psmid = bi.psmid
		WHERE bi.psmid = $1
	`, psmid, pageid).Scan(&title, &biblioID, &sourcePage)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("querying page header: %w", err)
	}
	view.Title = title
	view.BiblioID = biblioID
	if sourcePage != nil && *sourcePage != "" {
		view.SourcePage = *sourcePage
	}

	// Citations on the page.
	rows, err := db.Query(ctx, `
		SELECT cu.id, cu.raw, cl.status,
		       CASE WHEN cl.cap_case_id IS NOT NULL THEN 'cap'
		            WHEN cl.er_case_id IS NOT NULL THEN 'er'
		            WHEN cl.code_reporter_id IS NOT NULL THEN 'code' END AS source,
		       COALESCE(cl.cap_case_id::text, cl.er_case_id, cl.code_reporter_id::text) AS case_id,
		       COALESCE(cc.name_abbreviation, er.er_name, code.name) AS case_name
		FROM moml_citations.citations_unlinked cu
		LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
		LEFT JOIN cap.cases cc ON cc.id = cl.cap_case_id
		LEFT JOIN english_reports.cases er ON er.id = cl.er_case_id
		LEFT JOIN legalhist.code_reporter code ON code.id = cl.code_reporter_id
		WHERE cu.moml_treatise = $1 AND cu.moml_page = $2
		ORDER BY cl.status LIKE 'linked_%' DESC, cu.raw
	`, psmid, pageid)
	if err != nil {
		return nil, fmt.Errorf("querying page citations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c PageCitation
		if err := rows.Scan(&c.ID, &c.Raw, &c.Status, &c.Source, &c.CaseID, &c.CaseName); err != nil {
			return nil, fmt.Errorf("scanning page citation: %w", err)
		}
		c.Raw = cleanRaw(c.Raw)
		view.Citations = append(view.Citations, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating page citations: %w", err)
	}
	if len(view.Citations) == 0 && view.Title == nil {
		return nil, nil
	}
	slog.Debug("fetched treatise page", "psmid", psmid, "pageid", pageid, "citations", len(view.Citations))
	return view, nil
}

// getPageOCRText returns the OCR text for one treatise page, loaded on demand by
// the "Load OCR text" button rather than with the page itself.
func getPageOCRText(ctx context.Context, db *pgxpool.Pool, psmid, pageid string) (string, error) {
	slog.Debug("querying page OCR text", "psmid", psmid, "pageid", pageid)
	var text *string
	err := db.QueryRow(ctx, `
		SELECT ocrtext FROM moml.page_ocrtext WHERE psmid = $1 AND pageid = $2
	`, psmid, pageid).Scan(&text)
	if err != nil {
		return "", fmt.Errorf("querying page OCR text: %w", err)
	}
	if text == nil {
		return "", nil
	}
	return *text, nil
}

// ---------------------------------------------------------------------------
// Cases: browse list ranked by treatise-page citations
// ---------------------------------------------------------------------------

// caseSourceLabels maps the source code stored in case_citation_counts to a
// short human label.
var caseSourceLabels = map[string]string{
	"cap":  "CAP",
	"er":   "Eng. Rep.",
	"code": "Code rep.",
}

// CaseListItem is one cited case on the /cases ranking.
type CaseListItem struct {
	Source    string
	CaseID    string
	PageCount int
	CiteCount int
	Name      *string
	Year      *int
	Court     *string
	URL       *string // external link (CAP frontend), when available
}

// SourceLabel returns the short human label for the case's source.
func (c *CaseListItem) SourceLabel() string {
	if l, ok := caseSourceLabels[c.Source]; ok {
		return l
	}
	return c.Source
}

// DetailURL links to the case detail page.
func (c *CaseListItem) DetailURL() string {
	v := url.Values{}
	v.Set("source", c.Source)
	v.Set("id", c.CaseID)
	return "/case?" + v.Encode()
}

// getTopCases returns a page of cited cases ranked by the number of distinct
// treatise pages that cite them, joining each matview row to its source table.
func getTopCases(ctx context.Context, db *pgxpool.Pool, limit, offset int) ([]CaseListItem, error) {
	slog.Debug("querying top cases", "limit", limit, "offset", offset)
	query := `
	WITH top AS (
		SELECT source, case_id, page_count, cite_count,
		       CASE WHEN source IN ('cap', 'code') THEN case_id::bigint END AS num_id
		FROM moml_citations.case_citation_counts
		ORDER BY page_count DESC, source, case_id
		LIMIT $1 OFFSET $2
	)
	SELECT t.source, t.case_id, t.page_count, t.cite_count,
	       COALESCE(cc.name_abbreviation, er.er_name, code.name) AS name,
	       COALESCE(cc.decision_year, er.er_year, code.decision_year) AS year,
	       COALESCE(ct.name, er.court, code.court_name) AS court,
	       cc.frontend_url
	FROM top t
	LEFT JOIN cap.cases cc ON t.source = 'cap' AND cc.id = t.num_id
	LEFT JOIN cap.courts ct ON ct.id = cc.court
	LEFT JOIN english_reports.cases er ON t.source = 'er' AND er.id = t.case_id
	LEFT JOIN legalhist.code_reporter code ON t.source = 'code' AND code.id = t.num_id
	ORDER BY t.page_count DESC, t.source, t.case_id
	`
	rows, err := db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying top cases: %w", err)
	}
	defer rows.Close()

	var results []CaseListItem
	for rows.Next() {
		var c CaseListItem
		if err := rows.Scan(&c.Source, &c.CaseID, &c.PageCount, &c.CiteCount, &c.Name, &c.Year, &c.Court, &c.URL); err != nil {
			return nil, fmt.Errorf("scanning case: %w", err)
		}
		results = append(results, c)
	}
	slog.Debug("fetched top cases", "count", len(results))
	return results, rows.Err()
}

// ---------------------------------------------------------------------------
// Cases: case detail (the treatise pages that cite a case)
// ---------------------------------------------------------------------------

// CaseDetail is one case with the treatise pages that cite it.
type CaseDetail struct {
	Source    string
	CaseID    string
	Name      *string
	Year      *int
	Court     *string
	Citation  *string
	URL       *string // external link, when available
	PageCount int
	Pages     []CitingPage
}

// SourceLabel returns the short human label for the case's source.
func (c *CaseDetail) SourceLabel() string {
	if l, ok := caseSourceLabels[c.Source]; ok {
		return l
	}
	return c.Source
}

// CitingPage is one treatise page that cites a case.
type CitingPage struct {
	PSMID      string
	PageID     string
	BiblioID   *string
	Treatise   *string
	SourcePage string
	Cites      int
}

// PageURL links to the treatise page's citation detail.
func (p *CitingPage) PageURL() string {
	v := url.Values{}
	v.Set("psmid", p.PSMID)
	v.Set("pageid", p.PageID)
	return "/treatise/page?" + v.Encode()
}

// TreatiseURL links to the parent work, when known.
func (p *CitingPage) TreatiseURL() string {
	if p.BiblioID == nil {
		return ""
	}
	return "/treatise?id=" + url.QueryEscape(*p.BiblioID)
}

// caseLinkColumns maps a case source to the citation_links column holding its id.
// Values are fixed literals, never user input.
var caseLinkColumns = map[string]string{
	"cap":  "cap_case_id",
	"er":   "er_case_id",
	"code": "code_reporter_id",
}

// caseLinkStatuses maps a case source to the link status that selects it.
var caseLinkStatuses = map[string]string{
	"cap":  "linked_cap",
	"er":   "linked_english_reports",
	"code": "linked_code_reporter",
}

// ValidCaseSource reports whether source is one of the three known sources.
func ValidCaseSource(source string) bool {
	_, ok := caseLinkColumns[source]
	return ok
}

// caseDetailCitingLimit caps how many citing pages the case detail page renders.
const caseDetailCitingLimit = 2000

// getCaseDetail returns one case's metadata and the treatise pages that cite it.
// source must be one of "cap", "er", "code" (validate with ValidCaseSource
// before calling). The citing-pages query filters on the source's id column,
// which is covered by a partial index on citation_links.
func getCaseDetail(ctx context.Context, db *pgxpool.Pool, source, id string) (*CaseDetail, error) {
	slog.Debug("querying case detail", "source", source, "id", id)
	d := &CaseDetail{Source: source, CaseID: id}

	if err := scanCaseMeta(ctx, db, d); err != nil {
		return nil, err
	}

	col := caseLinkColumns[source]
	status := caseLinkStatuses[source]
	// The id is a bigint for cap/code and text for er; bind it as text and cast
	// the column to text so one query serves all three sources safely.
	query := `
	SELECT cu.moml_treatise, cu.moml_page,
	       bi.bibliographicid,
	       bc.displaytitle,
	       COALESCE(NULLIF(mp.sourcepage, ''), cu.moml_page) AS sourcepage,
	       count(*) AS cites
	FROM moml_citations.citation_links cl
	JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
	LEFT JOIN moml.book_info bi ON bi.psmid = cu.moml_treatise
	LEFT JOIN moml.book_citation bc ON bc.psmid = cu.moml_treatise
	LEFT JOIN moml.page mp ON mp.psmid = cu.moml_treatise AND mp.pageid = cu.moml_page
	WHERE cl.status = '` + status + `' AND cl.` + col + `::text = $1
	GROUP BY cu.moml_treatise, cu.moml_page, bi.bibliographicid, bc.displaytitle, sourcepage
	ORDER BY cites DESC, bc.displaytitle NULLS LAST, cu.moml_page
	LIMIT $2
	`
	rows, err := db.Query(ctx, query, id, caseDetailCitingLimit)
	if err != nil {
		return nil, fmt.Errorf("querying citing pages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p CitingPage
		if err := rows.Scan(&p.PSMID, &p.PageID, &p.BiblioID, &p.Treatise, &p.SourcePage, &p.Cites); err != nil {
			return nil, fmt.Errorf("scanning citing page: %w", err)
		}
		d.Pages = append(d.Pages, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating citing pages: %w", err)
	}
	d.PageCount = len(d.Pages)
	slog.Debug("fetched case detail", "source", source, "id", id, "pages", d.PageCount)
	return d, nil
}

// scanCaseMeta fills the case's display metadata from the appropriate source
// table. A missing row leaves the metadata fields nil (the citing pages are
// still shown).
func scanCaseMeta(ctx context.Context, db *pgxpool.Pool, d *CaseDetail) error {
	switch d.Source {
	case "cap":
		// cap.cases.volume is a barcode, not a human volume number, so build the
		// display citation from cap.citations, preferring the official form.
		err := db.QueryRow(ctx, `
			SELECT cc.name_abbreviation, cc.decision_year, ct.name,
			       (SELECT cite FROM cap.citations
			         WHERE "case" = cc.id ORDER BY (type = 'official') DESC LIMIT 1),
			       cc.frontend_url
			FROM cap.cases cc
			LEFT JOIN cap.courts ct ON ct.id = cc.court
			WHERE cc.id = $1::bigint
		`, d.CaseID).Scan(&d.Name, &d.Year, &d.Court, &d.Citation, &d.URL)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("querying CAP case meta: %w", err)
		}
	case "er":
		err := db.QueryRow(ctx, `
			SELECT er_name, er_year, court, er_cite, er_url
			FROM english_reports.cases WHERE id = $1
		`, d.CaseID).Scan(&d.Name, &d.Year, &d.Court, &d.Citation, &d.URL)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("querying English Reports case meta: %w", err)
		}
	case "code":
		err := db.QueryRow(ctx, `
			SELECT name, decision_year, court_name, official_citation
			FROM legalhist.code_reporter WHERE id = $1::bigint
		`, d.CaseID).Scan(&d.Name, &d.Year, &d.Court, &d.Citation)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("querying code reporter case meta: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Normalized citations: browse list and detail
// ---------------------------------------------------------------------------

// NormalizedListItem is one normalized citation string on the /normalized list.
type NormalizedListItem struct {
	Cite        string
	CiteCount   int
	LinkedCount int
	PageCount   int
}

// DetailURL links to the normalized citation's detail page.
func (n *NormalizedListItem) DetailURL() string {
	return "/normalized/cite?c=" + url.QueryEscape(n.Cite)
}

// getTopNormalized returns a page of normalized citations ranked by occurrence
// count, optionally filtered by an anchored prefix (uses the text_pattern_ops
// index on the materialized view).
func getTopNormalized(ctx context.Context, db *pgxpool.Pool, q string, limit, offset int) ([]NormalizedListItem, error) {
	slog.Debug("querying top normalized citations", "q", q, "limit", limit, "offset", offset)
	query := `
	SELECT cite_normalized, cite_count, linked_count, page_count
	FROM moml_citations.normalized_citation_counts
	WHERE ($1 = '' OR cite_normalized LIKE $1 || '%')
	ORDER BY cite_count DESC, cite_normalized
	LIMIT $2 OFFSET $3
	`
	rows, err := db.Query(ctx, query, q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying top normalized citations: %w", err)
	}
	defer rows.Close()

	var results []NormalizedListItem
	for rows.Next() {
		var n NormalizedListItem
		if err := rows.Scan(&n.Cite, &n.CiteCount, &n.LinkedCount, &n.PageCount); err != nil {
			return nil, fmt.Errorf("scanning normalized citation: %w", err)
		}
		results = append(results, n)
	}
	slog.Debug("fetched top normalized citations", "count", len(results))
	return results, rows.Err()
}

// NormalizedCite is one detected instance of a normalized citation, on a
// particular treatise page.
type NormalizedCite struct {
	ID         uuid.UUID
	Raw        string
	Status     *string
	BiblioID   *string
	Treatise   *string
	PSMID      string
	PageID     string
	SourcePage string
}

// StatusClass reuses the shared linked/not-linked colour classes.
func (c *NormalizedCite) StatusClass() string { return statusClass(c.Status) }

// IsLinked reports whether the instance resolved to a case.
func (c *NormalizedCite) IsLinked() bool { return isLinked(c.Status) }

// CiteURL links to the existing per-citation detail page.
func (c *NormalizedCite) CiteURL() string { return "/cite?id=" + c.ID.String() }

// PageURL links to the treatise page the citation was found on.
func (c *NormalizedCite) PageURL() string {
	v := url.Values{}
	v.Set("psmid", c.PSMID)
	v.Set("pageid", c.PageID)
	return "/treatise/page?" + v.Encode()
}

// TreatiseURL links to the parent work, when known.
func (c *NormalizedCite) TreatiseURL() string {
	if c.BiblioID == nil {
		return ""
	}
	return "/treatise?id=" + url.QueryEscape(*c.BiblioID)
}

// normalizedCitesLimit caps how many instances the normalized detail page
// renders; larger groups are truncated for display and the true total reported
// separately, matching the unmatched-cites page.
const normalizedCitesLimit = 5000

// getNormalizedCites returns the detected instances of one normalized citation
// string, on the treatise pages where they appear, capped at
// normalizedCitesLimit, plus the true total. Relies on the index on
// citation_links.cite_normalized.
func getNormalizedCites(ctx context.Context, db *pgxpool.Pool, cite string) ([]NormalizedCite, int, error) {
	slog.Debug("querying instances of normalized citation", "cite", cite)
	query := `
	SELECT cu.id, cu.raw, cl.status,
	       bi.bibliographicid, bc.displaytitle,
	       cu.moml_treatise, cu.moml_page,
	       COALESCE(NULLIF(mp.sourcepage, ''), cu.moml_page) AS sourcepage,
	       count(*) OVER() AS total
	FROM moml_citations.citation_links cl
	JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
	LEFT JOIN moml.book_info bi ON bi.psmid = cu.moml_treatise
	LEFT JOIN moml.book_citation bc ON bc.psmid = cu.moml_treatise
	LEFT JOIN moml.page mp ON mp.psmid = cu.moml_treatise AND mp.pageid = cu.moml_page
	WHERE cl.cite_normalized = $1
	ORDER BY bc.displaytitle NULLS LAST, cu.moml_treatise, cu.moml_page
	LIMIT $2
	`
	rows, err := db.Query(ctx, query, cite, normalizedCitesLimit)
	if err != nil {
		return nil, 0, fmt.Errorf("querying instances of normalized citation: %w", err)
	}
	defer rows.Close()

	var results []NormalizedCite
	var total int
	for rows.Next() {
		var c NormalizedCite
		if err := rows.Scan(&c.ID, &c.Raw, &c.Status, &c.BiblioID, &c.Treatise, &c.PSMID, &c.PageID, &c.SourcePage, &total); err != nil {
			return nil, 0, fmt.Errorf("scanning normalized instance: %w", err)
		}
		c.Raw = cleanRaw(c.Raw)
		results = append(results, c)
	}
	slog.Debug("fetched instances of normalized citation", "cite", cite, "shown", len(results), "total", total)
	return results, total, rows.Err()
}

// parsePageParam reads a 1-based ?page= value, defaulting to 1.
func parsePageParam(s string) int {
	if s == "" {
		return 1
	}
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 {
		return 1
	}
	return p
}
