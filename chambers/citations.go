package main

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
)

// A citation is one string the detector found on a page
// (moml_citations.citations_unlinked) and what the linker made of it
// (citation_links). The citation list finds citations by an exact cite
// string, a reporter, an edition or a case -- each a shape of query an index
// serves -- and the detail page shows everything about one.

// CitationFilter is the citation list's query string.
type CitationFilter struct {
	ID       string
	Cite     string
	Reporter string
	Volume   string
	Page     string
	Year     string
	Status   string
	Tier     string
	Edition  string
	Case     string
}

func parseCitationFilter(q url.Values) CitationFilter {
	get := func(k string) string { return strings.TrimSpace(q.Get(k)) }
	return CitationFilter{
		ID: get("id"), Cite: get("cite"), Reporter: get("reporter"), Volume: get("volume"), Page: get("page"),
		Year: get("year"), Status: get("status"), Tier: get("tier"), Edition: get("edition"), Case: get("case"),
	}
}

// Shape names the indexed query the filter runs: "cite" (an exact normalized
// cite, which a reporter with a volume and page is turned into), "reporter" (a
// sample of the reporter's citations), "edition" (a treatise edition, and
// optionally one case within it), "case" (a case), or "" when nothing
// selective was asked for.
func (f CitationFilter) Shape() string {
	switch {
	case f.Cite != "":
		return "cite"
	case f.Reporter != "" && f.Page != "":
		return "cite"
	case f.Edition != "":
		return "edition"
	case f.Case != "":
		return "case"
	case f.Reporter != "":
		return "reporter"
	}
	return ""
}

// Values renders the filter back into a query string, for links that keep it.
func (f CitationFilter) Values() url.Values {
	v := url.Values{}
	for k, val := range map[string]string{
		"cite": f.Cite, "reporter": f.Reporter, "volume": f.Volume, "page": f.Page, "year": f.Year,
		"status": f.Status, "tier": f.Tier, "edition": f.Edition, "case": f.Case,
	} {
		if val != "" {
			v.Set(k, val)
		}
	}
	return v
}

// CitationRow is one citation on the list.
type CitationRow struct {
	ID             uuid.UUID
	Raw            string
	Chip           Chip
	CiteNormalized *string
	PSMID          string
	PageID         string
	SourcePage     string
	PageType       string
	BiblioID       *string
	Title          *string
	Year           *int
	Case           *CaseRef
}

func (c CitationRow) DetailURL() string { return citationURL(c.ID.String()) }
func (c CitationRow) PageURL() string   { return pageURL(c.PSMID, c.PageID) }
func (c CitationRow) EditionURL() string {
	if c.BiblioID == nil {
		return ""
	}
	return editionURL(*c.BiblioID)
}

// PageLabel is the printed page, or the image number.
func (c CitationRow) PageLabel() string {
	if c.SourcePage != "" {
		return "p. " + c.SourcePage
	}
	return "image " + strings.TrimLeft(strings.TrimRight(c.PageID, "0"), "0")
}

// citationsLimit is how many citations the list shows; one more is fetched
// to know whether it was cut short.
const citationsLimit = 500

// citationQuery is one shape of citation query, built by buildCitationQuery.
type citationQuery struct {
	where   []string
	args    []any
	ordered bool   // the result is ordered, so a truncation is "the first 500"
	note    string // shown above the list
}

func (q *citationQuery) arg(v any) string {
	q.args = append(q.args, v)
	return "$" + strconv.Itoa(len(q.args))
}

