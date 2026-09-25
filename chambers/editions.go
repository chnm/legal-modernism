package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
)

// An edition (moml.editions, keyed by bibliographicid) is one printing of a
// work in one or more volumes. Its page shows the catalogue record, the cases
// it cites most, the reporters it cites, and every page on which a citation
// was detected, volume by volume.

// EditionDetail is the edition page.
type EditionDetail struct {
	Page
	BiblioID         string
	Title            string
	Author           *string
	AuthorRole       *string
	EditionStatement *string
	Imprint          *string
	Place            *string
	Year             *int
	Jurisdiction     *string // nil when the edition is outside the treatise view
	Derivative       bool
	TotalVolumes     *int
	Collation        *string
	WorkID           int
	WorkTitle        string
	Subjects         []string
	LoCHeadings      []string
	Gale             GaleLinks
	Counts           *EditionCounts
	Reporters        []ReporterShare
	USCites          int
	UKCites          int
	Volumes          []EditionVolume
	TopCases         TopCasesView
	ExcludePincite   bool
}

func (e *EditionDetail) URL() string         { return editionURL(e.BiblioID) }
func (e *EditionDetail) WorkURL() string     { return workURL(e.WorkID) }
func (e *EditionDetail) InTreatises() bool   { return e.Jurisdiction != nil }
func (e *EditionDetail) AllCasesURL() string { return editionCasesURL(e.BiblioID) }
func (e *EditionDetail) CitationsURL() string {
	return citationsURL(url.Values{"edition": {e.BiblioID}})
}

// ReporterCites is the number of whitelisted citations behind the reporter
// profile, for the jurisdiction shares.
func (e *EditionDetail) ReporterCites() int {
	n := 0
	for _, r := range e.Reporters {
		n += r.Cites
	}
	return n
}

// EditionCounts is the edition's row of edition_citation_counts.
type EditionCounts struct {
	Cites       int
	Linked      int
	Cases       int
	CasesDirect int
}

func (c EditionCounts) LinkedPct() string { return pct(c.Linked, c.Cites) }

// ReporterShare is one reporter the edition cites and how often.
type ReporterShare struct {
	Standard     string
	Title        *string
	Jurisdiction *string
	Cites        int
	Linked       int
}

func (r ReporterShare) URL() string { return reporterURL(r.Standard) }

// Side is US or UK from the reporter's jurisdiction ("us:ma", "uk:kb"), or "".
func (r ReporterShare) Side() string {
	if r.Jurisdiction == nil {
		return ""
	}
	side, _, _ := strings.Cut(*r.Jurisdiction, ":")
	return strings.ToUpper(side)
}

// EditionVolume is one volume of the edition with the pages that carry
// citations.
type EditionVolume struct {
	PSMID            string
	Number           int // current_volume; 0 when the volume is not numbered
	Title            string
	EditionStatement *string
	Imprint          *string
	Year             *int
	TotalPages       *int
	Gale             GaleLinks
	Pages            []EditionPageRow
	Cites            int
	Linked           int
}

// Label names the volume for a heading.
func (v EditionVolume) Label() string {
	if v.Number == 0 {
		return "Unnumbered volume"
	}
	return fmt.Sprintf("Volume %d", v.Number)
}

// EditionPageRow is one page of a volume with its citation counts.
type EditionPageRow struct {
	PSMID      string
	PageID     string
	SourcePage string // printed page label, "" when the scan has none
	Type       string // moml.page.type: bodyPage, index, TOC, ...
	Section    *string
	Cites      int
	Linked     int
}

func (p EditionPageRow) URL() string { return pageURL(p.PSMID, p.PageID) }

// Label is the printed page, or the image number when there is none.
func (p EditionPageRow) Label() string {
	if p.SourcePage != "" {
		return "p. " + p.SourcePage
	}
	return "image " + strings.TrimLeft(strings.TrimRight(p.PageID, "0"), "0")
}

