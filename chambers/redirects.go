package main

import (
	"net/http"
	"net/url"
)

// The routes chambers had before the redesign (issue #305), redirected to
// their new homes so bookmarks and links in issues keep working. Each entry
// maps an old path to a function of its query string.
var legacyRoutes = map[string]func(q url.Values) string{
	"/linking-dashboard":  func(url.Values) string { return "/linking" },
	"/tiers":              func(url.Values) string { return "/linking/tiers" },
	"/whitelist-extender": func(url.Values) string { return "/linking/whitelist" },
	"/unmatched":          func(url.Values) string { return "/linking" },
	"/normalized":         func(url.Values) string { return "/citations" },
	"/treatises": func(q url.Values) string {
		if s := q.Get("q"); s != "" {
			return "/works?" + url.Values{"q": {s}}.Encode()
		}
		return "/works"
	},
	"/treatise": func(q url.Values) string {
		if id := q.Get("id"); id != "" {
			return editionURL(id)
		}
		return "/works"
	},
	"/treatise/page": func(q url.Values) string {
		if psmid, pageid := q.Get("psmid"), q.Get("pageid"); psmid != "" && pageid != "" {
			return pageURL(psmid, pageid)
		}
		return "/works"
	},
	"/case": func(q url.Values) string {
		if source, id := q.Get("source"), q.Get("id"); validCaseSource(source) && id != "" {
			return caseURL(source, id)
		}
		return "/cases"
	},
	"/cite": func(q url.Values) string {
		if id := q.Get("id"); id != "" {
			return citationURL(id)
		}
		return "/citations"
	},
	"/reporters/check": func(q url.Values) string {
		r := q.Get("r")
		if r == "" {
			return "/reporters"
		}
		if tier := q.Get("tier"); tier != "" {
			return citationsURL(url.Values{"reporter": {r}, "tier": {tier}})
		}
		return reporterURL(r)
	},
	"/unmatched/cites": func(q url.Values) string {
		v := url.Values{}
		for _, k := range []string{"reporter", "volume", "page"} {
			if s := q.Get(k); s != "" {
				v.Set(k, s)
			}
		}
		if len(v) == 0 {
			return "/linking"
		}
		v.Set("status", "no_match")
		return citationsURL(v)
	},
	"/normalized/cite": func(q url.Values) string {
		if c := q.Get("c"); c != "" {
			return citationsURL(url.Values{"cite": {c}})
		}
		return "/citations"
	},
}

// handleLegacy redirects an old route permanently to its new home.
func handleLegacy(w http.ResponseWriter, r *http.Request) {
	target, ok := legacyRoutes[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, target(r.URL.Query()), http.StatusMovedPermanently)
}

// handleGone answers an endpoint that no longer exists.
func handleGone(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "This endpoint was removed.", http.StatusGone)
}
