package main

import (
	"embed"
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
// an API: cached for an hour, like the JSON the other APIs write.
func serveData(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := dataFS.ReadFile("data/" + name)
		if err != nil {
			slog.Error("reading embedded data", "file", name, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}
}
