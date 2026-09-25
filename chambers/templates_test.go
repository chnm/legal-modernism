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
		}{Page: Page{Title: "Citations", Section: "citations"}, Statuses: statusOrder, Tiers: tierVocabulary}
		out := render(t, "citations.html", data)
		require.Contains(t, out, `name="cite"`)
		require.Contains(t, out, `<option value="cap_direct"`)
		require.NotContains(t, out, `<option value="no_match" selected`)
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
				Rows:          []TopCase{{CaseRef: CaseRef{Source: "cap", ID: "6754004", Name: &name, Year: &year, Cite: &cite}, Editions: 12, Cites: 40, PincitesOnly: true}},
				ShowEditions:  true,
				TotalEditions: 119,
			},
		}
		out := render(t, "work.html", data)
		require.Contains(t, out, `href="/cases/cap/6754004"`)
		require.Contains(t, out, "of 119")
		require.Contains(t, out, `title="Every citation was a pin cite`)
	})
}
