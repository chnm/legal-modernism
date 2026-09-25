package main

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/lmullen/legal-modernism/go/sources"
)

// A page is one scanned page of a volume, identified by the volume's psmid and
// the page's pageid (a zero-padded image number; pageid values repeat across
// volumes, so both are always needed). The page view shows the text the
// detector read, the citations it found there in the order they appear, and
// what each one linked to.

// PageView is the page page.
type PageView struct {
	Page
	PSMID        string
	PageID       string
	SourcePage   string // printed page label, "" when the scan has none
	Type         string // moml.page.type
	VolumeTitle  string
	VolumeNumber int
	VolumeCount  int
	Year         *int
	BiblioID     string
	WorkID       int
	WorkTitle    string
	Sections     []string
	Gale         GaleLinks
	Text         string
	Original     bool // the raw OCR text is shown, without corrections or highlights
	Prev         string
	Next         string
	PrevCited    string
	NextCited    string
	Citations    []PageCitation
	Highlighted  template.HTML
}

func (v *PageView) URL() string         { return pageURL(v.PSMID, v.PageID) }
func (v *PageView) EditionURL() string  { return editionURL(v.BiblioID) }
func (v *PageView) WorkURL() string     { return workURL(v.WorkID) }
func (v *PageView) OriginalURL() string { return v.URL() + "?text=original" }

// Label is the printed page, or the image number when there is none.
func (v *PageView) Label() string {
	if v.SourcePage != "" {
		return "p. " + v.SourcePage
	}
	return "image " + strings.TrimLeft(strings.TrimRight(v.PageID, "0"), "0")
}

// LinkedCount is how many of the page's citations resolved to a case.
func (v *PageView) LinkedCount() int {
	n := 0
	for _, c := range v.Citations {
		if c.Chip.Linked() {
			n++
		}
	}
	return n
}

// PageCitation is one citation detected on the page.
type PageCitation struct {
	ID               uuid.UUID
	Index            int // 1-based position in text order, the id of its mark
	Raw              string
	ReporterAbbr     string
	ReporterStandard *string
	Chip             Chip
	CiteLinked       *string
	Case             *CaseRef
	Found            bool

	orig string // raw as stored, for locating it in the text
	pos  int    // byte offset in the text; unfound citations sort after every found one
	end  int
}

func (c PageCitation) DetailURL() string { return citationURL(c.ID.String()) }

func (c PageCitation) ReporterURL() string {
	if c.ReporterStandard == nil {
		return ""
	}
	return reporterURL(*c.ReporterStandard)
}

// getPageHeader loads the volume, edition and work a page belongs to and the
// page's own catalogue row. Returns (nil, nil) when there is no such volume.
func (s *server) getPageHeader(ctx context.Context, psmid, pageid string) (*PageView, error) {
	slog.Debug("querying page", "psmid", psmid, "pageid", pageid)
	v := &PageView{PSMID: psmid, PageID: pageid}
	var productLink *string
	var sourcePage, pageType *string
	err := s.db.QueryRow(ctx, `
		SELECT v.bibliographicid, v.display_title, v.current_volume, v.year, v.product_link,
		       e.work_id, w.title,
		       (SELECT count(*) FROM moml.volumes v2 WHERE v2.bibliographicid = v.bibliographicid),
		       mp.sourcepage, mp.type
		FROM moml.volumes v
		JOIN moml.editions e ON e.bibliographicid = v.bibliographicid
		JOIN moml.works w ON w.work_id = e.work_id
		LEFT JOIN moml.page mp ON mp.psmid = v.psmid AND mp.pageid = $2
		WHERE v.psmid = $1`, psmid, pageid).Scan(
		&v.BiblioID, &v.VolumeTitle, &v.VolumeNumber, &v.Year, &productLink,
		&v.WorkID, &v.WorkTitle, &v.VolumeCount, &sourcePage, &pageType)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("page header: %w", err)
	}
	v.SourcePage = derefStr(sourcePage)
	v.Type = derefStr(pageType)
	v.Gale = galeLinks(productLink, pageid)
	return v, nil
}

// getPageSections lists the section headers recorded on the page.
func (s *server) getPageSections(ctx context.Context, psmid, pageid string) ([]string, error) {
	items, err := collect(ctx, s.db, `
		SELECT COALESCE(sectionheader_type, '') || ': ' || sectionheader
		FROM moml.page_content
		WHERE psmid = $1 AND pageid = $2 AND sectionheader <> ''
		ORDER BY sectionheader_type`, []any{psmid, pageid},
		func(rows pgx.Rows) (string, error) {
			var v string
			return v, rows.Scan(&v)
		})
	if err != nil {
		return nil, fmt.Errorf("page sections: %w", err)
	}
	return items, nil
}