// getEditionHeader loads the catalogue record of an edition, taking the
// varying fields (title, statement, imprint, year, product link) from its
// first volume. The treatise view is looked up by id, which is fast even
// though scanning it is not. Returns (nil, nil) when there is no such edition.
func (s *server) getEditionHeader(ctx context.Context, id string) (*EditionDetail, error) {
	slog.Debug("querying edition", "bibliographicid", id)
	d := &EditionDetail{BiblioID: id}
	var productLink *string
	err := s.db.QueryRow(ctx, `
		SELECT e.author, e.author_role, e.publication_place, e.total_volumes, e.book_collation, e.derivative,
		       e.work_id, w.title,
		       v.title, v.edition_statement, v.imprint, v.year, v.product_link,
		       (SELECT t.jurisdiction FROM moml.treatises t WHERE t.bibliographicid = e.bibliographicid)
		FROM moml.editions e
		JOIN moml.works w ON w.work_id = e.work_id
		JOIN LATERAL (
		  SELECT min(vv.year) AS year,
		         (array_agg(vv.display_title ORDER BY vv.current_volume, vv.psmid))[1] AS title,
		         (array_agg(vv.edition_statement ORDER BY vv.current_volume, vv.psmid))[1] AS edition_statement,
		         (array_agg(vv.imprint ORDER BY vv.current_volume, vv.psmid))[1] AS imprint,
		         (array_agg(vv.product_link ORDER BY vv.current_volume, vv.psmid))[1] AS product_link
		  FROM moml.volumes vv WHERE vv.bibliographicid = e.bibliographicid
		) v ON true
		WHERE e.bibliographicid = $1`, id).Scan(
		&d.Author, &d.AuthorRole, &d.Place, &d.TotalVolumes, &d.Collation, &d.Derivative,
		&d.WorkID, &d.WorkTitle,
		&d.Title, &d.EditionStatement, &d.Imprint, &d.Year, &productLink, &d.Jurisdiction)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("edition header: %w", err)
	}
	d.Gale = galeLinks(productLink, "")
	return d, nil
}

// getEditionCounts reads the edition's row of edition_citation_counts; nil
// when the edition is not a treatise.
func (s *server) getEditionCounts(ctx context.Context, id string) (*EditionCounts, error) {
	var c EditionCounts
	err := s.db.QueryRow(ctx, `
		SELECT cites, linked, cases, cases_not_pincite_only
		FROM moml_citations.edition_citation_counts WHERE bibliographicid = $1`, id).Scan(
		&c.Cites, &c.Linked, &c.Cases, &c.CasesDirect)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("edition counts: %w", err)
	}
	return &c, nil
}

// getEditionSubjects lists the edition's Gale subjects in catalogue order.
func (s *server) getEditionSubjects(ctx context.Context, id string) ([]string, error) {
	items, err := collect(ctx, s.db, `
		SELECT subject FROM moml.edition_subjects
		WHERE bibliographicid = $1 AND subject <> '' ORDER BY position`, []any{id},
		func(rows pgx.Rows) (string, error) {
			var v string
			return v, rows.Scan(&v)
		})
	if err != nil {
		return nil, fmt.Errorf("edition subjects: %w", err)
	}
	return items, nil
}

// locRow is one MARC subfield of a Library of Congress heading.
type locRow struct {
	Subfield string
	Value    string
}

// getEditionLoCHeadings lists the edition's LoC subject headings.
func (s *server) getEditionLoCHeadings(ctx context.Context, id string) ([]string, error) {
	rows, err := collect(ctx, s.db, `
		SELECT subfield, COALESCE(locsubject, '') FROM moml.edition_loc_subjects
		WHERE bibliographicid = $1 ORDER BY position`, []any{id},
		func(rows pgx.Rows) (locRow, error) {
			var r locRow
			return r, rows.Scan(&r.Subfield, &r.Value)
		})
	if err != nil {
		return nil, fmt.Errorf("edition LoC headings: %w", err)
	}
	return groupLoCHeadings(rows), nil
}

// groupLoCHeadings joins the subfields of each heading: subfield a begins a
// heading, and the subfields that follow it (topical, geographic, form and
// chronological subdivisions) are appended with a dash.
func groupLoCHeadings(rows []locRow) []string {
	var headings []string
	for _, r := range rows {
		v := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(r.Value), "."))
		if v == "" {
			continue
		}
		if r.Subfield == "a" || len(headings) == 0 {
			headings = append(headings, v)
			continue
		}
		headings[len(headings)-1] += " — " + v
	}
	return headings
}

