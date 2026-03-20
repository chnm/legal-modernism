package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lmullen/legal-modernism/go/db"
	flag "github.com/spf13/pflag"
)

var pool *pgxpool.Pool

// Template functions for dereferencing pointers in templates.
var funcMap = template.FuncMap{
	"deref": func(v any) any {
		switch p := v.(type) {
		case *int:
			if p != nil {
				return *p
			}
		case *string:
			if p != nil {
				return *p
			}
		}
		return nil
	},
	"derefStr": func(v *string) string {
		if v != nil {
			return *v
		}
		return ""
	},
	"ptrOr": func(v *string, fallback string) template.HTML {
		if v != nil && *v != "" {
			return template.HTML(template.HTMLEscapeString(*v))
		}
		return template.HTML(fallback)
	},
	"highlightRaw": func(ocrtext, raw string) template.HTML {
		escaped := template.HTMLEscapeString(ocrtext)
		rawEscaped := template.HTMLEscapeString(raw)
		highlighted := strings.ReplaceAll(escaped, rawEscaped, "<mark>"+rawEscaped+"</mark>")
		return template.HTML(highlighted)
	},
}

func init() {
	initLogger()
}

// parseTemplates parses each page template together with baseof.html so that
// block overrides work correctly.
func parseTemplates() map[string]*template.Template {
	pages := []string{
		"home.html",
		"detail.html",
		"cite-lookup.html",
		"reporters.html",
		"reporter-cites.html",
		"dashboard.html",
		"whitelist-extender.html",
	}
	tmpls := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t := template.Must(
			template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/baseof.html", "templates/"+page),
		)
		tmpls[page] = t
	}
	return tmpls
}

