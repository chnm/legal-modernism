package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

// Draft public visualizations (VISUALIZATIONS.md): charts meant for readers
// outside the project, developed here before they move to the website. Each
// has a page at /viz/<name> that holds the title, source and notes, and an API
// at /api/viz/<name> that serves a plain JSON array the chart module under
// static/viz/ draws. The handlers read nothing but the API's view, so the page
// renders without the database and the script shows the 503 when the view is
// not populated yet. A visualization whose data is not in the database yet
// serves it from the embedded files of data/ instead (data.go).

// vizPage is the data of a visualization page.
type vizPage struct {
	Page
}

// slugPattern is the shape of a cap.jurisdictions slug, which keys the
// jurisdictions API: lower-case letters, digits and hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// handleVizJurisdictions renders the citations-between-jurisdictions chart
// (issue #216). The page is a shell; the script fetches the two APIs below.
func (s *server) handleVizJurisdictions(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "viz_jurisdictions.html", http.StatusOK, vizPage{Page{
		Title: "Citations between jurisdictions", Section: "viz",
	}})
}

// vizJurisdiction is one row of the jurisdictions list: a jurisdiction that
// cites or is cited in opinion_citations.jurisdiction_citations, with its
// totals in each direction and the span of its citing cases' years.
type vizJurisdiction struct {
	Jurisdiction   string `json:"jurisdiction"` // cap.jurisdictions.slug
	Name           string `json:"name"`         // the abbreviation, N.Y.
	NameLong       string `json:"name_long"`    // New York
	Citations      int64  `json:"citations"`    // citations from its cases
	CitingCases    int64  `json:"citing_cases"` // its cases that cite
	CitedCitations int64  `json:"cited_citations"`
	MinYear        *int   `json:"min_year"` // of its citing cases; null when it only is cited
	MaxYear        *int   `json:"max_year"`
}

const vizJurisdictionsSQL = `
WITH citing AS (
    SELECT citing_jurisdiction AS slug,
           sum(citations) AS citations,
           sum(citing_cases) AS citing_cases,
           min(citing_year) AS min_year,
           max(citing_year) AS max_year
      FROM opinion_citations.jurisdiction_citations
     GROUP BY 1
), cited AS (
    SELECT cited_jurisdiction AS slug, sum(citations) AS citations
      FROM opinion_citations.jurisdiction_citations
     GROUP BY 1
)
SELECT j.slug, j.name, j.name_long,
       COALESCE(citing.citations, 0), COALESCE(citing.citing_cases, 0),
       COALESCE(cited.citations, 0),
       citing.min_year, citing.max_year
  FROM cap.jurisdictions j
  LEFT JOIN citing ON citing.slug = j.slug
  LEFT JOIN cited ON cited.slug = j.slug
 WHERE citing.slug IS NOT NULL OR cited.slug IS NOT NULL
 ORDER BY j.name`