// buildCitationQuery turns a filter into predicates, looking up whatever the
// shape needs first: a reporter's spellings, an edition's volumes, or the
// normalized forms a reporter, volume and page take after the whitelist and
// diffvols translations. It returns nil when the filter is not selective.
func (s *server) buildCitationQuery(ctx context.Context, f CitationFilter) (*citationQuery, error) {
	q := &citationQuery{}
	switch f.Shape() {
	case "cite":
		cites := []string{f.Cite}
		if f.Cite == "" {
			var err error
			if cites, err = s.normalizedCites(ctx, f); err != nil {
				return nil, err
			}
			q.note = "Citations whose normalized form is " + strings.Join(cites, " or ") + "."
		}
		q.where = append(q.where, "cl.cite_normalized = ANY("+q.arg(cites)+"::text[])")
		q.ordered = true
	case "reporter":
		spellings, err := s.whitelistSpellings(ctx, f.Reporter)
		if err != nil {
			return nil, err
		}
		if len(spellings) == 0 {
			spellings = []string{f.Reporter}
		}
		q.where = append(q.where, "cu.reporter_abbr = ANY("+q.arg(spellings)+"::text[])")
		q.note = fmt.Sprintf("A sample of the citations whose spelling the whitelist maps to %s, in no particular order. Add a volume and page, or an edition, to narrow it.", f.Reporter)
	case "edition":
		psmids, err := s.editionVolumeIDs(ctx, f.Edition)
		if err != nil {
			return nil, err
		}
		q.where = append(q.where, "cu.moml_treatise = ANY("+q.arg(psmids)+"::text[])")
		q.ordered = true
		if f.Case != "" {
			if err := q.addCase(f.Case); err != nil {
				return nil, err
			}
		}
	case "case":
		if err := q.addCase(f.Case); err != nil {
			return nil, err
		}
		q.ordered = true
	default:
		return nil, nil
	}
	switch f.Status {
	case "":
	case statusUnprocessed:
		q.where = append(q.where, "cl.citation_id IS NULL")
	default:
		q.where = append(q.where, "cl.status = "+q.arg(f.Status))
	}
	if f.Tier != "" {
		q.where = append(q.where, "cl.match_tier = "+q.arg(f.Tier))
	}
	return q, nil
}

// addCase adds the predicate for a source:id case key on the typed column.
func (q *citationQuery) addCase(key string) error {
	source, id, ok := splitCaseKey(key)
	if !ok {
		return fmt.Errorf("%q is not a case key of the form source:id", key)
	}
	col, placeholder := caseIDColumn(source, len(q.args)+1)
	q.args = append(q.args, id)
	q.where = append(q.where, "cl."+col+" = "+placeholder)
	return nil
}

// whitelistSpellings lists the detected spellings the whitelist maps to a
// reporter standard.
func (s *server) whitelistSpellings(ctx context.Context, standard string) ([]string, error) {
	items, err := collect(ctx, s.db, `SELECT reporter_found FROM legalhist.whitelist WHERE reporter_standard = $1`,
		[]any{standard}, func(rows pgx.Rows) (string, error) {
			var v string
			return v, rows.Scan(&v)
		})
	if err != nil {
		return nil, fmt.Errorf("whitelist spellings: %w", err)
	}
	return items, nil
}

// editionVolumeIDs lists the psmids of an edition's volumes.
func (s *server) editionVolumeIDs(ctx context.Context, bibliographicid string) ([]string, error) {
	items, err := collect(ctx, s.db, `SELECT psmid FROM moml.volumes WHERE bibliographicid = $1 ORDER BY current_volume, psmid`,
		[]any{bibliographicid}, func(rows pgx.Rows) (string, error) {
			var v string
			return v, rows.Scan(&v)
		})
	if err != nil {
		return nil, fmt.Errorf("edition volumes: %w", err)
	}
	if len(items) == 0 {
		items = []string{bibliographicid}
	}
	return items, nil
}

