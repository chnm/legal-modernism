package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The cases of a work, edition by edition (issue #437): a matrix with the
// work's treatise editions as its columns, earliest first, the cases their
// linked citations reach as its rows, and a mark where an edition cites a
// case. It shows which cases last through the editions and which enter as the
// case law changes the treatise. The columns can be all the editions or only
// the US or the UK ones. It reads edition_case_citations, so a citation the
// linker did not resolve is not in it. It was built to read Stephen on
// Pleading (work 1107, issue #353) and every case is on one page, which is a
// very long page for the works that cite tens of thousands.

// MatrixEdition is one column of the matrix: an edition, and how the cases it
// cites stand among the other editions shown.
type MatrixEdition struct {
	WorkEdition
	Cited   int // cases the edition cites
	Entered int // of them, the cases no earlier edition cites
	Left    int // of them, the cases no later edition cites
}

// Class is the CSS class that colours the column by jurisdiction.
func (e MatrixEdition) Class() string { return strings.ToLower(e.JurisdictionLabel()) }

// YearLabel is the edition's year, or "n.d.".
func (e MatrixEdition) YearLabel() string {
	if e.Year == nil {
		return "n.d."
	}
	return strconv.Itoa(*e.Year)
}

// PlaceLabel is where the edition was published, "" when the catalogue does
// not say.
func (e MatrixEdition) PlaceLabel() string { return strings.TrimSpace(derefStr(e.Place)) }

// Statement is the head of the edition statement, short enough for a column
// heading.
func (e MatrixEdition) Statement() string { return truncate(statementHead(e.EditionStatement), 26) }

// Name identifies the edition in a sentence: "1841, Philadelphia, 4th
// American ed.".
func (e MatrixEdition) Name() string {
	parts := []string{e.YearLabel()}
	for _, p := range []string{e.PlaceLabel(), statementHead(e.EditionStatement)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// Tooltip is the catalogue record behind the heading, a line for each part.
func (e MatrixEdition) Tooltip() string {
	lines := []string{e.Title}
	for _, p := range []*string{e.EditionStatement, e.Imprint, e.Author} {
		if s := strings.Join(strings.Fields(derefStr(p)), " "); s != "" {
			lines = append(lines, s)
		}
	}
	if e.Derivative {
		lines = append(lines, "Derivative edition")
	}
	return strings.Join(lines, "\n")
}

// statementHead is an edition statement up to its first break: "2nd American
// ed., with large additions / by Francis J. Troubat." is "2nd American ed.".
func statementHead(s *string) string {
	head := derefStr(s)
	if i := strings.IndexAny(head, ",/;"); i >= 0 {
		head = head[:i]
	}
	return strings.TrimSpace(head)
}

// The jurisdictions the columns can be narrowed to: "" is every edition.
var matrixJurisdictions = map[string]string{"": "", "us": "US", "uk": "UK"}

func normalizeMatrixJurisdiction(jur string) string {
	if _, ok := matrixJurisdictions[jur]; ok {
		return jur
	}
	return ""
}

// matrixColumns picks the columns from a work's editions, which come earliest
// first: the editions in the treatise view, all of them or those of one
// jurisdiction. It also counts the treatise editions on each side, whatever
// the filter, and the editions outside the view, which never have a column.
func matrixColumns(editions []WorkEdition, jur string) (cols []MatrixEdition, us, uk, outside int) {
	want := matrixJurisdictions[normalizeMatrixJurisdiction(jur)]
	for _, e := range editions {
		if !e.InTreatises() {
			outside++
			continue
		}
		switch e.JurisdictionLabel() {
		case "US":
			us++
		case "UK":
			uk++
		}
		if want == "" || e.JurisdictionLabel() == want {
			cols = append(cols, MatrixEdition{WorkEdition: e})
		}
	}
	return cols, us, uk, outside
}

// matrixCase is one row as the query returns it: a case, and the columns
// that cite it, as 1-based positions among the editions, with the number of
// citations each makes.
type matrixCase struct {
	CaseRef
	Editions  int
	Cites     int
	First     int
	Last      int
	Positions []int
	Counts    []int
}

// MatrixCell is one edition's cell on a case's row.
type MatrixCell struct {
	Count  int    // citations from the edition to the case, 0 when it does not cite it
	Before bool   // the edition was published before the case was decided
	Class  string // the column's jurisdiction class
	URL    string // the citations, when there are any
}

// Title says how many citations are behind a filled cell.
func (c MatrixCell) Title() string {
	if c.Count == 1 {
		return "1 citation"
	}
	return num(c.Count) + " citations"
}

// MatrixRow is one case on the matrix. Group is the heading of the group the
// row opens, when the order has groups.
type MatrixRow struct {
	CaseRef
	Editions int
	Cites    int
	Cells    []MatrixCell
	Group    string
}

// The orders of the rows, over the aggregate a of matrixCasesSQL and the case
// tables joined to it. Each ends on the case's key, so the order is total.
const matrixCaseKey = "a.cap_case_id, a.er_case_id, a.code_reporter_id, a.stub_cite"

var matrixSorts = map[string]string{
	"editions": "a.editions DESC, a.first_pos, a.cites DESC, " + matrixCaseKey,
	"entered":  "a.first_pos, a.editions DESC, a.last_pos DESC, a.cites DESC, " + matrixCaseKey,
	"left":     "a.last_pos, a.first_pos, a.editions DESC, a.cites DESC, " + matrixCaseKey,
	"year":     caseYearExpr + " NULLS LAST, a.first_pos, a.editions DESC, " + matrixCaseKey,
}

func normalizeMatrixSort(sort string) string {
	if _, ok := matrixSorts[sort]; ok {
		return sort
	}
	return "editions"
}

// matrixEditionsSQL numbers the edition ids of $1 in column order, so the
// position of an edition's row of edition_case_citations is its column.
const matrixEditionsSQL = `e AS (
	  SELECT id, pos::int AS pos FROM unnest($1::text[]) WITH ORDINALITY AS u(id, pos)
	)`

// getMatrixCases returns every case the editions cite, in the given order,
// with the columns that cite it: first_pos and last_pos are the first and
// last edition to do so. The predicate on the ids is the prefix of the unique
// index of edition_case_citations.
func (s *server) getMatrixCases(ctx context.Context, biblioIDs []string, sort string) ([]matrixCase, error) {
	sort = normalizeMatrixSort(sort)
	slog.Debug("querying case matrix", "editions", len(biblioIDs), "sort", sort)
	query := `
	WITH ` + matrixEditionsSQL + `, a AS (
	  SELECT c.cap_case_id, c.er_case_id, c.code_reporter_id, c.stub_cite,
	         count(*) AS editions, sum(c.cite_count)::bigint AS cites,
	         min(e.pos) AS first_pos, max(e.pos) AS last_pos,
	         array_agg(e.pos ORDER BY e.pos) AS positions,
	         array_agg(c.cite_count ORDER BY e.pos) AS counts
	  FROM moml_citations.edition_case_citations c
	  JOIN e ON e.id = c.bibliographicid
	  WHERE c.bibliographicid = ANY($1::text[])
	  GROUP BY 1, 2, 3, 4
	)
	SELECT a.editions, a.cites, a.first_pos, a.last_pos, a.positions, a.counts, ` + caseMetaColumns("a") + `
	FROM a ` + caseMetaJoins("a") + `
	ORDER BY ` + matrixSorts[sort]
	items, err := collect(ctx, s.db, query, []any{biblioIDs}, func(rows pgx.Rows) (matrixCase, error) {
		var m matrixCase
		var source, id, name, cite *string
		var year *int
		err := rows.Scan(&m.Editions, &m.Cites, &m.First, &m.Last, &m.Positions, &m.Counts, &source, &id, &name, &year, &cite)
		if ref := caseRefFrom(source, id, name, year, cite); ref != nil {
			m.CaseRef = *ref
		}
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("case matrix: %w", err)
	}
	return items, nil
}

// getMatrixTotals fills in, for each edition, the cases it cites and how many
// of them it is the first or the last of the editions to cite.
func (s *server) getMatrixTotals(ctx context.Context, biblioIDs []string, editions []MatrixEdition) error {
	rows, err := s.db.Query(ctx, `
		WITH `+matrixEditionsSQL+`, a AS (
		  SELECT min(e.pos) AS first_pos, max(e.pos) AS last_pos, array_agg(e.pos) AS positions
		  FROM moml_citations.edition_case_citations c
		  JOIN e ON e.id = c.bibliographicid
		  WHERE c.bibliographicid = ANY($1::text[])
		  GROUP BY c.cap_case_id, c.er_case_id, c.code_reporter_id, c.stub_cite
		)
		SELECT p.pos, count(*), count(*) FILTER (WHERE p.pos = a.first_pos), count(*) FILTER (WHERE p.pos = a.last_pos)
		FROM a CROSS JOIN LATERAL unnest(a.positions) AS p(pos)
		GROUP BY p.pos`, biblioIDs)
	if err != nil {
		return fmt.Errorf("case matrix totals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pos, cited, entered, left int
		if err := rows.Scan(&pos, &cited, &entered, &left); err != nil {
			return fmt.Errorf("case matrix totals: scanning: %w", err)
		}
		if pos < 1 || pos > len(editions) {
			continue
		}
		e := &editions[pos-1]
		e.Cited, e.Entered, e.Left = cited, entered, left
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("case matrix totals: %w", err)
	}
	return nil
}

// matrixRows lays the cases out against the editions: a cell for every
// edition, filled where it cites the case, and marked where the edition is
// older than the case. In the orders that group the cases, the first row of
// each group carries its heading.
func matrixRows(cases []matrixCase, editions []MatrixEdition, sort string) []MatrixRow {
	rows := make([]MatrixRow, 0, len(cases))
	for i, c := range cases {
		row := MatrixRow{CaseRef: c.CaseRef, Editions: c.Editions, Cites: c.Cites, Cells: make([]MatrixCell, len(editions))}
		for j, e := range editions {
			row.Cells[j].Class = e.Class()
			row.Cells[j].Before = c.Year != nil && e.Year != nil && *e.Year < *c.Year
		}
		for k, pos := range c.Positions {
			if pos < 1 || pos > len(editions) || k >= len(c.Counts) {
				continue
			}
			cell := &row.Cells[pos-1]
			cell.Count = c.Counts[k]
			cell.Before = false
			cell.URL = citationsURL(url.Values{"edition": {editions[pos-1].BiblioID}, "case": {c.Key()}})
		}
		if i == 0 || matrixGroupKey(cases[i-1], sort) != matrixGroupKey(c, sort) {
			row.Group = matrixGroup(c, editions, sort)
		}
		rows = append(rows, row)
	}
	return rows
}

// matrixGroupKey is what the cases of a group share: the first edition to
// cite them, or the last. The other orders have no groups, since the number of
// citing editions and the year are columns of the row.
func matrixGroupKey(c matrixCase, sort string) int {
	switch sort {
	case "entered":
		return c.First
	case "left":
		return c.Last
	}
	return 0
}

// matrixGroup is the heading of the group a case belongs to, "" in an order
// without groups.
func matrixGroup(c matrixCase, editions []MatrixEdition, sort string) string {
	pos := matrixGroupKey(c, sort)
	if pos < 1 || pos > len(editions) {
		return ""
	}
	e := editions[pos-1]
	if sort == "entered" {
		return fmt.Sprintf("First cited in %s: %s", e.Name(), plural(e.Entered, "case"))
	}
	return fmt.Sprintf("Last cited in %s: %s", e.Name(), plural(e.Left, "case"))
}

// plural renders a count with its noun: "1 case", "1,480 cases".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return num(n) + " " + noun + "s"
}

// WorkCasesPage is the page: the work, the editions shown, and the matrix.
type WorkCasesPage struct {
	Page
	WorkID   int
	Title    string
	Author   *string
	Jur      string          // "", "us" or "uk": the editions shown
	Sort     string          // the order of the rows
	Editions []MatrixEdition // the columns
	US       int             // treatise editions on each side, whatever is shown
	UK       int
	Outside  int // editions outside the treatise view, which have no column
	Rows     []MatrixRow
	Cites    int // citations detected in the editions shown, when the counts are there
	Linked   int // of them, linked to a case
}

func (d WorkCasesPage) WorkURL() string { return workURL(d.WorkID) }

// Link is the page with the given order and editions, leaving the defaults
// out of the query string.
func (d WorkCasesPage) Link(sort, jur string) string {
	v := url.Values{}
	if sort = normalizeMatrixSort(sort); sort != "editions" {
		v.Set("sort", sort)
	}
	if jur = normalizeMatrixJurisdiction(jur); jur != "" {
		v.Set("jur", jur)
	}
	if len(v) == 0 {
		return workCasesURL(d.WorkID)
	}
	return workCasesURL(d.WorkID) + "?" + v.Encode()
}

// BothSides reports whether the work has US and UK editions, so that there is
// something to narrow the columns to.
func (d WorkCasesPage) BothSides() bool { return d.US > 0 && d.UK > 0 }

// Shown says which editions the columns are: "All 18 editions (6 UK, 12 US)",
// or "The 12 US editions, of 18".
func (d WorkCasesPage) Shown() string {
	if d.Jur != "" {
		return fmt.Sprintf("The %s, of %d", plural(len(d.Editions), matrixJurisdictions[d.Jur]+" edition"), d.US+d.UK)
	}
	return fmt.Sprintf("All %s (%d UK, %d US)", plural(len(d.Editions), "edition"), d.UK, d.US)
}

// Columns is the width of the table: the five columns that describe the case
// and one for each edition.
func (d WorkCasesPage) Columns() int { return 5 + len(d.Editions) }

// Years is the span of the years of the editions shown.
func (d WorkCasesPage) Years() string {
	var first, last *int
	for _, e := range d.Editions {
		if e.Year == nil {
			continue
		}
		if first == nil || *e.Year < *first {
			first = e.Year
		}
		if last == nil || *e.Year > *last {
			last = e.Year
		}
	}
	return yearSpan(first, last)
}

// LinkedPct is the share of the editions' citations that linked to a case.
func (d WorkCasesPage) LinkedPct() string { return pct(d.Linked, d.Cites) }

func (s *server) handleWorkCases(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		s.notFound(w, r, "There is no work with that id.")
		return
	}
	d := WorkCasesPage{
		WorkID: id,
		Jur:    normalizeMatrixJurisdiction(r.URL.Query().Get("jur")),
		Sort:   normalizeMatrixSort(r.URL.Query().Get("sort")),
	}

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	err = s.db.QueryRow(ctx, `SELECT title, author FROM moml.works WHERE work_id = $1`, id).Scan(&d.Title, &d.Author)
	if isNoRows(err) {
		s.notFound(w, r, "There is no work with that id.")
		return
	}
	if err != nil {
		s.serverError(w, r, err, "loading the work", "work_id", id)
		return
	}
	d.Page = Page{
		Title:   "Cases across the editions of " + d.Title,
		Section: "works",
		Crumbs: []Crumb{
			{Label: "Treatises", URL: "/works"},
			{Label: truncate(d.Title, 60), URL: workURL(id)},
			{Label: "Cases across the editions"},
		},
	}

	// The editions come from the catalogue; their citation counts, which give
	// the linked share, from edition_citation_counts, and are optional.
	editions, err := s.getWorkEditions(ctx, id, true)
	if isUnavailable(err) {
		d.Page.unavailable("moml_citations.edition_citation_counts")
		editions, err = s.getWorkEditions(ctx, id, false)
	}
	if err != nil {
		s.serverError(w, r, err, "listing the work's editions", "work_id", id)
		return
	}
	d.Editions, d.US, d.UK, d.Outside = matrixColumns(editions, d.Jur)
	ids := make([]string, len(d.Editions))
	for i, e := range d.Editions {
		ids[i] = e.BiblioID
		d.Cites += e.Cites
		d.Linked += e.Linked
	}

	if len(ids) > 0 {
		cases, err := s.getMatrixCases(ctx, ids, d.Sort)
		if err == nil {
			err = s.getMatrixTotals(ctx, ids, d.Editions)
		}
		if err := optional(err, &d.Page, "moml_citations.edition_case_citations"); err != nil {
			s.serverError(w, r, err, "laying out the work's cases", "work_id", id)
			return
		}
		d.Rows = matrixRows(cases, d.Editions, d.Sort)
	}
	s.render(w, r, "work_cases.html", http.StatusOK, d)
}
