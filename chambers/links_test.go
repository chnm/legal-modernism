package main

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func strptr(s string) *string { return &s }

func TestGaleLinks(t *testing.T) {
	productLink := "http://link.galegroup.com/apps/doc/F0103227568/MOML?sid=dhxml"
	tests := []struct {
		name        string
		productLink *string
		pageid      string
		gmu         string
		columbia    string
	}{
		{
			name:        "with page",
			productLink: strptr(productLink),
			pageid:      "06870",
			gmu:         "https://link.gale.com/apps/doc/F0103227568/MOML?u=viva_gmu&sid=dhxml&pg=687",
			columbia:    "https://link.gale.com/apps/doc/F0103227568/MOML?u=columbiau&sid=dhxml&pg=687",
		},
		{
			name:        "no page",
			productLink: strptr(productLink),
			pageid:      "",
			gmu:         "https://link.gale.com/apps/doc/F0103227568/MOML?u=viva_gmu&sid=dhxml",
			columbia:    "https://link.gale.com/apps/doc/F0103227568/MOML?u=columbiau&sid=dhxml",
		},
		{
			name:        "nil product link",
			productLink: nil,
			pageid:      "06870",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := galeLinks(tt.productLink, tt.pageid)
			require.Equal(t, tt.gmu, g.GMU)
			require.Equal(t, tt.columbia, g.Columbia)
		})
	}
}

func TestURLs(t *testing.T) {
	require.Equal(t, "/works/103", workURL(103))
	require.Equal(t, "/editions/CTRG95-B2993", editionURL("CTRG95-B2993"))
	require.Equal(t, "/editions/CTRG95-B2993/cases", editionCasesURL("CTRG95-B2993"))
	require.Equal(t, "/pages/20000685307/04080", pageURL("20000685307", "04080"))
	require.Equal(t, "/cases/cap/6754004", caseURL("cap", "6754004"))
	require.Equal(t, "/cases/stub/%5B1905%5D%202%20K.B.%201", caseURL("stub", "[1905] 2 K.B. 1"))
	require.Equal(t, "/reporters/Ad%20&%20E", reporterURL("Ad & E"))
	require.Equal(t, "/reporters/L.%20R.%20Ch.", reporterURL("L. R. Ch."))
	require.Equal(t, "/citations", citationsURL(nil))
	require.Equal(t, "/citations?case=cap%3A1&edition=x", citationsURL(url.Values{"edition": {"x"}, "case": {"cap:1"}}))

	// A stub cite survives the round trip through a path segment.
	u, err := url.Parse(caseURL("stub", "[1905] 2 K.B. 1"))
	require.NoError(t, err)
	require.Equal(t, "/cases/stub/[1905] 2 K.B. 1", u.Path)
}

func TestCaseRef(t *testing.T) {
	name := "Marbury v. Madison"
	cite := "5 U.S. 137"
	c := CaseRef{Source: "cap", ID: "1", Name: &name, Cite: &cite}
	require.Equal(t, "cap:1", c.Key())
	require.Equal(t, "Marbury v. Madison", c.Display())
	require.Equal(t, "CAP", c.SourceLabel())
	require.Equal(t, "src-cap", c.BadgeClass())
	require.Equal(t, "5 U.S. 137", CaseRef{Source: "cap", ID: "1", Cite: &cite}.Display())
	require.Equal(t, "(unknown case)", CaseRef{Source: "er", ID: "x"}.Display())

	source, id, ok := splitCaseKey("stub:[1905] 2 K.B. 1")
	require.True(t, ok)
	require.Equal(t, "stub", source)
	require.Equal(t, "[1905] 2 K.B. 1", id)
	_, _, ok = splitCaseKey("nope:1")
	require.False(t, ok)
	_, _, ok = splitCaseKey("cap:")
	require.False(t, ok)

	col, ph := caseIDColumn("cap", 3)
	require.Equal(t, "cap_case_id", col)
	require.Equal(t, "$3::bigint", ph)
	col, ph = caseIDColumn("stub", 1)
	require.Equal(t, "stub_cite", col)
	require.Equal(t, "$1", ph)
}