// normalizedCites builds the normalized forms a reporter, volume and page
// take in citation_links.cite_normalized: the whitelist's standard spelling
// with the volume and page ("2 Mass. 420"; a single-volume reporter has no
// volume; a year-cited reporter carries its year, "[1905] 2 K.B. 1"), and,
// where reporters_diffvols renumbers the reporter into CAP, the translated
// form as well.
func (s *server) normalizedCites(ctx context.Context, f CitationFilter) ([]string, error) {
	var citedByYearFrom *int
	err := s.db.QueryRow(ctx, `SELECT cited_by_year_from FROM legalhist.reporters WHERE reporter_standard = $1`,
		f.Reporter).Scan(&citedByYearFrom)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("reporter: %w", err)
	}
	prefix := ""
	if y, err := strconv.Atoi(f.Year); err == nil && citedByYearFrom != nil && y >= *citedByYearFrom {
		prefix = "[" + f.Year + "] "
	}
	cites := []string{}
	vol, hasVol := strconv.Atoi(f.Volume)
	if hasVol == nil {
		cites = append(cites, fmt.Sprintf("%s%d %s %s", prefix, vol, f.Reporter, f.Page))
	} else {
		cites = append(cites, fmt.Sprintf("%s%s %s", prefix, f.Reporter, f.Page))
	}
	rows, err := collect(ctx, s.db, `
		SELECT cap_vol, cap_reporter FROM legalhist.reporters_diffvols
		WHERE reporter_standard = $1 AND (vol IS NULL OR vol = $2::int)`,
		[]any{f.Reporter, nullableInt(f.Volume)}, func(rows pgx.Rows) (string, error) {
			var capVol int
			var capReporter string
			err := rows.Scan(&capVol, &capReporter)
			return fmt.Sprintf("%s%d %s %s", prefix, capVol, capReporter, f.Page), err
		})
	if err != nil {
		return nil, fmt.Errorf("diffvols: %w", err)
	}
	return append(cites, rows...), nil
}

// nullableInt parses s as an int, or returns nil for SQL NULL.
func nullableInt(s string) *int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

// getCitations runs a built query and returns up to citationsLimit rows and
// whether there were more.
func (s *server) getCitations(ctx context.Context, q *citationQuery) ([]CitationRow, bool, error) {
	order := ""
	if q.ordered {
		order = "ORDER BY v.year NULLS LAST, v.current_volume, cu.moml_treatise, cu.moml_page, cu.id"
	}
	query := `
	SELECT cu.id, cu.raw, cl.status, cl.match_tier, cl.cite_normalized,
	       cu.moml_treatise, cu.moml_page, COALESCE(mp.sourcepage, ''), COALESCE(mp.type, ''),
	       v.bibliographicid, v.display_title, v.year,
	       ` + caseMetaColumns("cl") + `
	FROM moml_citations.citations_unlinked cu
	LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
	LEFT JOIN moml.volumes v ON v.psmid = cu.moml_treatise
	LEFT JOIN moml.page mp ON mp.psmid = cu.moml_treatise AND mp.pageid = cu.moml_page
	` + caseMetaJoins("cl") + `
	WHERE ` + strings.Join(q.where, " AND ") + `
	` + order + `
	LIMIT ` + strconv.Itoa(citationsLimit+1)
	slog.Debug("querying citations", "where", q.where)
	items, err := collect(ctx, s.db, query, q.args, func(rows pgx.Rows) (CitationRow, error) {
		var c CitationRow
		var status, tier, source, id, name, cite *string
		var year *int
		err := rows.Scan(&c.ID, &c.Raw, &status, &tier, &c.CiteNormalized,
			&c.PSMID, &c.PageID, &c.SourcePage, &c.PageType, &c.BiblioID, &c.Title, &c.Year,
			&source, &id, &name, &year, &cite)
		c.Raw = cleanRaw(c.Raw)
		c.Chip = chipFor(status, tier)
		c.Case = caseRefFrom(source, id, name, year, cite)
		return c, err
	})
	if err != nil {
		return nil, false, fmt.Errorf("citations: %w", err)
	}
	more := len(items) > citationsLimit
	if more {
		items = items[:citationsLimit]
	}
	return items, more, nil
}

func (s *server) handleCitations(w http.ResponseWriter, r *http.Request) {
	f := parseCitationFilter(r.URL.Query())
	if f.ID != "" {
		id, err := uuid.Parse(f.ID)
		if err != nil {
			s.badRequest(w, r, f.ID+" is not a citation id (a UUID).")
			return
		}
		http.Redirect(w, r, citationURL(id.String()), http.StatusFound)
		return
	}

	data := struct {
		Page
		Filter   CitationFilter
		Shape    string
		Note     string
		Items    []CitationRow
		More     bool
		Ordered  bool
		Statuses []string
		Tiers    []TierInfo
	}{Page: Page{Title: "Citations", Section: "citations"}, Filter: f, Shape: f.Shape(),
		Statuses: statusOrder, Tiers: tierVocabulary}

	if data.Shape == "" {
		s.render(w, r, "citations.html", http.StatusOK, data)
		return
	}

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	q, err := s.buildCitationQuery(ctx, f)
	if err != nil {
		s.badRequest(w, r, err.Error())
		return
	}
	data.Note = q.note
	data.Ordered = q.ordered
	data.Items, data.More, err = s.getCitations(ctx, q)
	if err != nil {
		s.serverError(w, r, err, "listing citations", "shape", data.Shape)
		return
	}
	s.render(w, r, "citations.html", http.StatusOK, data)
}