// getEditionReporters lists the reporters the edition cites, most cited
// first, from edition_reporter_citations. Statute "reporters" are left out.
func (s *server) getEditionReporters(ctx context.Context, id string) ([]ReporterShare, error) {
	items, err := collect(ctx, s.db, `
		SELECT erc.reporter_standard, r.reporter_title, r.jurisdiction, erc.cites, erc.linked
		FROM moml_citations.edition_reporter_citations erc
		LEFT JOIN legalhist.reporters r ON r.reporter_standard = erc.reporter_standard
		WHERE erc.bibliographicid = $1 AND COALESCE(r.type, '') <> 'statute'
		ORDER BY erc.cites DESC, erc.reporter_standard`, []any{id},
		func(rows pgx.Rows) (ReporterShare, error) {
			var r ReporterShare
			return r, rows.Scan(&r.Standard, &r.Title, &r.Jurisdiction, &r.Cites, &r.Linked)
		})
	if err != nil {
		return nil, fmt.Errorf("edition reporters: %w", err)
	}
	return items, nil
}

// getEditionVolumes lists the edition's volumes in order, each with the pages
// on which citations were detected and their counts. The pages query is bound
// by the unique index on citations_unlinked (moml_treatise, moml_page, ...);
// the largest edition (seven volumes, 285K citations) takes about a second.
func (s *server) getEditionVolumes(ctx context.Context, id string) ([]EditionVolume, error) {
	slog.Debug("querying edition volumes and pages", "bibliographicid", id)
	vols, err := collect(ctx, s.db, `
		SELECT psmid, current_volume, display_title, edition_statement, imprint, year, total_pages, product_link
		FROM moml.volumes WHERE bibliographicid = $1
		ORDER BY current_volume, psmid`, []any{id},
		func(rows pgx.Rows) (EditionVolume, error) {
			var v EditionVolume
			var productLink *string
			err := rows.Scan(&v.PSMID, &v.Number, &v.Title, &v.EditionStatement, &v.Imprint, &v.Year, &v.TotalPages, &productLink)
			v.Gale = galeLinks(productLink, "")
			return v, err
		})
	if err != nil {
		return nil, fmt.Errorf("edition volumes: %w", err)
	}
	if len(vols) == 0 {
		return nil, nil
	}
	psmids := make([]string, len(vols))
	index := make(map[string]int, len(vols)) // psmid -> position, since appending may move the slice
	for i, v := range vols {
		psmids[i] = v.PSMID
		index[v.PSMID] = i
	}
	pages, err := collect(ctx, s.db, `
		SELECT cu.moml_treatise, cu.moml_page,
		       COALESCE(mp.sourcepage, ''), COALESCE(mp.type, ''),
		       (SELECT string_agg(pc.sectionheader, ' / ' ORDER BY pc.sectionheader_type)
		          FROM moml.page_content pc
		         WHERE pc.psmid = cu.moml_treatise AND pc.pageid = cu.moml_page AND pc.sectionheader <> ''),
		       count(*), count(*) FILTER (WHERE cl.status LIKE 'linked%')
		FROM moml_citations.citations_unlinked cu
		LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
		LEFT JOIN moml.page mp ON mp.psmid = cu.moml_treatise AND mp.pageid = cu.moml_page
		WHERE cu.moml_treatise = ANY($1::text[])
		GROUP BY cu.moml_treatise, cu.moml_page, mp.sourcepage, mp.type
		ORDER BY cu.moml_treatise, cu.moml_page`, []any{psmids},
		func(rows pgx.Rows) (EditionPageRow, error) {
			var p EditionPageRow
			return p, rows.Scan(&p.PSMID, &p.PageID, &p.SourcePage, &p.Type, &p.Section, &p.Cites, &p.Linked)
		})
	if err != nil {
		return nil, fmt.Errorf("edition pages: %w", err)
	}
	for _, p := range pages {
		i, ok := index[p.PSMID]
		if !ok {
			continue
		}
		vols[i].Pages = append(vols[i].Pages, p)
		vols[i].Cites += p.Cites
		vols[i].Linked += p.Linked
	}
	return vols, nil
}

// editionTopCases ranks the cases one edition cites.
func (s *server) editionTopCases(ctx context.Context, id string, excludePincite bool, limit int) (TopCasesView, error) {
	rows, err := s.getTopCasesForEditions(ctx, []string{id}, excludePincite, limit)
	for i := range rows {
		rows[i].PagesURL = citationsURL(url.Values{"edition": {id}, "case": {rows[i].Key()}})
	}
	return TopCasesView{Rows: rows, Empty: "No linked citations in this edition."}, err
}

