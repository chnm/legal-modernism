package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The comparison of our CAP-to-CAP links with CAP's own citation graph (issues
// #74, #349): the second half of the CAP linking dashboard, and its two
// drill-downs, one citing case's edges and the disagreeing edges to one cited
// reporter. The unit is the edge, a citing case and a cited CAP case, not the
// citation: a case that cites another three times is one edge on either side.
//
// The dashboard and the reporter page read the cap_graph_* materialized views
// (db/migrations/20261020120100_opinion-citations-cap-graph.sql), which state
// the comparison's scope. The case page reads the live tables, so it also
// works for a case outside the scope, and says whether the case is in it.

// agreement is how two edge sets compare: the edges in both, in ours only and
// in theirs only.
type agreement struct {
	InBoth     int `json:"inBoth"`
	OursOnly   int `json:"oursOnly"`
	TheirsOnly int `json:"theirsOnly"`
}

// Ours and Theirs are the size of each edge set; Edges is their union.
func (a agreement) Ours() int   { return a.InBoth + a.OursOnly }
func (a agreement) Theirs() int { return a.InBoth + a.TheirsOnly }
func (a agreement) Edges() int  { return a.InBoth + a.OursOnly + a.TheirsOnly }

// RecallPct is the share of CAP's edges we also have; PrecisionPct the share
// of ours CAP also has. Both are a dash when the denominator is zero.
func (a agreement) RecallPct() string    { return pct(a.InBoth, a.Theirs()) }
func (a agreement) PrecisionPct() string { return pct(a.InBoth, a.Ours()) }

func (a *agreement) add(b agreement) {
	a.InBoth += b.InBoth
	a.OursOnly += b.OursOnly
	a.TheirsOnly += b.TheirsOnly
}

// ratio is part/whole, or nil when whole is zero, which the charts leave as a
// gap rather than plot as zero.
func ratio(part, whole int) *float64 {
	if whole == 0 {
		return nil
	}
	r := float64(part) / float64(whole)
	return &r
}

// graphRow is the agreement of one year, cited reporter or citing
// jurisdiction, in the shape the charts read.
type graphRow struct {
	Key   int64  `json:"key"`
	Label string `json:"label"`
	Name  string `json:"name,omitempty"`
	agreement
	Recall    *float64 `json:"recall"`
	Precision *float64 `json:"precision"`
	// OursURL and TheirsURL are a reporter's two drill-downs.
	OursURL   string `json:"oursURL,omitempty"`
	TheirsURL string `json:"theirsURL,omitempty"`
}

func newGraphRow(key int64, label, name string, a agreement) graphRow {
	return graphRow{
		Key: key, Label: label, Name: name, agreement: a,
		Recall: ratio(a.InBoth, a.Theirs()), Precision: ratio(a.InBoth, a.Ours()),
	}
}

// caseDistribution is how the edges spread over the citing cases in scope.
// The arrays are the 50th, 90th and 99th percentiles and the maximum of each
// count per case, a case with no edge on either side counting as zeros.
type caseDistribution struct {
	Cases      int
	NoEdges    int // neither graph has an edge from the case
	Agreeing   int // the two have the same edges, at least one
	OnlyTheirs int // we have no edge from the case, CAP has some
	OnlyOurs   int // CAP has no edge from the case, we have some
	InBoth     [4]int
	OursOnly   [4]int
	TheirsOnly [4]int
}

// graphCase is a citing case and its agreement, for the lists of the cases
// where the two graphs differ most.
type graphCase struct {
	CaseRef
	Jurisdiction *string
	agreement
}

// GraphURL is the case's comparison page.
func (c graphCase) GraphURL() string {
	id, _ := strconv.ParseInt(c.ID, 10, 64)
	return capGraphCaseURL(id)
}

// capGraph is the comparison section of the CAP dashboard. A section whose view
// is not populated stays empty, and the page carries a notice.
type capGraph struct {
	Available     bool // cap_graph_agreement could be read
	Total         graphRow
	Years         []graphRow
	Reporters     []graphRow // the reporterLimit with the most disagreement
	ReporterCount int
	Jurisdictions []graphRow
	Cases         *caseDistribution
	TopOurs       []graphCase
	TopTheirs     []graphCase
	Spellings     []UnwhitelistedReporter
	SpellingsURL  string
}

// graphCaseList is a list of citing cases and the side it is ranked by, "ours"
// or "theirs", for the cap_graph_cases partial.
type graphCaseList struct {
	Rows []graphCase
	Side string
}

func (g *capGraph) OursList() graphCaseList { return graphCaseList{Rows: g.TopOurs, Side: "ours"} }
func (g *capGraph) TheirsList() graphCaseList {
	return graphCaseList{Rows: g.TopTheirs, Side: "theirs"}
}