// CitationDetail is everything about one citation: what was detected, what
// the linker did with it, the case it reached, and the page it was found on.
type CitationDetail struct {
	Page
	ID           uuid.UUID
	Raw          string
	Volume       *int
	ReporterAbbr string
	ReporterStd  *string
	Junk         *bool
	PageNo       int
	Year         *int
	PSMID        string
	PageID       string

	Chip           Chip
	Status         *string
	CiteCleaned    *string
	CiteNormalized *string
	CiteLinked     *string
	Case           *CaseRef
	FullName       *string
	Court          *string
	Jurisdiction   *string
	ExternalURL    *string
	StubCite       *string // nil for a stub link whose stub was pruned
	StubPruned     bool
	VolYear        *int

	BiblioID    *string
	VolumeTitle *string
	VolumeYear  *int
	Author      *string
	WorkID      *int
	WorkTitle   *string
	Treatise    *string // the treatise view's jurisdiction, nil when outside it
	SourcePage  *string
	PageType    *string
	Gale        GaleLinks
	OCRText     *string
	Highlighted template.HTML
}

func (c *CitationDetail) PageURL() string { return pageURL(c.PSMID, c.PageID) }

func (c *CitationDetail) EditionURL() string {
	if c.BiblioID == nil {
		return ""
	}
	return editionURL(*c.BiblioID)
}

func (c *CitationDetail) WorkURL() string {
	if c.WorkID == nil {
		return ""
	}
	return workURL(*c.WorkID)
}
func (c *CitationDetail) ReporterURL() string {
	if c.ReporterStd == nil {
		return ""
	}
	return reporterURL(*c.ReporterStd)
}

// Parsed is the citation as the detector parsed it.
func (c *CitationDetail) Parsed() string {
	var b strings.Builder
	if c.Year != nil {
		fmt.Fprintf(&b, "[%d] ", *c.Year)
	}
	if c.Volume != nil {
		fmt.Fprintf(&b, "%d ", *c.Volume)
	}
	fmt.Fprintf(&b, "%s %d", c.ReporterAbbr, c.PageNo)
	return b.String()
}

