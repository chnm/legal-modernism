package main

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseTemplates parses every page under templates/ with the base layout
// and the partials, as the server does, so a template that does not parse
// fails here rather than at the first request. It also checks that the pages
// the routes render exist, and that no page still defines the removed nav
// block or shows SQL.
func TestParseTemplates(t *testing.T) {
	tmpls := parseTemplates()

	for _, page := range []string{
		"home.html", "error.html", "works.html", "work.html", "edition.html", "edition_cases.html",
		"page.html", "cases.html", "case.html", "reporters.html", "reporter.html",
		"citations.html", "citation.html", "linking.html", "linking_tiers.html", "linking_whitelist.html",
		"cap_graph_case.html", "cap_graph_reporter.html", "viz_jurisdictions.html",
	} {
		require.Contains(t, tmpls, page)
	}

	entries, err := fs.ReadDir(templateFS, "templates")
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		if name == "baseof.html" || strings.HasPrefix(name, "_") {
			continue
		}
		require.Contains(t, tmpls, name, "every page template is served")
		b, err := fs.ReadFile(templateFS, "templates/"+name)
		require.NoError(t, err)
		src := string(b)
		require.NotContains(t, src, `{{define "nav"}}`, "%s: the nav block was replaced by the navbar", name)
		require.NotContains(t, src, "Show SQL", "%s: the SQL blocks were removed", name)
		require.NotContains(t, src, "SQL queries", "%s: the SQL blocks were removed", name)
	}
}