// Chart is the data the page's script draws.
func (g *capGraph) Chart() map[string]any {
	return map[string]any{"years": g.Years, "reporters": g.Reporters}
}

const (
	graphReporterLimit = 40
	graphTopCases      = 25
	graphSpellings     = 100
)

// getCAPGraph reads every section of the comparison. A view that is missing or
// not populated becomes a notice on page, once per view; any other error is
// returned.
func (s *server) getCAPGraph(ctx context.Context, page *Page) (*capGraph, error) {
	g := &capGraph{SpellingsURL: capCorpus.WhitelistURL()}

	err := s.loadGraphAgreement(ctx, g)
	if err := optional(err, page, "opinion_citations.cap_graph_agreement"); err != nil {
		return nil, err
	}
	g.Available = err == nil

	err = s.loadGraphCases(ctx, g)
	if err := optional(err, page, "opinion_citations.cap_graph_case_agreement"); err != nil {
		return nil, err
	}

	g.Spellings, err = s.getUnwhitelistedReporters(ctx, capCorpus, graphSpellings)
	if err := optional(err, page, "opinion_citations.top_reporters"); err != nil {
		return nil, err
	}
	return g, nil
}

// loadGraphAgreement fills the totals and the breakdowns by year, cited
// reporter and citing jurisdiction from cap_graph_agreement, about 213K rows,
// so each roll-up is a scan of it.
func (s *server) loadGraphAgreement(ctx context.Context, g *capGraph) error {
	const sums = `sum(a.in_both)::bigint, sum(a.ours_only)::bigint, sum(a.theirs_only)::bigint`
	scanAgreement := func(rows pgx.Rows, dest ...any) (agreement, error) {
		var a agreement
		err := rows.Scan(append(dest, &a.InBoth, &a.OursOnly, &a.TheirsOnly)...)
		return a, err
	}

	years, err := collect(ctx, s.db, `
		SELECT a.citing_year, `+sums+`
		FROM opinion_citations.cap_graph_agreement a
		GROUP BY a.citing_year
		ORDER BY a.citing_year`, nil,
		func(rows pgx.Rows) (graphRow, error) {
			var year int64
			a, err := scanAgreement(rows, &year)
			return newGraphRow(year, strconv.FormatInt(year, 10), "", a), err
		})
	if err != nil {
		return fmt.Errorf("agreement by year: %w", err)
	}
	g.Years = years
	var total agreement
	for _, y := range years {
		total.add(y.agreement)
	}
	g.Total = newGraphRow(0, "All years", "", total)

	reporters, err := collect(ctx, s.db, `
		SELECT a.cited_reporter, coalesce(r.short_name, a.cited_reporter::text), coalesce(r.full_name, ''),
		       count(*) OVER (), `+sums+`
		FROM opinion_citations.cap_graph_agreement a
		LEFT JOIN cap.reporters r ON r.id = a.cited_reporter
		GROUP BY a.cited_reporter, r.short_name, r.full_name
		ORDER BY sum(a.ours_only + a.theirs_only) DESC, a.cited_reporter
		LIMIT $1`, []any{graphReporterLimit},
		func(rows pgx.Rows) (graphRow, error) {
			var id int64
			var label, name string
			a, err := scanAgreement(rows, &id, &label, &name, &g.ReporterCount)
			row := newGraphRow(id, label, name, a)
			row.OursURL = capGraphReporterURL(id, "ours")
			row.TheirsURL = capGraphReporterURL(id, "theirs")
			return row, err
		})
	if err != nil {
		return fmt.Errorf("agreement by cited reporter: %w", err)
	}
	g.Reporters = reporters

	jurisdictions, err := collect(ctx, s.db, `
		SELECT a.citing_jurisdiction, coalesce(j.name, a.citing_jurisdiction::text), coalesce(j.name_long, ''), `+sums+`
		FROM opinion_citations.cap_graph_agreement a
		LEFT JOIN cap.jurisdictions j ON j.id = a.citing_jurisdiction
		GROUP BY a.citing_jurisdiction, j.name, j.name_long
		ORDER BY sum(a.in_both + a.ours_only + a.theirs_only) DESC, a.citing_jurisdiction`, nil,
		func(rows pgx.Rows) (graphRow, error) {
			var id int64
			var label, name string
			a, err := scanAgreement(rows, &id, &label, &name)
			return newGraphRow(id, label, name, a), err
		})
	if err != nil {
		return fmt.Errorf("agreement by citing jurisdiction: %w", err)
	}
	g.Jurisdictions = jurisdictions
	return nil
}