const citationDetailQuery = `
SELECT cu.id, cu.raw, cu.volume, cu.reporter_abbr, cu.page, cu.year, cu.moml_treatise, cu.moml_page,
       wl.reporter_standard, wl.junk,
       cl.status, cl.match_tier, cl.cite_cleaned, cl.cite_normalized, cl.cite_linked, cl.stub_cite,
       ` + "CASE WHEN cl.cap_case_id IS NOT NULL THEN 'cap' WHEN cl.er_case_id IS NOT NULL THEN 'er' WHEN cl.code_reporter_id IS NOT NULL THEN 'code' WHEN cl.stub_cite IS NOT NULL THEN 'stub' END," + `
       COALESCE(cl.cap_case_id::text, cl.er_case_id, cl.code_reporter_id::text, cl.stub_cite),
       COALESCE(cc.name_abbreviation, er.murrell_title, er.er_name, code.name_abbreviation, sm.party_names),
       COALESCE(cc.decision_year, er.murrell_year, er.er_year, code.decision_year, sm.year_decided),
       COALESCE(capcite.cite, er.er_cite, code.official_citation, cl.stub_cite),
       COALESCE(cc.name, er.er_name, code.name),
       COALESCE(ct.name, er.court, code.court_name),
       COALESCE(j.name_long, code.jurisdiction, sm.jurisdiction),
       COALESCE(cc.frontend_url, er.er_url),
       st.cite, st.vol_year,
       v.bibliographicid, v.display_title, v.year, e.author, w.work_id, w.title,
       (SELECT t.jurisdiction FROM moml.treatises t WHERE t.bibliographicid = v.bibliographicid),
       v.product_link, NULLIF(mp.sourcepage, ''), mp.type, po.ocrtext
FROM moml_citations.citations_unlinked cu
LEFT JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr
LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
LEFT JOIN cap.cases cc ON cc.id = cl.cap_case_id
LEFT JOIN LATERAL (
  SELECT ci.cite FROM cap.citations ci WHERE ci."case" = cl.cap_case_id
  ORDER BY (ci.type = 'official') DESC, ci.cite LIMIT 1
) capcite ON true
LEFT JOIN cap.courts ct ON ct.id = cc.court
LEFT JOIN cap.jurisdictions j ON j.id = cc.jurisdiction
LEFT JOIN legalhist.code_reporter code ON code.id = cl.code_reporter_id
LEFT JOIN english_reports.cases er ON er.id = cl.er_case_id
LEFT JOIN legalhist.stub_cases st ON st.cite = cl.stub_cite
LEFT JOIN legalhist.stub_case_metadata sm ON sm.cite = cl.stub_cite
LEFT JOIN moml.volumes v ON v.psmid = cu.moml_treatise
LEFT JOIN moml.editions e ON e.bibliographicid = v.bibliographicid
LEFT JOIN moml.works w ON w.work_id = e.work_id
LEFT JOIN moml.page mp ON mp.psmid = cu.moml_treatise AND mp.pageid = cu.moml_page
LEFT JOIN moml.page_ocrtext po ON po.psmid = cu.moml_treatise AND po.pageid = cu.moml_page
WHERE cu.id = $1
LIMIT 1`

// getCitationDetail loads one citation. Returns (nil, nil) when there is none.
func (s *server) getCitationDetail(ctx context.Context, id uuid.UUID) (*CitationDetail, error) {
	slog.Debug("querying citation detail", "id", id)
	c := &CitationDetail{}
	var tier, source, caseID, name, cite *string
	var year *int
	var productLink *string
	err := s.db.QueryRow(ctx, citationDetailQuery, id).Scan(
		&c.ID, &c.Raw, &c.Volume, &c.ReporterAbbr, &c.PageNo, &c.Year, &c.PSMID, &c.PageID,
		&c.ReporterStd, &c.Junk,
		&c.Status, &tier, &c.CiteCleaned, &c.CiteNormalized, &c.CiteLinked, &c.StubCite,
		&source, &caseID, &name, &year, &cite,
		&c.FullName, &c.Court, &c.Jurisdiction, &c.ExternalURL,
		&c.StubCite, &c.VolYear,
		&c.BiblioID, &c.VolumeTitle, &c.VolumeYear, &c.Author, &c.WorkID, &c.WorkTitle, &c.Treatise,
		&productLink, &c.SourcePage, &c.PageType, &c.OCRText)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("citation detail: %w", err)
	}
	c.Chip = chipFor(c.Status, tier)
	c.Case = caseRefFrom(source, caseID, name, year, cite)
	c.StubPruned = c.Case != nil && c.Case.Source == "stub" && c.StubCite == nil
	c.Gale = galeLinks(productLink, c.PageID)
	if c.OCRText != nil {
		text := *c.OCRText
		escaped := template.HTMLEscapeString(text)
		raw := template.HTMLEscapeString(c.Raw)
		c.Highlighted = template.HTML(strings.Replace(escaped, raw, "<mark>"+raw+"</mark>", 1))
	}
	return c, nil
}

func (s *server) handleCitation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.badRequest(w, r, r.PathValue("id")+" is not a citation id (a UUID).")
		return
	}

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	c, err := s.getCitationDetail(ctx, id)
	if err != nil {
		s.serverError(w, r, err, "loading the citation", "id", id)
		return
	}
	if c == nil {
		s.notFound(w, r, "There is no citation with the id "+id.String()+".")
		return
	}
	c.Page = Page{
		Title:   "Citation " + c.Raw,
		Section: "citations",
		Crumbs:  []Crumb{{Label: "Citations", URL: "/citations"}, {Label: c.Raw}},
	}
	s.render(w, r, "citation.html", http.StatusOK, c)
}