func main() {
	var port int
	flag.IntVar(&port, "port", 4567, "port to listen on")
	flag.Parse()

	slog.Info("starting chambers")

	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(quit)
		cancel()
	}()

	var err error
	pool, err = db.Connect(ctx)
	if err != nil {
		slog.Error("error connecting to database", "database", db.Host(), "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to database", "database", db.Host())

	tmpls := parseTemplates()
	slog.Debug("parsed templates", "count", len(tmpls))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleHome(w, r, tmpls["home.html"])
	})
	mux.HandleFunc("/cite", func(w http.ResponseWriter, r *http.Request) {
		handleCiteLookup(w, r, tmpls["cite-lookup.html"], tmpls["detail.html"])
	})
	mux.HandleFunc("/reporters", func(w http.ResponseWriter, r *http.Request) {
		handleReporters(w, r, tmpls["reporters.html"])
	})
	mux.HandleFunc("/reporters/check", func(w http.ResponseWriter, r *http.Request) {
		handleReporterCites(w, r, tmpls["reporter-cites.html"])
	})
	mux.HandleFunc("/linking-dashboard", func(w http.ResponseWriter, r *http.Request) {
		handleDashboard(w, r, tmpls["dashboard.html"])
	})
	mux.HandleFunc("/api/linking-dashboard", handleDashboardAPI)
	mux.HandleFunc("/whitelist-extender", func(w http.ResponseWriter, r *http.Request) {
		handleWhitelistExtender(w, r, tmpls["whitelist-extender.html"])
	})
	mux.HandleFunc("/api/whitelist-extender", handleWhitelistExtenderAPI)
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		select {
		case <-quit:
			slog.Info("shutting down server")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				slog.Error("error shutting down server", "error", err)
			}
			cancel()
		case <-ctx.Done():
		}
	}()

	slog.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "home")
	if r.URL.Path != "/" {
		slog.Debug("not found", "path", r.URL.Path)
		http.NotFound(w, r)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "baseof", nil); err != nil {
		slog.Error("error rendering home", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func handleCiteLookup(w http.ResponseWriter, r *http.Request, lookupTmpl, detailTmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "cite-lookup")
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		slog.Debug("rendering cite-lookup form (no id)")
		data := struct{ Error string }{}
		if err := lookupTmpl.ExecuteTemplate(w, "baseof", data); err != nil {
			slog.Error("error rendering cite-lookup", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	slog.Debug("cite lookup requested", "id", idStr)
	id, err := uuid.Parse(idStr)
	if err != nil {
		slog.Debug("invalid UUID", "id", idStr)
		w.WriteHeader(http.StatusBadRequest)
		data := struct{ Error string }{Error: fmt.Sprintf("Invalid UUID: %s", idStr)}
		lookupTmpl.ExecuteTemplate(w, "baseof", data)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	cite, err := getCitationDetail(ctx, pool, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("citation not found", "id", id)
			w.WriteHeader(http.StatusNotFound)
			data := struct{ Error string }{Error: fmt.Sprintf("Citation not found: %s", id)}
			lookupTmpl.ExecuteTemplate(w, "baseof", data)
			return
		}
		slog.Error("error querying citation", "id", id, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Debug("rendering citation detail", "id", id, "status", cite.Status)
	data := struct{ Cite *CitationDetail }{Cite: cite}
	if err := detailTmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering detail", "id", id, "error", err)
	}
}

func handleReporters(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "reporters")
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	reporters, err := getReporterStandards(ctx, pool)
	if err != nil {
		slog.Error("error querying reporters", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Debug("rendering reporters page", "count", len(reporters))
	data := struct{ Reporters []ReporterStandard }{Reporters: reporters}
	if err := tmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering reporters", "error", err)
	}
}

func handleDashboard(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "dashboard")
	if err := tmpl.ExecuteTemplate(w, "baseof", nil); err != nil {
		slog.Error("error rendering dashboard", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func handleDashboardAPI(w http.ResponseWriter, r *http.Request) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "dashboard-api")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	slog.Debug("fetching dashboard data from database")
	data, err := getDashboardData(ctx, pool)
	if err != nil {
		slog.Error("error querying dashboard data", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Debug("sending dashboard JSON response",
		"total_raw_cites", data.TotalRawCites,
		"total_linked", data.TotalLinked(),
	)
	w.Header().Set("Cache-Control", "max-age=3600")
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("error encoding dashboard JSON", "error", err)
	}
}

func handleReporterCites(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "reporter-cites")
	reporter := r.URL.Query().Get("r")
	if reporter == "" {
		slog.Debug("no reporter specified, redirecting to /reporters")
		http.Redirect(w, r, "/reporters", http.StatusFound)
		return
	}

	slog.Debug("looking up reporter cites", "reporter", reporter)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	variants, err := getReporterVariants(ctx, pool, reporter)
	if err != nil {
		slog.Error("error querying variants for reporter", "reporter", reporter, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	cites, err := getCitesForReporter(ctx, pool, reporter)
	if err != nil {
		slog.Error("error querying cites for reporter", "reporter", reporter, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Debug("rendering reporter cites", "reporter", reporter, "variants", len(variants), "cites", len(cites))
	data := struct {
		Reporter string
		Variants []string
		Cites    []ReporterCite
	}{Reporter: reporter, Variants: variants, Cites: cites}
	if err := tmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering reporter cites", "reporter", reporter, "error", err)
	}
}

func handleWhitelistExtender(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "whitelist-extender")
	if err := tmpl.ExecuteTemplate(w, "baseof", nil); err != nil {
		slog.Error("error rendering whitelist extender", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func handleWhitelistExtenderAPI(w http.ResponseWriter, r *http.Request) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "whitelist-extender-api")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	reporters, err := getUnwhitelistedReporters(ctx, pool)
	if err != nil {
		slog.Error("error querying unwhitelisted reporters", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	standards, err := getDistinctReporterStandards(ctx, pool)
	if err != nil {
		slog.Error("error querying reporter standards", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	capMap, err := getCapInfoMap(ctx, pool)
	if err != nil {
		slog.Error("error querying cap info", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	for i := range reporters {
		reporters[i].Matches = computeMatches(reporters[i].ReporterAbbr, standards, capMap)
	}

	slog.Debug("sending whitelist extender JSON response", "count", len(reporters))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reporters); err != nil {
		slog.Error("error encoding whitelist extender JSON", "error", err)
	}
}
