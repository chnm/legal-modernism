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
	require.Equal(t, http.StatusNotFound, get(t, "/nope").StatusCode)
	require.Equal(t, http.StatusGone, get(t, "/api/page-text?psmid=1&pageid=2").StatusCode)

	resp, err := client.Post(srv.URL+"/works", "text/plain", nil)
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