// getPageNeighbours finds the previous and next page of the volume, and the
// previous and next page on which a citation was detected. Each lookup is a
// bounded index scan.
func (s *server) getPageNeighbours(ctx context.Context, v *PageView) error {
	var prev, next, prevCited, nextCited *string
	err := s.db.QueryRow(ctx, `
		SELECT (SELECT pageid FROM moml.page WHERE psmid = $1 AND pageid < $2 ORDER BY pageid DESC LIMIT 1),
		       (SELECT pageid FROM moml.page WHERE psmid = $1 AND pageid > $2 ORDER BY pageid LIMIT 1),
		       (SELECT max(moml_page) FROM moml_citations.citations_unlinked WHERE moml_treatise = $1 AND moml_page < $2),
		       (SELECT min(moml_page) FROM moml_citations.citations_unlinked WHERE moml_treatise = $1 AND moml_page > $2)`,
		v.PSMID, v.PageID).Scan(&prev, &next, &prevCited, &nextCited)
	if err != nil {
		return fmt.Errorf("page neighbours: %w", err)
	}
	link := func(p *string) string {
		if p == nil {
			return ""
		}
		return pageURL(v.PSMID, *p)
	}
	v.Prev, v.Next, v.PrevCited, v.NextCited = link(prev), link(next), link(prevCited), link(nextCited)
	return nil
}

// getPageText returns the page's OCR text, or "" when the page has none.
func (s *server) getPageText(ctx context.Context, psmid, pageid string) (string, error) {
	var text *string
	err := s.db.QueryRow(ctx, `SELECT ocrtext FROM moml.page_ocrtext WHERE psmid = $1 AND pageid = $2`,
		psmid, pageid).Scan(&text)
	if isNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("page text: %w", err)
	}
	return derefStr(text), nil
}

// getPageCitations lists the citations detected on a page in stored order,
// with their links and the cases they resolved to.
func (s *server) getPageCitations(ctx context.Context, psmid, pageid string) ([]PageCitation, error) {
	query := `
	SELECT cu.id, cu.raw, cu.reporter_abbr, wl.reporter_standard, cl.status, cl.match_tier, cl.cite_linked,
	       ` + caseMetaColumns("cl") + `
	FROM moml_citations.citations_unlinked cu
	LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
	LEFT JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr
	` + caseMetaJoins("cl") + `
	WHERE cu.moml_treatise = $1 AND cu.moml_page = $2
	ORDER BY cu.created_at, cu.id`
	items, err := collect(ctx, s.db, query, []any{psmid, pageid}, func(rows pgx.Rows) (PageCitation, error) {
		var c PageCitation
		var status, tier, source, id, name, cite *string
		var year *int
		err := rows.Scan(&c.ID, &c.orig, &c.ReporterAbbr, &c.ReporterStandard, &status, &tier, &c.CiteLinked,
			&source, &id, &name, &year, &cite)
		c.Raw = cleanRaw(c.orig)
		c.Chip = chipFor(status, tier)
		c.Case = caseRefFrom(source, id, name, year, cite)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("page citations: %w", err)
	}
	return items, nil
}

// detectorText reproduces the text the detector read: the OCR text with the
// corrections from legalhist.ocr_corrections applied, then the Law Reports
// series prefix moved behind the volume, in that order, as in
// cite-detector-moml. A citation's raw string was cut from this text, so it is
// where the raw strings are found.
func detectorText(rep *sources.OCRReplacer, text string) string {
	return citations.NormalizeSeriesPrefix(rep.Replace(text))
}

// locateCitations finds where each citation's raw string occurs in the text
// and sorts the citations into that order. Identical raw strings claim
// successive occurrences, so a cite repeated on a page is marked twice. A raw
// string that does not occur verbatim (OCR drift between the stored text and
// the detector's input) is looked for with its whitespace relaxed; one still
// not found keeps its stored order after every located citation.
func locateCitations(text string, cites []PageCitation) []PageCitation {
	claimed := make(map[string]int) // raw string -> offset after its last claimed occurrence
	for i := range cites {
		c := &cites[i]
		from := claimed[c.orig]
		start, end := findCitation(text, c.orig, from)
		if start < 0 && from > 0 {
			start, end = findCitation(text, c.orig, 0)
		}
		if start < 0 {
			c.pos = len(text) + i
			continue
		}
		c.Found = true
		c.pos, c.end = start, end
		claimed[c.orig] = end
	}
	sort.SliceStable(cites, func(i, j int) bool { return cites[i].pos < cites[j].pos })
	for i := range cites {
		cites[i].Index = i + 1
	}
	return cites
}

// findCitation returns the byte span of raw in text at or after from, first
// verbatim, then with any run of whitespace matching any other; (-1, -1) when
// it is absent.
func findCitation(text, raw string, from int) (int, int) {
	if raw == "" || from > len(text) {
		return -1, -1
	}
	if i := strings.Index(text[from:], raw); i >= 0 {
		return from + i, from + i + len(raw)
	}
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return -1, -1
	}
	quoted := make([]string, len(fields))
	for i, f := range fields {
		quoted[i] = regexp.QuoteMeta(f)
	}
	re, err := regexp.Compile(strings.Join(quoted, `\s+`))
	if err != nil {
		return -1, -1
	}
	loc := re.FindStringIndex(text[from:])
	if loc == nil {
		return -1, -1
	}
	return from + loc[0], from + loc[1]
}