func (s *server) handleEdition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	excludePincite := r.URL.Query().Get("pincite") == "exclude"

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	d, err := s.getEditionHeader(ctx, id)
	if err != nil {
		s.serverError(w, r, err, "loading the edition", "bibliographicid", id)
		return
	}
	if d == nil {
		s.notFound(w, r, "There is no edition with the bibliographic id "+id+".")
		return
	}
	d.ExcludePincite = excludePincite
	d.Page = Page{
		Title:   d.Title,
		Section: "works",
		Crumbs: []Crumb{
			{Label: "Treatises", URL: "/works"},
			{Label: truncate(d.WorkTitle, 50), URL: workURL(d.WorkID)},
			{Label: editionCrumb(d)},
		},
	}

	if d.Subjects, err = s.getEditionSubjects(ctx, id); err != nil {
		s.serverError(w, r, err, "loading the edition's subjects", "bibliographicid", id)
		return
	}
	if d.LoCHeadings, err = s.getEditionLoCHeadings(ctx, id); err != nil {
		s.serverError(w, r, err, "loading the edition's headings", "bibliographicid", id)
		return
	}

	counts, err := s.getEditionCounts(ctx, id)
	if err := optional(err, &d.Page, "moml_citations.edition_citation_counts"); err != nil {
		s.serverError(w, r, err, "loading the edition's counts", "bibliographicid", id)
		return
	}
	d.Counts = counts

	reporters, err := s.getEditionReporters(ctx, id)
	if err := optional(err, &d.Page, "moml_citations.edition_reporter_citations"); err != nil {
		s.serverError(w, r, err, "loading the edition's reporters", "bibliographicid", id)
		return
	}
	d.Reporters = reporters
	for _, rep := range reporters {
		switch rep.Side() {
		case "US":
			d.USCites += rep.Cites
		case "UK":
			d.UKCites += rep.Cites
		}
	}

	top, err := s.editionTopCases(ctx, id, excludePincite, 25)
	if err := optional(err, &d.Page, "moml_citations.edition_case_citations"); err != nil {
		s.serverError(w, r, err, "ranking the edition's cases", "bibliographicid", id)
		return
	}
	d.TopCases = top

	if d.Volumes, err = s.getEditionVolumes(ctx, id); err != nil {
		s.serverError(w, r, err, "listing the edition's pages", "bibliographicid", id)
		return
	}

	s.render(w, r, "edition.html", http.StatusOK, d)
}

// editionCrumb names an edition in a breadcrumb by its year and statement.
func editionCrumb(d *EditionDetail) string {
	parts := []string{}
	if d.EditionStatement != nil && *d.EditionStatement != "" {
		parts = append(parts, *d.EditionStatement)
	}
	if d.Year != nil {
		parts = append(parts, fmt.Sprint(*d.Year))
	}
	if len(parts) == 0 {
		return "Edition " + d.BiblioID
	}
	return strings.Join(parts, ", ")
}

// EditionCasesPage lists every case an edition cites.
type EditionCasesPage struct {
	Page
	Edition        *EditionDetail
	Cases          TopCasesView
	Nav            Pagination
	ExcludePincite bool
	Sort           string
}

const editionCasesPageSize = 100

var editionCaseSorts = map[string]string{
	"cites": "c.cite_count DESC, name",
	"year":  "year NULLS LAST, c.cite_count DESC",
	"name":  "name NULLS LAST, c.cite_count DESC",
}

func normalizeEditionCaseSort(sort string) string {
	if _, ok := editionCaseSorts[sort]; ok {
		return sort
	}
	return "cites"
}