// loadGraphCases fills the distribution over the citing cases and the cases
// with the most edges on one side only, from cap_graph_case_agreement.
func (s *server) loadGraphCases(ctx context.Context, g *capGraph) error {
	d := &caseDistribution{}
	var inBoth, oursOnly, theirsOnly []int64
	var maxBoth, maxOurs, maxTheirs int
	err := s.db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE in_both + ours_only + theirs_only = 0),
		       count(*) FILTER (WHERE in_both > 0 AND ours_only = 0 AND theirs_only = 0),
		       count(*) FILTER (WHERE in_both + ours_only = 0 AND theirs_only > 0),
		       count(*) FILTER (WHERE in_both + theirs_only = 0 AND ours_only > 0),
		       percentile_disc(ARRAY[0.5, 0.9, 0.99]) WITHIN GROUP (ORDER BY in_both),
		       percentile_disc(ARRAY[0.5, 0.9, 0.99]) WITHIN GROUP (ORDER BY ours_only),
		       percentile_disc(ARRAY[0.5, 0.9, 0.99]) WITHIN GROUP (ORDER BY theirs_only),
		       coalesce(max(in_both), 0), coalesce(max(ours_only), 0), coalesce(max(theirs_only), 0)
		FROM opinion_citations.cap_graph_case_agreement`).Scan(
		&d.Cases, &d.NoEdges, &d.Agreeing, &d.OnlyTheirs, &d.OnlyOurs,
		&inBoth, &oursOnly, &theirsOnly, &maxBoth, &maxOurs, &maxTheirs)
	if err != nil {
		return fmt.Errorf("case distribution: %w", err)
	}
	d.InBoth = quantiles(inBoth, maxBoth)
	d.OursOnly = quantiles(oursOnly, maxOurs)
	d.TheirsOnly = quantiles(theirsOnly, maxTheirs)
	g.Cases = d

	top := func(column string) ([]graphCase, error) {
		return collect(ctx, s.db, `
			SELECT a.citing_case_id::text, c.name_abbreviation, a.citing_year, capcite.cite, j.name,
			       a.in_both, a.ours_only, a.theirs_only
			FROM opinion_citations.cap_graph_case_agreement a
			JOIN cap.cases c ON c.id = a.citing_case_id
			LEFT JOIN cap.jurisdictions j ON j.id = a.citing_jurisdiction
			LEFT JOIN LATERAL (
			  SELECT ci.cite FROM cap.citations ci WHERE ci."case" = a.citing_case_id
			  ORDER BY (ci.type = 'official') DESC, ci.cite LIMIT 1
			) capcite ON true
			ORDER BY a.`+column+` DESC, a.citing_case_id
			LIMIT $1`, []any{graphTopCases},
			func(rows pgx.Rows) (graphCase, error) {
				c := graphCase{CaseRef: CaseRef{Source: "cap"}}
				return c, rows.Scan(&c.ID, &c.Name, &c.Year, &c.Cite, &c.Jurisdiction,
					&c.InBoth, &c.OursOnly, &c.TheirsOnly)
			})
	}
	if g.TopOurs, err = top("ours_only"); err != nil {
		return fmt.Errorf("cases with the most ours-only edges: %w", err)
	}
	if g.TopTheirs, err = top("theirs_only"); err != nil {
		return fmt.Errorf("cases with the most theirs-only edges: %w", err)
	}
	return nil
}

// quantiles packs three percentiles and a maximum; missing percentiles (an
// empty view) are zero.
func quantiles(p []int64, maximum int) [4]int {
	var q [4]int
	for i := 0; i < 3 && i < len(p); i++ {
		q[i] = int(p[i])
	}
	q[3] = maximum
	return q
}

// The per-case page.

// capCitation is one citation we detected in a CAP case's opinions and what
// the linker made of it.
type capCitation struct {
	ID             string
	Opinion        string // the opinion's type: majority, dissent, concurrence
	Raw            string
	Volume         *int
	ReporterAbbr   string
	Page           int
	Chip           Chip
	CapCaseID      *int64
	CiteNormalized *string
	Name           *DetectedName
}

// Explanations of an edge CAP's graph has and our links lack, from the most
// to the least direct evidence.
const (
	explainDetected  = "detected"    // we detected a citation to the case and did not link it to the case
	explainProbable  = "probable"    // a citation of ours falls in the case's page span under a spelling of its reporter
	explainInText    = "in_text"     // one of the case's cites occurs in an opinion, and we detected nothing for it
	explainNotInText = "not_in_text" // the case's cites do not occur in the opinions' text
)

// explanation is why an edge CAP's graph has is not among our links, with the
// citations of ours it rests on.
type explanation struct {
	Kind      string
	Citations []capCitation
}

// Label is the explanation in words.
func (x explanation) Label() string {
	switch x.Kind {
	case explainDetected:
		return "Detected, not linked to this case"
	case explainProbable:
		return "Probably detected under another spelling"
	case explainInText:
		return "In the opinion text, not detected"
	case explainNotInText:
		return "Not in the opinion text"
	}
	return ""
}

// Gloss says what the explanation rests on, for a tooltip.
func (x explanation) Gloss() string {
	switch x.Kind {
	case explainDetected:
		return "We detected a citation to this case; its status and tier say why it did not link here."
	case explainProbable:
		return "A citation of ours falls in this case's pages under a spelling of its reporter, which the linker did not resolve to it."
	case explainInText:
		return "One of this case's cites occurs in an opinion's text, but the detector found nothing for it."
	case explainNotInText:
		return "None of this case's cites occurs in the text of the opinions, so CAP's graph read the citation somewhere cap.opinions does not hold."
	}
	return ""
}

// Class is the explanation's colour class: the linker's, the detector's, or
// the text's.
func (x explanation) Class() string {
	switch x.Kind {
	case explainDetected:
		return "chip-nomatch"
	case explainProbable, explainInText:
		return "chip-skip"
	}
	return "chip-junk"
}

// capEdge is an edge from the citing case to a CAP case in either graph.
type capEdge struct {
	Cited       CaseRef
	FirstPage   *int
	LastPage    *int
	Cites       []string // the cited case's CAP citations, official first
	InOurs      bool
	InTheirs    bool
	CiteCount   int
	Ours        []capCitation // our citations linked to the cited case
	Explanation explanation   // for an edge only CAP's graph has
}

// capOtherLink is a link of ours CAP's graph cannot hold: to the English
// Reports, the code reporter, or a stub case.
type capOtherLink struct {
	CaseRef
	CiteCount int
}

// capCasePage is one citing case: its record, whether the comparison covers
// it, its edges grouped by which graph has them, our links outside CAP, and
// the citations of ours that explain nothing above.
type capCasePage struct {
	Page
	Case        *CaseDetail
	InMetadata  bool  // in cap_citations.metadata, so CAP's graph covers it
	OpinionsN   int   // its opinions in cap.opinions
	InScope     *bool // in cap_graph_scope; nil when the view is unavailable
	Counts      agreement
	TheirsOnly  []capEdge
	OursOnly    []capEdge
	Both        []capEdge
	Other       []capOtherLink
	SelfCites   int
	Unexplained []capCitation
	Detected    int
}

func (p *capCasePage) CAPCaseURL() string { return caseURL("cap", p.Case.ID) }

// handleCAPGraphCaseForm sends the dashboard's case form, ?id=, to the case's
// page.
func (s *server) handleCAPGraphCaseForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	if err != nil || id <= 0 {
		s.badRequest(w, r, "A CAP case is addressed by its numeric id.")
		return
	}
	http.Redirect(w, r, capGraphCaseURL(id), http.StatusSeeOther)
}

func (s *server) handleCAPGraphCase(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.badRequest(w, r, "A CAP case is addressed by its numeric id: /linking/cap/cases/{id}.")
		return
	}
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()

	d := &CaseDetail{CaseRef: CaseRef{Source: "cap", ID: strconv.FormatInt(id, 10)}}
	found, err := s.getCaseMeta(ctx, d)
	if err != nil {
		s.serverError(w, r, err, "loading the case", "id", id)
		return
	}
	if !found {
		s.notFound(w, r, fmt.Sprintf("There is no CAP case with the id %d.", id))
		return
	}
	p := &capCasePage{Case: d}
	p.Page = Page{
		Title:   d.Display() + ": against CAP's graph",
		Section: "linking",
		Crumbs: []Crumb{{Label: "Detecting & linking", URL: "/linking"}, {Label: capCorpus.Label, URL: capCorpus.Base},
			{Label: truncate(d.Display(), 60)}},
	}
	if err := s.loadCAPCase(ctx, id, p); err != nil {
		s.serverError(w, r, err, "comparing the case's edges with CAP's graph", "id", id)
		return
	}
	s.render(w, r, "cap_graph_case.html", http.StatusOK, p)
}

// loadCAPCase reads the case's coverage, its edges in both graphs, our
// citations and the opinions' text, and sorts each edge CAP's graph has that
// ours lacks into an explanation.
func (s *server) loadCAPCase(ctx context.Context, id int64, p *capCasePage) error {
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM cap_citations.metadata WHERE id = $1::bigint),
		       (SELECT count(*) FROM cap.opinions WHERE "case" = $1::bigint)`, id).Scan(&p.InMetadata, &p.OpinionsN)
	if err != nil {
		return fmt.Errorf("case coverage: %w", err)
	}
	var inScope bool
	err = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM opinion_citations.cap_graph_scope WHERE id = $1::bigint)`, id).Scan(&inScope)
	if err := optional(err, &p.Page, "opinion_citations.cap_graph_scope"); err != nil {
		return fmt.Errorf("case scope: %w", err)
	}
	if err == nil {
		p.InScope = &inScope
	}

	edges, err := s.getCAPCaseEdges(ctx, id)
	if err != nil {
		return err
	}
	cits, err := s.getCAPCaseCitations(ctx, id)
	if err != nil {
		return err
	}
	p.Detected = len(cits)
	texts, err := collect(ctx, s.db, `SELECT text FROM cap.opinions WHERE "case" = $1::bigint ORDER BY id`, []any{id},
		func(rows pgx.Rows) (string, error) {
			var t string
			return t, rows.Scan(&t)
		})
	if err != nil {
		return fmt.Errorf("opinion text: %w", err)
	}

	used := make(map[string]bool)
	for i := range edges {
		e := &edges[i]
		for _, c := range cits {
			if c.CapCaseID != nil && strconv.FormatInt(*c.CapCaseID, 10) == e.Cited.ID {
				e.Ours = append(e.Ours, c)
				used[c.ID] = true
			}
		}
	}
	for i := range edges {
		e := &edges[i]
		switch {
		case e.InOurs && e.InTheirs:
			p.Counts.InBoth++
			p.Both = append(p.Both, *e)
		case e.InOurs:
			p.Counts.OursOnly++
			p.OursOnly = append(p.OursOnly, *e)
		default:
			p.Counts.TheirsOnly++
			e.Explanation = explainTheirsOnly(*e, cits, texts)
			for _, c := range e.Explanation.Citations {
				used[c.ID] = true
			}
			p.TheirsOnly = append(p.TheirsOnly, *e)
		}
	}

	if p.Other, p.SelfCites, err = s.getCAPCaseOtherLinks(ctx, id); err != nil {
		return err
	}
	for _, c := range cits {
		if !used[c.ID] && !c.Chip.Linked() {
			p.Unexplained = append(p.Unexplained, c)
		}
	}
	return nil
}

// getCAPCaseEdges reads the case's edges in both graphs, live. ours and theirs
// are cap_graph_edges' definitions without the scope, which concerns the
// citing case and which the page states on its own: our CAP-to-CAP links
// without self-cites, and CAP's distinct cites_to. Theirs-only edges come first
// in the cited case's reporter order, which is how the page lists them.
func (s *server) getCAPCaseEdges(ctx context.Context, id int64) ([]capEdge, error) {
	edges, err := collect(ctx, s.db, `
		WITH ours AS (
		  SELECT cap_case_id AS cited, cite_count
		  FROM opinion_citations.case_case_citations
		  WHERE citing_case_id = $1::bigint AND source = 'cap' AND NOT self_cite
		),
		theirs AS (
		  SELECT DISTINCT cites_to::bigint AS cited
		  FROM cap_citations.citations
		  WHERE cites_from = $1::bigint AND cites_to <> cites_from
		)
		SELECT k.id::text, k.name_abbreviation, k.decision_year,
		       o.cited IS NOT NULL, t.cited IS NOT NULL, coalesce(o.cite_count, 0),
		       k.first_page, k.last_page,
		       (SELECT array_agg(ci.cite ORDER BY (ci.type = 'official') DESC, ci.cite)
		        FROM cap.citations ci WHERE ci."case" = k.id)
		FROM ours o
		FULL OUTER JOIN theirs t ON t.cited = o.cited
		JOIN cap.cases k ON k.id = coalesce(o.cited, t.cited)
		LEFT JOIN cap.volumes v ON v.barcode = k.volume
		LEFT JOIN cap.reporters r ON r.id = k.reporter
		ORDER BY r.short_name, v.volume_number, k.first_page, k.id`, []any{id},
		func(rows pgx.Rows) (capEdge, error) {
			e := capEdge{Cited: CaseRef{Source: "cap"}}
			err := rows.Scan(&e.Cited.ID, &e.Cited.Name, &e.Cited.Year,
				&e.InOurs, &e.InTheirs, &e.CiteCount,
				&e.FirstPage, &e.LastPage, &e.Cites)
			if len(e.Cites) > 0 {
				e.Cited.Cite = &e.Cites[0]
			}
			return e, err
		})
	if err != nil {
		return nil, fmt.Errorf("case edges: %w", err)
	}
	return edges, nil
}

// getCAPCaseCitations reads every citation we detected in the case's opinions,
// with its link, in opinion and text order of detection.
func (s *server) getCAPCaseCitations(ctx context.Context, id int64) ([]capCitation, error) {
	cits, err := collect(ctx, s.db, `
		SELECT cu.id::text, o.type, cu.raw, cu.volume, cu.reporter_abbr, cu.page,
		       cl.status, cl.match_tier, cl.cap_case_id, cl.cite_normalized,
		       cu.case_name, cu.case_name_source, `+caseNameAgreementSQL("cu.case_name", "k.name")+`
		FROM opinion_citations.citations_unlinked cu
		JOIN cap.opinions o ON o.id = cu.cap_opinion
		LEFT JOIN opinion_citations.citation_links cl ON cl.citation_id = cu.id
		LEFT JOIN cap.cases k ON k.id = cl.cap_case_id
		WHERE cu.cap_case = $1::bigint
		ORDER BY o.id, cu.reporter_abbr, cu.volume, cu.page`, []any{id},
		func(rows pgx.Rows) (capCitation, error) {
			var c capCitation
			var status, tier, caseName, nameSource *string
			var agrees *bool
			err := rows.Scan(&c.ID, &c.Opinion, &c.Raw, &c.Volume, &c.ReporterAbbr, &c.Page,
				&status, &tier, &c.CapCaseID, &c.CiteNormalized, &caseName, &nameSource, &agrees)
			c.Chip = chipFor(status, tier)
			c.Name = detectedNameFrom(caseName, nameSource, agrees)
			return c, err
		})
	if err != nil {
		return nil, fmt.Errorf("case citations: %w", err)
	}
	return cits, nil
}

// getCAPCaseOtherLinks reads the case's links to cases outside CAP, which CAP's
// graph cannot hold, and the citations behind its self-cites.
func (s *server) getCAPCaseOtherLinks(ctx context.Context, id int64) ([]capOtherLink, int, error) {
	other, err := collect(ctx, s.db, `
		SELECT `+caseMetaColumns("x")+`, x.cite_count
		FROM opinion_citations.case_case_citations x
		`+caseMetaJoins("x")+`
		WHERE x.citing_case_id = $1::bigint AND x.source <> 'cap'
		ORDER BY x.cite_count DESC, 2`, []any{id},
		func(rows pgx.Rows) (capOtherLink, error) {
			var source, key, name, cite *string
			var year *int
			var l capOtherLink
			if err := rows.Scan(&source, &key, &name, &year, &cite, &l.CiteCount); err != nil {
				return l, err
			}
			if ref := caseRefFrom(source, key, name, year, cite); ref != nil {
				l.CaseRef = *ref
			}
			return l, nil
		})
	if err != nil {
		return nil, 0, fmt.Errorf("links outside CAP: %w", err)
	}
	var self int
	err = s.db.QueryRow(ctx, `
		SELECT coalesce(sum(cite_count), 0)::bigint FROM opinion_citations.case_case_citations
		WHERE citing_case_id = $1::bigint AND self_cite`, id).Scan(&self)
	if err != nil {
		return nil, 0, fmt.Errorf("self-cites: %w", err)
	}
	return other, self, nil
}

// explainTheirsOnly says why an edge CAP's graph has is not among our links,
// trying the evidence from the most direct to the least:
//
//  1. detected: a citation of ours normalizes to one of the cited case's CAP
//     cites, so the detector found it and the linker did not link it here; its
//     status and tier say why (no match, an anachronism refused, or linked to
//     another case that begins on the same page).
//  2. probable: a citation of ours has the volume of one of the cited case's
//     CAP cites, a page in the case's span counted from that cite's page, and a
//     spelling that begins as that cite's reporter does, spaces and periods
//     aside (Mo. l. c. for Mo., Texas for Tex., 12 Wheat. 441 for 12 Wheat.
//     419): the cite was found, as a pin cite to an interior page, which the
//     linker does not resolve (#393), or under a spelling it did not resolve
//     to this case.
//  3. in_text: one of the case's cites occurs in an opinion's text, so the
//     detector missed it.
//  4. not_in_text: none of them occurs in the opinions, so CAP's graph read
//     the citation somewhere cap.opinions does not hold.
func explainTheirsOnly(e capEdge, cits []capCitation, texts []string) explanation {
	cites := make(map[string]bool, len(e.Cites))
	for _, c := range e.Cites {
		cites[c] = true
	}
	var detected []capCitation
	for _, c := range cits {
		if c.CiteNormalized != nil && cites[*c.CiteNormalized] {
			detected = append(detected, c)
		}
	}
	if len(detected) > 0 {
		return explanation{Kind: explainDetected, Citations: detected}
	}

	span := 0
	if e.FirstPage != nil && e.LastPage != nil && *e.LastPage > *e.FirstPage {
		span = *e.LastPage - *e.FirstPage
	}
	var probable []capCitation
	for _, c := range cits {
		if c.Volume == nil {
			continue
		}
		for _, cite := range e.Cites {
			vol, reporter, page, ok := parseCite(cite)
			if ok && *c.Volume == vol && c.Page >= page && c.Page <= page+span &&
				sameReporterStem(c.ReporterAbbr, reporter) {
				probable = append(probable, c)
				break
			}
		}
	}
	if len(probable) > 0 {
		return explanation{Kind: explainProbable, Citations: probable}
	}

	for _, cite := range e.Cites {
		re := citePattern(cite)
		if re == nil {
			continue
		}
		for _, t := range texts {
			if re.MatchString(t) {
				return explanation{Kind: explainInText}
			}
		}
	}
	return explanation{Kind: explainNotInText}
}

// sameReporterStem reports whether one spelling of a reporter begins with the
// other once spaces and periods are dropped and case is ignored: "N. Y." and
// "N.Y.", "Pa. St." and "Pa.", "Mo. l. c." and "Mo.", "Texas" and "Tex.".
func sameReporterStem(a, b string) bool {
	squeeze := strings.NewReplacer(" ", "", ".", "")
	a, b = strings.ToLower(squeeze.Replace(a)), strings.ToLower(squeeze.Replace(b))
	if a == "" || b == "" {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// parseCite splits a CAP cite such as "12 Wheat. 419" into its volume,
// reporter and page; ok is false for a cite not of that shape (a LEXIS or
// docket identifier).
func parseCite(cite string) (volume int, reporter string, page int, ok bool) {
	fields := strings.Fields(cite)
	if len(fields) < 3 {
		return 0, "", 0, false
	}
	volume, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, "", 0, false
	}
	page, err = strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return 0, "", 0, false
	}
	return volume, strings.Join(fields[1:len(fields)-1], " "), page, true
}

// citePattern matches a CAP cite such as "19 Mo. 233" in running text, loosely
// enough for the ways an opinion prints it: any spacing after a period
// ("N. Y." for "N.Y."), a comma before the page, and no digit on either side.
// Nil for a cite too short to be a volume, reporter and page.
func citePattern(cite string) *regexp.Regexp {
	fields := strings.Fields(cite)
	if len(fields) < 3 {
		return nil
	}
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = strings.ReplaceAll(regexp.QuoteMeta(f), `\.`, `\.\s*`)
	}
	return regexp.MustCompile(`(?:^|[^0-9])` + strings.Join(parts, `\s*,?\s*`) + `(?:[^0-9]|$)`)
}

// The reporter page.

// capReporterEdge is one edge to the reporter that one graph has and the other
// lacks.
type capReporterEdge struct {
	Citing CaseRef
	Cited  CaseRef
	Raws   []string // ours side: the strings we linked
}

// CitingGraphURL is the citing case's comparison page.
func (e capReporterEdge) CitingGraphURL() string {
	id, _ := strconv.ParseInt(e.Citing.ID, 10, 64)
	return capGraphCaseURL(id)
}

// capReporterCase is a cited case with many edges on the chosen side.
type capReporterCase struct {
	CaseRef
	Edges int
}

// capReporterPage is a sample of the edges to one cited CAP reporter that
// one graph has and the other lacks.
type capReporterPage struct {
	Page
	ID        int64
	ShortName string
	FullName  *string
	Side      string // "ours" or "theirs"
	Totals    *agreement
	TopCases  []capReporterCase
	Edges     []capReporterEdge
	Nav       Pagination
	OursURL   string
	TheirsURL string
}

// SideLabel says which edges the page lists.
func (p *capReporterPage) SideLabel() string {
	if p.Side == "ours" {
		return "Edges only we have"
	}
	return "Edges only CAP's graph has"
}

const capReporterPageSize = 100

func (s *server) handleCAPGraphReporter(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.badRequest(w, r, "A CAP reporter is addressed by its numeric id: /linking/cap/reporters/{id}.")
		return
	}
	side := r.URL.Query().Get("side")
	switch side {
	case "":
		side = "theirs"
	case "ours", "theirs":
	default:
		s.badRequest(w, r, "The side is ours (edges only we have) or theirs (edges only CAP's graph has).")
		return
	}
	page := parsePage(r.URL.Query().Get("page"))
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()

	p := &capReporterPage{ID: id, Side: side, OursURL: capGraphReporterURL(id, "ours"), TheirsURL: capGraphReporterURL(id, "theirs")}
	err = s.db.QueryRow(ctx, `SELECT coalesce(short_name, id::text), full_name FROM cap.reporters WHERE id = $1`, id).
		Scan(&p.ShortName, &p.FullName)
	if isNoRows(err) {
		s.notFound(w, r, fmt.Sprintf("There is no CAP reporter with the id %d.", id))
		return
	}
	if err != nil {
		s.serverError(w, r, err, "loading the reporter", "id", id)
		return
	}
	p.Page = Page{
		Title:   p.ShortName + ": " + strings.ToLower(p.SideLabel()[:1]) + p.SideLabel()[1:],
		Section: "linking",
		Crumbs: []Crumb{{Label: "Detecting & linking", URL: "/linking"}, {Label: capCorpus.Label, URL: capCorpus.Base},
			{Label: p.ShortName}},
	}

	err = s.loadCAPReporter(ctx, p, page)
	if err := optional(err, &p.Page, "opinion_citations.cap_graph_edges"); err != nil {
		s.serverError(w, r, err, "listing the reporter's edges", "id", id)
		return
	}
	p.Nav = paginate(r, page, capReporterPageSize, len(p.Edges), -1)
	s.render(w, r, "cap_graph_reporter.html", http.StatusOK, p)
}

// loadCAPReporter reads the reporter's totals, the cited cases with the most
// edges on the side, and a page of those edges, in cited-case order, through
// the partial index on cap_graph_edges' disagreeing rows.
func (s *server) loadCAPReporter(ctx context.Context, p *capReporterPage, page int) error {
	var a agreement
	err := s.db.QueryRow(ctx, `
		SELECT coalesce(sum(in_both), 0)::bigint, coalesce(sum(ours_only), 0)::bigint,
		       coalesce(sum(theirs_only), 0)::bigint
		FROM opinion_citations.cap_graph_agreement WHERE cited_reporter = $1`, p.ID).
		Scan(&a.InBoth, &a.OursOnly, &a.TheirsOnly)
	if err != nil {
		return fmt.Errorf("reporter totals: %w", err)
	}
	p.Totals = &a
	ours := p.Side == "ours"

	p.TopCases, err = collect(ctx, s.db, `
		WITH top AS (
		  SELECT cited_case_id, count(*) AS n
		  FROM opinion_citations.cap_graph_edges
		  WHERE cited_reporter = $1 AND in_ours = $2 AND in_ours <> in_theirs
		  GROUP BY cited_case_id
		  ORDER BY count(*) DESC, cited_case_id
		  LIMIT $3
		)
		SELECT k.id::text, k.name_abbreviation, k.decision_year, capcite.cite, top.n
		FROM top
		JOIN cap.cases k ON k.id = top.cited_case_id
		LEFT JOIN LATERAL (
		  SELECT ci.cite FROM cap.citations ci WHERE ci."case" = k.id
		  ORDER BY (ci.type = 'official') DESC, ci.cite LIMIT 1
		) capcite ON true
		ORDER BY top.n DESC, k.id`, []any{p.ID, ours, graphTopCases},
		func(rows pgx.Rows) (capReporterCase, error) {
			c := capReporterCase{CaseRef: CaseRef{Source: "cap"}}
			return c, rows.Scan(&c.ID, &c.Name, &c.Year, &c.Cite, &c.Edges)
		})
	if err != nil {
		return fmt.Errorf("reporter's top cases: %w", err)
	}

	// The strings behind an ours-only edge are found from the citing case's
	// citations, a few dozen rows by citations_unlinked_cap_case_idx, rather
	// than from the cited case's links, which can run to thousands.
	p.Edges, err = collect(ctx, s.db, `
		WITH page AS (
		  SELECT citing_case_id, cited_case_id
		  FROM opinion_citations.cap_graph_edges
		  WHERE cited_reporter = $1 AND in_ours = $2 AND in_ours <> in_theirs
		  ORDER BY cited_case_id, citing_case_id
		  LIMIT $3 OFFSET $4
		)
		SELECT c.id::text, c.name_abbreviation, c.decision_year, ccite.cite,
		       k.id::text, k.name_abbreviation, k.decision_year, kcite.cite,
		       CASE WHEN $2 THEN (
		         SELECT array_agg(DISTINCT cu.raw ORDER BY cu.raw)
		         FROM opinion_citations.citations_unlinked cu
		         JOIN opinion_citations.citation_links cl ON cl.citation_id = cu.id
		         WHERE cu.cap_case = p.citing_case_id AND cl.cap_case_id = p.cited_case_id)
		       END
		FROM page p
		JOIN cap.cases c ON c.id = p.citing_case_id
		JOIN cap.cases k ON k.id = p.cited_case_id
		LEFT JOIN LATERAL (
		  SELECT ci.cite FROM cap.citations ci WHERE ci."case" = c.id
		  ORDER BY (ci.type = 'official') DESC, ci.cite LIMIT 1
		) ccite ON true
		LEFT JOIN LATERAL (
		  SELECT ci.cite FROM cap.citations ci WHERE ci."case" = k.id
		  ORDER BY (ci.type = 'official') DESC, ci.cite LIMIT 1
		) kcite ON true
		ORDER BY p.cited_case_id, p.citing_case_id`,
		[]any{p.ID, ours, capReporterPageSize, (page - 1) * capReporterPageSize},
		func(rows pgx.Rows) (capReporterEdge, error) {
			e := capReporterEdge{Citing: CaseRef{Source: "cap"}, Cited: CaseRef{Source: "cap"}}
			return e, rows.Scan(&e.Citing.ID, &e.Citing.Name, &e.Citing.Year, &e.Citing.Cite,
				&e.Cited.ID, &e.Cited.Name, &e.Cited.Year, &e.Cited.Cite, &e.Raws)
		})
	if err != nil {
		return fmt.Errorf("reporter's edges: %w", err)
	}
	return nil
}
