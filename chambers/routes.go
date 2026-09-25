package main

import (
	"io/fs"
	"log/slog"
	"net/http"
)

// routes registers every page, API and legacy redirect on a ServeMux.
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleHome)

	// Treatises: works, editions, pages.
	mux.HandleFunc("GET /works", s.handleWorks)
	mux.HandleFunc("GET /works/{id}", s.handleWork)
	mux.HandleFunc("GET /editions/{id}", s.handleEdition)
	mux.HandleFunc("GET /editions/{id}/cases", s.handleEditionCases)
	mux.HandleFunc("GET /pages/{psmid}/{pageid}", s.handlePage)

	// Cases.
	mux.HandleFunc("GET /cases", s.handleCases)
	mux.HandleFunc("GET /cases/{source}/{id}", s.handleCase)

	// Reporters.
	mux.HandleFunc("GET /reporters", s.handleReporters)
	mux.HandleFunc("GET /reporters/{standard}", s.handleReporter)

	// Citations.
	mux.HandleFunc("GET /citations", s.handleCitations)
	mux.HandleFunc("GET /citations/{id}", s.handleCitation)

	// Detecting and linking.
	mux.HandleFunc("GET /linking", s.handleLinking)
	mux.HandleFunc("GET /linking/tiers", s.handleLinkingTiers)
	mux.HandleFunc("GET /linking/whitelist", s.handleWhitelist)
	mux.HandleFunc("GET /api/linking-dashboard", s.handleLinkingAPI)
	mux.HandleFunc("GET /api/tiers", s.handleTiersAPI)
	mux.HandleFunc("GET /api/whitelist-extender", s.handleWhitelistAPI)

	// Static files.
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("embedded static files: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// The routes of the app before the redesign.
	for path := range legacyRoutes {
		mux.HandleFunc("GET "+path, handleLegacy)
	}
	mux.HandleFunc("GET /api/page-text", handleGone)

	return logRequests(mux)
}

// logRequests logs every request at debug level.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("request", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)
		next.ServeHTTP(w, r)
	})
}

// handleHome renders the landing page. It reads nothing from the database.
func (s *server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "home.html", http.StatusOK, struct{ Page }{Page{Title: "Chambers", Section: "home"}})
}