// getEditionCases lists the cases one edition cites, a page at a time, with
// the total. Sorted by citations, the page is cut from edition_case_citations
// before the case tables are joined, so the largest edition (82K cases) pages
// in milliseconds; sorted by name or year the join has to come first, since
// those columns belong to the case tables.
func (s *server) getEditionCases(ctx context.Context, id, sort string, excludePincite bool, limit, offset int) ([]TopCase, int, error) {
	var total int
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM moml_citations.edition_case_citations
		WHERE bibliographicid = $1 AND (NOT $2::bool OR NOT pincite_only)`, id, excludePincite).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("edition cases: counting: %w", err)
	}
	sort = normalizeEditionCaseSort(sort)
	var query string
	if sort == "cites" {
		query = `
		WITH c AS (
		  SELECT * FROM moml_citations.edition_case_citations
		  WHERE bibliographicid = $1 AND (NOT $2::bool OR NOT pincite_only)
		  ORDER BY cite_count DESC, cap_case_id, er_case_id, code_reporter_id, stub_cite
		  LIMIT $3 OFFSET $4
		)
		SELECT c.cite_count, c.pincite_only, ` + caseMetaColumns("c") + `
		FROM c ` + caseMetaJoins("c") + `
		ORDER BY c.cite_count DESC, c.cap_case_id, c.er_case_id, c.code_reporter_id, c.stub_cite`
	} else {
		query = `
		SELECT c.cite_count, c.pincite_only, ` + caseMetaColumns("c") + `
		FROM moml_citations.edition_case_citations c ` + caseMetaJoins("c") + `
		WHERE c.bibliographicid = $1 AND (NOT $2::bool OR NOT c.pincite_only)
		ORDER BY ` + editionCaseSorts[sort] + `
		LIMIT $3 OFFSET $4`
		// The ORDER BY names the name and year columns of caseMetaColumns, which
		// the fragment leaves unaliased, so alias them here.
		query = strings.Replace(query, "COALESCE(cc.name_abbreviation, er.murrell_title, er.er_name, code.name, sm.party_names),",
			"COALESCE(cc.name_abbreviation, er.murrell_title, er.er_name, code.name, sm.party_names) AS name,", 1)
		query = strings.Replace(query, "COALESCE(cc.decision_year, er.murrell_year, er.er_year, code.decision_year, sm.year_decided),",
			"COALESCE(cc.decision_year, er.murrell_year, er.er_year, code.decision_year, sm.year_decided) AS year,", 1)
	}
	items, err := collect(ctx, s.db, query, []any{id, excludePincite, limit, offset}, func(rows pgx.Rows) (TopCase, error) {
		var t TopCase
		var source, cid, name, cite *string
		var year *int
		err := rows.Scan(&t.Cites, &t.PincitesOnly, &source, &cid, &name, &year, &cite)
		if ref := caseRefFrom(source, cid, name, year, cite); ref != nil {
			t.CaseRef = *ref
			t.PagesURL = citationsURL(url.Values{"edition": {id}, "case": {ref.Key()}})
		}
		return t, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("edition cases: %w", err)
	}
	return items, total, nil
}

func (s *server) handleEditionCases(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	excludePincite := r.URL.Query().Get("pincite") == "exclude"
	sort := normalizeEditionCaseSort(r.URL.Query().Get("sort"))
	page := parsePage(r.URL.Query().Get("page"))

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	ed, err := s.getEditionHeader(ctx, id)
	if err != nil {
		s.serverError(w, r, err, "loading the edition", "bibliographicid", id)
		return
	}
	if ed == nil {
		s.notFound(w, r, "There is no edition with the bibliographic id "+id+".")
		return
	}
	d := EditionCasesPage{Edition: ed, ExcludePincite: excludePincite, Sort: sort}
	d.Page = Page{
		Title:   "Cases cited by " + ed.Title,
		Section: "works",
		Crumbs: []Crumb{
			{Label: "Treatises", URL: "/works"},
			{Label: truncate(ed.WorkTitle, 50), URL: workURL(ed.WorkID)},
			{Label: editionCrumb(ed), URL: editionURL(id)},
			{Label: "Cases cited"},
		},
	}
	rows, total, err := s.getEditionCases(ctx, id, sort, excludePincite, editionCasesPageSize, (page-1)*editionCasesPageSize)
	if err := optional(err, &d.Page, "moml_citations.edition_case_citations"); err != nil {
		s.serverError(w, r, err, "listing the edition's cases", "bibliographicid", id)
		return
	}
	d.Cases = TopCasesView{Rows: rows, Empty: "No linked citations in this edition."}
	d.Nav = paginate(r, page, editionCasesPageSize, len(rows), total)
	s.render(w, r, "edition_cases.html", http.StatusOK, d)
}