// TestRenderPages executes the pages whose data needs no database with
// representative data, so a missing field or a wrong pipeline fails here.
func TestRenderPages(t *testing.T) {
	tmpls := parseTemplates()
	render := func(t *testing.T, page string, data any) string {
		t.Helper()
		var buf bytes.Buffer
		require.NoError(t, tmpls[page].ExecuteTemplate(&buf, "baseof", data))
		return buf.String()
	}

	t.Run("home", func(t *testing.T) {
		out := render(t, "home.html", struct{ Page }{Page{Title: "Chambers", Section: "home"}})
		require.Contains(t, out, `href="/works"`)
		require.Contains(t, out, `href="/linking/whitelist"`)
		require.Contains(t, out, `href="/linking/cap"`)
		require.Contains(t, out, `href="/viz/jurisdictions"`)
	})

	t.Run("viz jurisdictions", func(t *testing.T) {
		out := render(t, "viz_jurisdictions.html", vizPage{Page{Title: "Citations between jurisdictions", Section: "viz"}})
		require.Contains(t, out, `const API = "/api/viz/jurisdictions"`)
		require.Contains(t, out, `from "/static/viz/jurisdictions.js"`)
		require.Contains(t, out, `from "/static/viz/style.js"`)
		require.Contains(t, out, `class="viz-source"`)
		require.Contains(t, out, `class="viz-notes"`)
		require.Contains(t, out, `href="/#viz"`)
	})

	t.Run("linking dashboard, treatises", func(t *testing.T) {
		out := render(t, "linking.html", newLinkingPage(momlCorpus, "dashboard", "Linking dashboard", "Dashboard"))
		require.Contains(t, out, `fetch("/api/linking-dashboard")`)
		require.Contains(t, out, "REPORTER_PAGES =  true")
		require.Contains(t, out, "<code>cite-linker</code>")
		require.Contains(t, out, `<a href="/linking" class="btn btn-primary">MOML treatises</a>`)
		require.Contains(t, out, `id="names-container"`)
		require.Contains(t, out, "renderNames(d.Names)")
		require.NotContains(t, out, `id="graph"`)
	})

	t.Run("linking dashboard, CAP opinions", func(t *testing.T) {
		name, cite, jur := "State v. Buck", "186 Mo. 15", "Mo."
		year := 1905
		data := newLinkingPage(capCorpus, "dashboard", "Linking dashboard: CAP opinions", "Dashboard")
		data.Graph = &capGraph{
			Available:     true,
			Total:         newGraphRow(0, "All years", "", agreement{InBoth: 3884023, OursOnly: 1153832, TheirsOnly: 2376036}),
			Years:         []graphRow{newGraphRow(1905, "1905", "", agreement{InBoth: 10, OursOnly: 2, TheirsOnly: 5})},
			Reporters:     []graphRow{{Key: 7, Label: "Mo.", OursURL: capGraphReporterURL(7, "ours"), TheirsURL: capGraphReporterURL(7, "theirs")}},
			ReporterCount: 226,
			Jurisdictions: []graphRow{newGraphRow(3, "Mo.", "Missouri", agreement{InBoth: 1})},
			Cases:         &caseDistribution{Cases: 1058387, InBoth: [4]int{2, 9, 40, 300}},
			TopTheirs:     []graphCase{{CaseRef: CaseRef{Source: "cap", ID: "1343497", Name: &name, Year: &year, Cite: &cite}, Jurisdiction: &jur, agreement: agreement{TheirsOnly: 9}}},
			Spellings:     []UnwhitelistedReporter{{ReporterAbbr: "Mo. l. c.", Count: 14080, CitingCases: 9000}},
			SpellingsURL:  capCorpus.WhitelistURL(),
		}
		out := render(t, "linking.html", data)
		require.Contains(t, out, `fetch("/api/linking-dashboard?corpus=cap")`)
		require.Contains(t, out, "REPORTER_PAGES =  false")
		require.Contains(t, out, "<code>cite-linker-cap</code>")
		require.Contains(t, out, `<a href="/linking/cap" class="btn btn-primary">CAP opinions</a>`)
		require.Contains(t, out, `id="graph"`)
		require.Contains(t, out, "3,884,023")
		require.Contains(t, out, "1,058,387 cases")
		require.Contains(t, out, "62%")
		require.Contains(t, out, `href="/linking/cap/cases/1343497"`)
		require.Contains(t, out, `href="/linking/cap/reporters/7?side=theirs"`)
		require.Contains(t, out, "<code>Mo. l. c.</code>")
		require.Contains(t, out, `href="/linking/whitelist?corpus=cap"`)
		require.Contains(t, out, `"oursURL":"/linking/cap/reporters/7?side=ours"`)
	})

	t.Run("CAP case against CAP's graph", func(t *testing.T) {
		name, cite, raw := "State v. Buck", "186 Mo. 15", "186 Mo. l. c. 118"
		year, other := 1905, 1853
		inScope := false
		chunn := "State v. Chunn"
		p := &capCasePage{
			Page:       Page{Title: "State v. Buck: against CAP's graph", Section: "linking"},
			Case:       &CaseDetail{CaseRef: CaseRef{Source: "cap", ID: "1343497", Name: &name, Year: &year, Cite: &cite}, Cites: []string{cite}},
			InMetadata: true, OpinionsN: 1, InScope: &inScope,
			Counts: agreement{TheirsOnly: 1},
			TheirsOnly: []capEdge{{
				Cited: CaseRef{Source: "cap", ID: "315565", Name: &chunn, Year: &other}, Cites: []string{"19 Mo. 233"}, InTheirs: true,
				Explanation: explanation{Kind: explainProbable, Citations: []capCitation{{ID: "x", Opinion: "majority", Raw: raw, Chip: Chip{Status: "skipped_not_whitelisted"}}}},
			}},
			Unexplained: []capCitation{{ID: "y", Opinion: "majority", Raw: "2 Ed.), 805", Chip: Chip{Status: "skipped_not_whitelisted"}}},
		}
		out := render(t, "cap_graph_case.html", p)
		require.Contains(t, out, "Outside the comparison")
		require.Contains(t, out, `href="/cases/cap/315565"`)
		require.Contains(t, out, "Probably detected under another spelling")
		require.Contains(t, out, "<code>186 Mo. l. c. 118</code>")
		require.Contains(t, out, "2 Ed.), 805")
		require.Contains(t, out, `href="/cases/cap/1343497"`)
	})

	t.Run("CAP reporter against CAP's graph", func(t *testing.T) {
		name := "State v. Buck"
		p := &capReporterPage{
			Page: Page{Title: "Mo.: edges only we have", Section: "linking"}, ID: 7, ShortName: "Mo.", Side: "ours",
			Totals:   &agreement{InBoth: 4, OursOnly: 2, TheirsOnly: 1},
			TopCases: []capReporterCase{{CaseRef: CaseRef{Source: "cap", ID: "1343458", Name: &name}, Edges: 12}},
			Edges: []capReporterEdge{{Citing: CaseRef{Source: "cap", ID: "1343497", Name: &name}, Cited: CaseRef{Source: "cap", ID: "1343458"},
				Raws: []string{"186 Mo. 25"}}},
			OursURL: capGraphReporterURL(7, "ours"), TheirsURL: capGraphReporterURL(7, "theirs"),
		}
		out := render(t, "cap_graph_reporter.html", p)
		require.Contains(t, out, "Edges only we have")
		require.Contains(t, out, `href="/linking/cap/cases/1343497"`)
		require.Contains(t, out, "<code class=\"me-2\">186 Mo. 25</code>")
		require.Contains(t, out, `<a href="/linking/cap/reporters/7?side=ours" class="btn btn-primary">Only ours</a>`)
	})

	t.Run("error", func(t *testing.T) {
		out := render(t, "error.html", errorPage{Page: Page{Title: "Not found"}, Status: 404, Message: "No such work."})
		require.Contains(t, out, "No such work.")
		require.Contains(t, out, "HTTP 404")
	})

	t.Run("citations form", func(t *testing.T) {
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
			Names    []string
		}{Page: Page{Title: "Citations", Section: "citations"}, Statuses: statusOrder, Tiers: tierVocabulary, Names: namesOptions}
		out := render(t, "citations.html", data)
		require.Contains(t, out, `name="cite"`)
		require.Contains(t, out, `<option value="cap_direct"`)
		require.NotContains(t, out, `<option value="no_match" selected`)
		require.Contains(t, out, `name="name"`)
		require.Contains(t, out, `<option value="disagree"`)

		no := false
		name := "Marbury v. Madison"
		data.Shape = "case"
		data.Items = []CitationRow{
			{Raw: "1 Cranch 137", Chip: Chip{Status: "linked_cap", Tier: "cap_direct"}, PSMID: "p", PageID: "00010",
				Case: &CaseRef{Source: "cap", ID: "1", Name: &name}, Name: &DetectedName{Name: "Marbury v. Madison", Source: "parallel", Agrees: &no}},
			{Raw: "2 Mass. 420", PSMID: "p", PageID: "00011"},
		}
		out = render(t, "citations.html", data)
		require.Contains(t, out, `<span class="cname name-disagree" title="does not name the linked case; inherited from the preceding parallel cite`)
		require.Contains(t, out, `<span class="cname-src">parallel</span>`)
	})

	t.Run("page with names", func(t *testing.T) {
		yes := true
		v := &PageView{Page: Page{Title: "P", Section: "works"}, PSMID: "p", PageID: "00010", VolumeTitle: "T",
			Text: "Smith v. Jones, 2 Mass. 420"}
		v.Citations = locateCitations(v.Text, []PageCitation{
			{orig: "2 Mass. 420", Chip: Chip{Status: "linked_cap", Tier: "cap_direct"}, Name: &DetectedName{Name: "Smith v. Jones", Source: "direct", Agrees: &yes}},
		})
		v.Highlighted = highlightSpans(v.Text, v.Citations)
		out := render(t, "page.html", v)
		require.Contains(t, out, "1 with a case name")
		require.Contains(t, out, `<mark class="name" id="name-1">Smith v. Jones</mark>`)
		require.Contains(t, out, `href="#name-1"`)
		require.Contains(t, out, `<span class="cname name-agree"`)
		require.Contains(t, out, "names the linked case")
	})

	t.Run("case page named as", func(t *testing.T) {
		yes, no := true, false
		name := "Young v. Gibson"
		d := &CaseDetail{Page: Page{Title: "C", Section: "cases"}, CaseRef: CaseRef{Source: "cap", ID: "1", Name: &name},
			CitedAsTotal: 10, NamedTotal: 8, NamedAgree: 6, Unnamed: 2,
			NamedAs: []NamedAs{
				{Name: DetectedName{Name: "Young v. Gibson", Source: "direct", Agrees: &yes}, N: 6},
				{Name: DetectedName{Name: "Stivers v. Cherryvale", Source: "direct", Agrees: &no}, N: 2},
			}}
		out := render(t, "case.html", d)
		require.Contains(t, out, "<strong>75%</strong>")
		require.Contains(t, out, "2 have none")
		require.Contains(t, out, `<span class="cname name-disagree">Stivers v. Cherryvale</span>`)
		require.Contains(t, out, `href="/citations?case=cap%3A1&amp;names=disagree"`)
	})

	t.Run("case page opinion citations", func(t *testing.T) {
		name, citing, cited := "Young v. Gibson", "Smith v. Jones", "Doe v. Roe"
		year := 1890
		d := &CaseDetail{Page: Page{Title: "C", Section: "cases"}, CaseRef: CaseRef{Source: "cap", ID: "1", Name: &name},
			CitedByOpinions: OpinionCases{Total: 3, TotalCites: 7, Cases: []OpinionCase{
				{CaseRef: CaseRef{Source: "cap", ID: "22", Name: &citing, Year: &year}, Cites: 4, Opinions: 2},
			}},
			OpinionsCite: OpinionCases{Total: 1, TotalCites: 2, Cases: []OpinionCase{
				{CaseRef: CaseRef{Source: "er", ID: "er-9", Name: &cited}, Cites: 2, Opinions: 1},
			}}}
		out := render(t, "case.html", d)
		require.Contains(t, out, "Cited by CAP opinions")
		require.Contains(t, out, "3 cases make 7 citations; the 1 that make the most are shown")
		require.Contains(t, out, `href="/cases/cap/22"`)
		require.Contains(t, out, "Cited in its opinions")
		require.Contains(t, out, `href="/cases/er/er-9"`)

		// Another source's case has no opinions of its own.
		d.Source = "er"
		out = render(t, "case.html", d)
		require.Contains(t, out, "Cited by CAP opinions")
		require.NotContains(t, out, "Cited in its opinions")
	})

	t.Run("citation detail with name", func(t *testing.T) {
		yes := true
		name := "Smith v. Jones"
		c := &CitationDetail{Page: Page{Title: "C", Section: "citations"}, Raw: "2 Mass. 420", ReporterAbbr: "Mass.", PageNo: 420,
			PSMID: "p", PageID: "00010", Chip: Chip{Status: "linked_cap", Tier: "cap_direct"},
			Case: &CaseRef{Source: "cap", ID: "1", Name: &name},
			Name: &DetectedName{Name: "Smith v. Jones", Source: "direct", Agrees: &yes}}
		text := "Smith v. Jones, 2 Mass. 420"
		c.OCRText = &text
		c.Highlighted, c.NameFound = highlightCitation(text, c.Raw, c.Name)
		out := render(t, "citation.html", c)
		require.Contains(t, out, "it <strong>names the linked case</strong>")
		require.Contains(t, out, `<mark class="name">Smith v. Jones</mark>, <mark>2 Mass. 420</mark>`)
		require.Contains(t, out, "the case name read before it")
	})

	t.Run("notices and crumbs", func(t *testing.T) {
		page := Page{Title: "T", Section: "works", Crumbs: []Crumb{{Label: "Treatises", URL: "/works"}, {Label: "Here"}}}
		page.unavailable("moml_citations.work_citation_counts")
		out := render(t, "error.html", errorPage{Page: page, Status: 500, Message: "m"})
		require.Contains(t, out, "make db-maintenance")
		require.Contains(t, out, `<a href="/works">Treatises</a>`)
		require.Contains(t, out, `class="nav-link active" href="/works"`)
	})

	t.Run("top cases partial", func(t *testing.T) {
		name := "Bradberry v. Hooks"
		cite := "1 N.C. 1"
		year := 1816
		data := WorkDetail{
			Page: Page{Title: "W", Section: "works"}, WorkID: 103, Title: "Commentaries",
			TopCases: TopCasesView{
				Rows:          []TopCase{{CaseRef: CaseRef{Source: "cap", ID: "6754004", Name: &name, Year: &year, Cite: &cite}, Editions: 12, Cites: 40}},
				ShowEditions:  true,
				TotalEditions: 119,
			},
		}
		out := render(t, "work.html", data)
		require.Contains(t, out, `href="/cases/cap/6754004"`)
		require.Contains(t, out, "of 119")
	})
}