// highlightSpans renders the text with each located citation wrapped in a
// mark whose id is the citation's index, so the citation list can link to it.
// A span inside an earlier, longer span is dropped rather than nested.
func highlightSpans(text string, cites []PageCitation) template.HTML {
	found := make([]PageCitation, 0, len(cites))
	for _, c := range cites {
		if c.Found {
			found = append(found, c)
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].pos != found[j].pos {
			return found[i].pos < found[j].pos
		}
		return found[i].end > found[j].end
	})
	var b strings.Builder
	at := 0
	for _, c := range found {
		if c.pos < at {
			continue
		}
		b.WriteString(template.HTMLEscapeString(text[at:c.pos]))
		fmt.Fprintf(&b, `<mark id="cite-%d">%s</mark>`, c.Index, template.HTMLEscapeString(text[c.pos:c.end]))
		at = c.end
	}
	b.WriteString(template.HTMLEscapeString(text[at:]))
	return template.HTML(b.String())
}

func (s *server) handlePage(w http.ResponseWriter, r *http.Request) {
	psmid, pageid := r.PathValue("psmid"), r.PathValue("pageid")
	original := r.URL.Query().Get("text") == "original"

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	v, err := s.getPageHeader(ctx, psmid, pageid)
	if err != nil {
		s.serverError(w, r, err, "loading the page", "psmid", psmid, "pageid", pageid)
		return
	}
	if v == nil {
		s.notFound(w, r, "There is no volume with the id "+psmid+".")
		return
	}
	v.Original = original
	v.Page = Page{
		Title:   v.VolumeTitle + ", " + v.Label(),
		Section: "works",
		Crumbs: []Crumb{
			{Label: "Treatises", URL: "/works"},
			{Label: truncate(v.WorkTitle, 40), URL: workURL(v.WorkID)},
			{Label: truncate(v.VolumeTitle, 40), URL: editionURL(v.BiblioID)},
		},
	}
	if v.VolumeCount > 1 {
		v.Page.Crumbs = append(v.Page.Crumbs, Crumb{Label: fmt.Sprintf("Volume %d", v.VolumeNumber), URL: editionURL(v.BiblioID) + "#" + psmid})
	}
	v.Page.Crumbs = append(v.Page.Crumbs, Crumb{Label: v.Label()})

	if v.Sections, err = s.getPageSections(ctx, psmid, pageid); err != nil {
		s.serverError(w, r, err, "loading the page's sections", "psmid", psmid, "pageid", pageid)
		return
	}
	if err = s.getPageNeighbours(ctx, v); err != nil {
		s.serverError(w, r, err, "finding the neighbouring pages", "psmid", psmid, "pageid", pageid)
		return
	}
	text, err := s.getPageText(ctx, psmid, pageid)
	if err != nil {
		s.serverError(w, r, err, "loading the page's text", "psmid", psmid, "pageid", pageid)
		return
	}
	cites, err := s.getPageCitations(ctx, psmid, pageid)
	if err != nil {
		s.serverError(w, r, err, "loading the page's citations", "psmid", psmid, "pageid", pageid)
		return
	}
	if text == "" && len(cites) == 0 && v.Type == "" {
		s.notFound(w, r, fmt.Sprintf("Volume %s has no page %s.", psmid, pageid))
		return
	}

	if original {
		v.Text = text
		v.Citations = locateCitations(text, cites)
		v.Highlighted = template.HTML(template.HTMLEscapeString(text))
	} else {
		v.Text = detectorText(s.ocrReplacer(ctx), text)
		v.Citations = locateCitations(v.Text, cites)
		v.Highlighted = highlightSpans(v.Text, v.Citations)
	}
	slog.Debug("rendering page", "psmid", psmid, "pageid", pageid, "citations", len(cites))
	s.render(w, r, "page.html", http.StatusOK, v)
}