func (s *server) getVizJurisdictions(ctx context.Context) ([]vizJurisdiction, error) {
	rows, err := s.db.Query(ctx, vizJurisdictionsSQL)
	if err != nil {
		return nil, fmt.Errorf("querying jurisdictions: %w", err)
	}
	defer rows.Close()
	out := []vizJurisdiction{}
	for rows.Next() {
		var j vizJurisdiction
		if err := rows.Scan(&j.Jurisdiction, &j.Name, &j.NameLong, &j.Citations, &j.CitingCases,
			&j.CitedCitations, &j.MinYear, &j.MaxYear); err != nil {
			return nil, fmt.Errorf("scanning jurisdiction: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// handleVizJurisdictionsAPI serves the list of jurisdictions, which fills the
// chart's menu and sets the year slider's range.
func (s *server) handleVizJurisdictionsAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()
	data, err := s.getVizJurisdictions(ctx)
	if err != nil {
		apiError(w, err, "error querying jurisdictions", capCorpus)
		return
	}
	writeJSON(w, data)
}

// vizJurisdictionEdge is one row of opinion_citations.jurisdiction_citations:
// the citations from the cases of one jurisdiction decided in one year to the
// cases of another (or the same) jurisdiction, and the citing cases making
// them. A citation is one opinion citing one case, counted once however many
// strings cite it.
type vizJurisdictionEdge struct {
	CitingYear         int    `json:"citing_year"`
	CitingJurisdiction string `json:"citing_jurisdiction"`
	CitedJurisdiction  string `json:"cited_jurisdiction"`
	Citations          int64  `json:"citations"`
	CitingCases        int64  `json:"citing_cases"`
}

const vizJurisdictionEdgesSQL = `
SELECT citing_year, citing_jurisdiction, cited_jurisdiction, citations, citing_cases
  FROM opinion_citations.jurisdiction_citations
 WHERE citing_jurisdiction = $1 OR cited_jurisdiction = $1
 ORDER BY citing_year, citing_jurisdiction, cited_jurisdiction`

func (s *server) getVizJurisdictionEdges(ctx context.Context, slug string) ([]vizJurisdictionEdge, error) {
	rows, err := s.db.Query(ctx, vizJurisdictionEdgesSQL, slug)
	if err != nil {
		return nil, fmt.Errorf("querying jurisdiction edges: %w", err)
	}
	defer rows.Close()
	out := []vizJurisdictionEdge{}
	for rows.Next() {
		var e vizJurisdictionEdge
		if err := rows.Scan(&e.CitingYear, &e.CitingJurisdiction, &e.CitedJurisdiction, &e.Citations, &e.CitingCases); err != nil {
			return nil, fmt.Errorf("scanning jurisdiction edge: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// handleVizJurisdictionAPI serves every edge of one jurisdiction, citing and
// cited, year by year; the chart filters the years and picks the direction in
// the browser. An unknown slug is an empty array. The literal route
// /api/viz/jurisdictions/totals takes precedence over this pattern.
func (s *server) handleVizJurisdictionAPI(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !slugPattern.MatchString(slug) {
		http.Error(w, "jurisdiction must be a cap.jurisdictions slug", http.StatusBadRequest)
		return
	}
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()
	data, err := s.getVizJurisdictionEdges(ctx, slug)
	if err != nil {
		apiError(w, err, "error querying jurisdiction edges", capCorpus)
		return
	}
	writeJSON(w, data)
}

// vizJurisdictionTotal is one jurisdiction's citations in one year, in each
// direction, with its home citations left out: what it cites in other
// jurisdictions and what other jurisdictions cite in it. The share measure
// divides a bar by the other jurisdiction's total, so the browser fetches
// these once and takes the denominator for any years and direction itself.
type vizJurisdictionTotal struct {
	Year         int    `json:"year"`
	Jurisdiction string `json:"jurisdiction"`
	CitationsOut int64  `json:"citations_out"` // from its cases to other jurisdictions
	CitationsIn  int64  `json:"citations_in"`  // to its cases from other jurisdictions
}

const vizJurisdictionTotalsSQL = `
WITH outgoing AS (
    SELECT citing_year AS year, citing_jurisdiction AS slug, sum(citations) AS citations
      FROM opinion_citations.jurisdiction_citations
     WHERE citing_jurisdiction <> cited_jurisdiction
     GROUP BY 1, 2
), incoming AS (
    SELECT citing_year AS year, cited_jurisdiction AS slug, sum(citations) AS citations
      FROM opinion_citations.jurisdiction_citations
     WHERE citing_jurisdiction <> cited_jurisdiction
     GROUP BY 1, 2
)
SELECT COALESCE(o.year, i.year), COALESCE(o.slug, i.slug),
       COALESCE(o.citations, 0), COALESCE(i.citations, 0)
  FROM outgoing o
  FULL JOIN incoming i ON i.year = o.year AND i.slug = o.slug
 ORDER BY 1, 2`

func (s *server) getVizJurisdictionTotals(ctx context.Context) ([]vizJurisdictionTotal, error) {
	rows, err := s.db.Query(ctx, vizJurisdictionTotalsSQL)
	if err != nil {
		return nil, fmt.Errorf("querying jurisdiction totals: %w", err)
	}
	defer rows.Close()
	out := []vizJurisdictionTotal{}
	for rows.Next() {
		var t vizJurisdictionTotal
		if err := rows.Scan(&t.Year, &t.Jurisdiction, &t.CitationsOut, &t.CitationsIn); err != nil {
			return nil, fmt.Errorf("scanning jurisdiction total: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// handleVizJurisdictionTotalsAPI serves every jurisdiction's citations by
// year in each direction, the denominators of the share measure.
func (s *server) handleVizJurisdictionTotalsAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()
	data, err := s.getVizJurisdictionTotals(ctx)
	if err != nil {
		apiError(w, err, "error querying jurisdiction totals", capCorpus)
		return
	}
	writeJSON(w, data)
}

// handleVizFieldCode renders the Field Code network (issue #117): figure 5 of
// Funk and Mullen, "The Spine of American Law" (AHR 2018), the codes of civil
// procedure of the nineteenth-century United States connected by the sections
// they borrowed from one another, as an interactive force layout. The page is
// a shell; the script fetches the two arrays embedded from data/, which
// scripts/field-code-network derives from the article's research compendium.
func (s *server) handleVizFieldCode(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "viz_field_code.html", http.StatusOK, vizPage{Page{
		Title: "The migration of the Field Code", Section: "viz",
	}})
}

// stephenWorkID is Stephen on Pleading in moml.works, the work whose
// editions Claude read for citations in issue #439.
const stephenWorkID = 1107

// handleVizStephen renders the citations of the American editions of Stephen
// on Pleading (issue #439), as Claude read and linked them page by page. The
// page is a shell; the script fetches the API below.
func (s *server) handleVizStephen(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "viz_stephen.html", http.StatusOK, vizPage{Page{
		Title: "Stephen on Pleading", Section: "viz",
	}})
}

// vizStephenRow is the citations of cases of one jurisdiction in one American
// edition of Stephen on Pleading: US, UK or Unknown. Every edition with a
// page read has all three rows, a zero included, and the pages read and in
// the copy, so that a partly read edition can be marked.
type vizStephenRow struct {
	Year             int     `json:"year"`
	Bibliographicid  string  `json:"bibliographicid"`
	Psmid            string  `json:"psmid"`
	EditionStatement *string `json:"edition_statement"`
	Jurisdiction     string  `json:"jurisdiction"` // US, UK or Unknown
	Citations        int64   `json:"citations"`
	PagesRead        int64   `json:"pages_read"`
	Pages            int64   `json:"pages"`
}

// vizStephenSQL counts the LLM-read citations (moml_citations.llm_links, not
// the detector's citations_unlinked) in the official copy of each American
// edition of a work: the editions moml.treatises counts, which leaves out the
// copies marked by moml.editions.copy_of, so that the Gale copies also read
// are not counted twice. An edition no page of which is loaded is absent. A
// citation is American when it links to CAP or the New York Code Reporter,
// English when it links to the English Reports, and for a stub case whatever
// its reporter's jurisdiction says (legalhist.stub_cases.reporter_standard,
// a foreign key onto legalhist.reporters, whose jurisdiction is us:xx or
// uk:xx); a citation that matched nothing, or a stub whose reporter has no
// jurisdiction, is Unknown.
const vizStephenSQL = `
WITH copies AS (
    SELECT v.bibliographicid, v.psmid, v.year, v.edition_statement
      FROM moml.editions e
      JOIN moml.treatises t USING (bibliographicid)
      JOIN moml.volumes v USING (bibliographicid)
     WHERE e.work_id = $1 AND t.jurisdiction = 'US'
), editions AS (
    SELECT c.bibliographicid, min(c.psmid) AS psmid, min(c.year) AS year,
           min(c.edition_statement) AS edition_statement,
           count(r.pageid) AS pages_read, count(*) AS pages
      FROM copies c
      JOIN moml.page p USING (psmid)
      LEFT JOIN moml_citations.llm_pages r USING (psmid, pageid)
     GROUP BY c.bibliographicid
    HAVING count(r.pageid) > 0
), links AS (
    SELECT c.bibliographicid,
           CASE WHEN l.status IN ('linked_cap', 'linked_code_reporter') OR rep.jurisdiction LIKE 'us:%' THEN 'US'
                WHEN l.status = 'linked_english_reports' OR rep.jurisdiction LIKE 'uk:%' THEN 'UK'
                ELSE 'Unknown' END AS jurisdiction
      FROM copies c
      JOIN moml_citations.llm_links l USING (psmid)
      LEFT JOIN legalhist.stub_cases s ON s.cite = l.stub_cite
      LEFT JOIN legalhist.reporters rep ON rep.reporter_standard = s.reporter_standard
)
SELECT e.year, e.bibliographicid, e.psmid, e.edition_statement, j.jurisdiction,
       count(l.jurisdiction), e.pages_read, e.pages
  FROM editions e
 CROSS JOIN (VALUES ('US', 1), ('UK', 2), ('Unknown', 3)) AS j (jurisdiction, ord)
  LEFT JOIN links l ON l.bibliographicid = e.bibliographicid AND l.jurisdiction = j.jurisdiction
 GROUP BY e.year, e.bibliographicid, e.psmid, e.edition_statement, j.jurisdiction, j.ord, e.pages_read, e.pages
 ORDER BY e.year, e.bibliographicid, j.ord`

func (s *server) getVizStephen(ctx context.Context) ([]vizStephenRow, error) {
	rows, err := s.db.Query(ctx, vizStephenSQL, stephenWorkID)
	if err != nil {
		return nil, fmt.Errorf("querying Stephen citations: %w", err)
	}
	defer rows.Close()
	out := []vizStephenRow{}
	for rows.Next() {
		var r vizStephenRow
		if err := rows.Scan(&r.Year, &r.Bibliographicid, &r.Psmid, &r.EditionStatement, &r.Jurisdiction,
			&r.Citations, &r.PagesRead, &r.Pages); err != nil {
			return nil, fmt.Errorf("scanning Stephen citations: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// handleVizStephenAPI serves the citations of each American edition of
// Stephen on Pleading by jurisdiction. Before any official copy is loaded it
// is an empty array, which the page says.
func (s *server) handleVizStephenAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()
	data, err := s.getVizStephen(ctx)
	if err != nil {
		apiError(w, err, "error querying Stephen citations", momlCorpus)
		return
	}
	writeJSON(w, data)
}
