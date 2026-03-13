package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"io/fs"
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

	tmpl := template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleHome(w, r, tmpl)
	})
	mux.HandleFunc("/cite", func(w http.ResponseWriter, r *http.Request) {
		handleCiteLookup(w, r, tmpl)
	})
	mux.HandleFunc("/reporters", func(w http.ResponseWriter, r *http.Request) {
		handleReporters(w, r, tmpl)
	})
	mux.HandleFunc("/reporters/check", func(w http.ResponseWriter, r *http.Request) {
		handleReporterCites(w, r, tmpl)
	})
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
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "home.html", nil); err != nil {
		slog.Error("error rendering home", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func handleCiteLookup(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		data := struct{ Error string }{}
		if err := tmpl.ExecuteTemplate(w, "cite-lookup.html", data); err != nil {
			slog.Error("error rendering cite-lookup", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		data := struct{ Error string }{Error: fmt.Sprintf("Invalid UUID: %s", idStr)}
		tmpl.ExecuteTemplate(w, "cite-lookup.html", data)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	cite, err := getCitationDetail(ctx, pool, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			data := struct{ Error string }{Error: fmt.Sprintf("Citation not found: %s", id)}
			tmpl.ExecuteTemplate(w, "cite-lookup.html", data)
			return
		}
		slog.Error("error querying citation", "id", id, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct{ Cite *CitationDetail }{Cite: cite}
	if err := tmpl.ExecuteTemplate(w, "detail.html", data); err != nil {
		slog.Error("error rendering detail", "id", id, "error", err)
	}
}

func handleReporters(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	reporters, err := getReporterStandards(ctx, pool)
	if err != nil {
		slog.Error("error querying reporters", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct{ Reporters []ReporterStandard }{Reporters: reporters}
	if err := tmpl.ExecuteTemplate(w, "reporters.html", data); err != nil {
		slog.Error("error rendering reporters", "error", err)
	}
}

func handleReporterCites(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	reporter := r.URL.Query().Get("r")
	if reporter == "" {
		http.Redirect(w, r, "/reporters", http.StatusFound)
		return
	}

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

	data := struct {
		Reporter string
		Variants []string
		Cites    []ReporterCite
	}{Reporter: reporter, Variants: variants, Cites: cites}
	if err := tmpl.ExecuteTemplate(w, "reporter-cites.html", data); err != nil {
		slog.Error("error rendering reporter cites", "reporter", reporter, "error", err)
	}
}
