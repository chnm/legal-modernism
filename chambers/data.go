package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"log/slog"
	"net/http"
)

// dataFS holds the static data behind the public visualizations that do not
// read the database: for now the Field Code network (issue #117), two JSON
// arrays that scripts/field-code-network derives from the research
// compendium of Funk and Mullen, "The Spine of American Law" (AHR 2018). The
// API handlers serve these files as they would a query's rows, so that the
// chart code does not change if the data moves to the database.
//
//go:embed data/*.json
var dataFS embed.FS

// serveData returns a handler that serves one embedded JSON file of data/ as
// an API. The file changes only with a deploy, so rather than the hour's
// cache the query-backed APIs allow, it carries an ETag of its content and
// asks the browser to revalidate, answering 304 when nothing changed: a
// reload after a deploy then sees the new data, not a stale copy.
func serveData(name string) http.HandlerFunc {
	b, err := dataFS.ReadFile("data/" + name)
	if err != nil {
		panic("embedded data: " + err.Error())
	}
	sum := sha256.Sum256(b)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(b); err != nil {
			slog.Debug("writing embedded data", "file", name, "error", err)
		}
	}
}
