package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRoutes exercises the routes that need no database: the home page, the
// static files, the 404, the method check, and every legacy redirect.
func TestRoutes(t *testing.T) {
	srv := httptest.NewServer(newServer(nil, parseTemplates()).routes())
	defer srv.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	get := func(t *testing.T, path string) *http.Response {
		t.Helper()
		resp, err := client.Get(srv.URL + path)
		require.NoError(t, err)
		resp.Body.Close()
		return resp
	}

	require.Equal(t, http.StatusOK, get(t, "/").StatusCode)
	require.Equal(t, http.StatusOK, get(t, "/static/portrait.jpg").StatusCode)
	require.Equal(t, http.StatusOK, get(t, "/static/viz/style.js").StatusCode)
	require.Equal(t, http.StatusOK, get(t, "/static/viz/jurisdictions.js").StatusCode)
	require.Equal(t, http.StatusOK, get(t, "/static/viz/census.js").StatusCode)
	require.Equal(t, http.StatusOK, get(t, "/static/viz/field-code.js").StatusCode)

	// Static files carry an ETag that a conditional request matches, and ask
	// to be revalidated.
	static := get(t, "/static/viz/style.js")
	require.Equal(t, http.StatusOK, static.StatusCode)
	require.NotEmpty(t, static.Header.Get("ETag"))
	require.Equal(t, "no-cache", static.Header.Get("Cache-Control"))
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/static/viz/style.js", nil)
	require.NoError(t, err)
	req.Header.Set("If-None-Match", static.Header.Get("ETag"))
	cached, err := client.Do(req)
	require.NoError(t, err)
	cached.Body.Close()
	require.Equal(t, http.StatusNotModified, cached.StatusCode)
	require.Len(t, assetVersion, 16)
	require.Equal(t, http.StatusNotFound, get(t, "/nope").StatusCode)
	require.Equal(t, http.StatusGone, get(t, "/api/page-text?psmid=1&pageid=2").StatusCode)

	// The linking pages that only render a shell, for each corpus, and the
	// requests refused before any query.
	for _, path := range []string{"/linking", "/linking/tiers", "/linking/cap/tiers", "/linking/whitelist", "/linking/whitelist?corpus=cap", "/viz/jurisdictions", "/viz/field-code"} {
		require.Equal(t, http.StatusOK, get(t, path).StatusCode, path)
	}
	for _, path := range []string{
		"/linking/whitelist?corpus=x", "/api/linking-dashboard?corpus=x", "/api/tiers?corpus=x", "/api/whitelist-extender?corpus=x",
		"/linking/cap/cases/abc", "/linking/cap/cases/-4", "/linking/cap/cases?id=abc",
		"/linking/cap/reporters/x", "/linking/cap/reporters/3?side=both",
		"/api/viz/jurisdictions/N.Y.", "/api/viz/jurisdictions/ny%20",
	} {
		require.Equal(t, http.StatusBadRequest, get(t, path).StatusCode, path)
	}
	// The embedded data of the Field Code network is served as JSON, with an
	// ETag that a conditional request matches.
	for _, path := range []string{"/api/viz/field-code/codes", "/api/viz/field-code/borrowings"} {
		resp := get(t, path)
		require.Equal(t, http.StatusOK, resp.StatusCode, path)
		require.Equal(t, "application/json", resp.Header.Get("Content-Type"), path)
		require.NotEmpty(t, resp.Header.Get("ETag"), path)
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		require.NoError(t, err)
		req.Header.Set("If-None-Match", resp.Header.Get("ETag"))
		again, err := client.Do(req)
		require.NoError(t, err)
		again.Body.Close()
		require.Equal(t, http.StatusNotModified, again.StatusCode, path)
	}

	resp := get(t, "/linking/cap/cases?id=+1343497")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/linking/cap/cases/1343497", resp.Header.Get("Location"))

	resp, err = client.Post(srv.URL+"/works", "text/plain", nil)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

	redirects := map[string]string{
		"/linking-dashboard":        "/linking",
		"/tiers":                    "/linking/tiers",
		"/whitelist-extender":       "/linking/whitelist",
		"/treatises":                "/works",
		"/treatises?q=contracts":    "/works?q=contracts",
		"/treatise?id=CTRG95-B2993": "/editions/CTRG95-B2993",
		"/treatise":                 "/works",
		"/treatise/page?psmid=20000685307&pageid=04080": "/pages/20000685307/04080",
		"/case?source=cap&id=6754004":                   "/cases/cap/6754004",
		"/case?source=bad&id=1":                         "/cases",
		"/cite?id=3f2504e0-4f89-11d3-9a0c-0305e82c3301": "/citations/3f2504e0-4f89-11d3-9a0c-0305e82c3301",
		"/cite":                    "/citations",
		"/reporters/check?r=Mass.": "/reporters/Mass.",
		"/reporters/check?r=Mass.&tier=us_page_absent": "/citations?reporter=Mass.&tier=us_page_absent",
		"/unmatched": "/linking",
		"/unmatched/cites?volume=4&reporter=Wil.&page=877": "/citations?page=877&reporter=Wil.&status=no_match&volume=4",
		"/normalized": "/citations",
		"/normalized/cite?c=" + url.QueryEscape("2 Mass. 420"): "/citations?cite=2+Mass.+420",
	}
	for from, to := range redirects {
		t.Run(from, func(t *testing.T) {
			resp := get(t, from)
			require.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
			require.Equal(t, to, resp.Header.Get("Location"))
		})
	}
}

func TestIsUnavailable(t *testing.T) {
	require.False(t, isUnavailable(nil))
	require.False(t, isUnavailable(http.ErrServerClosed))
}
