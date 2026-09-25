package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lmullen/legal-modernism/go/sources"
)

// server holds what every handler needs: the connection pool, the parsed
// templates, and the OCR replacer the page view uses to reproduce the text the
// detector saw. Handlers are its methods, one entity per file.
type server struct {
	db   *pgxpool.Pool
	tmpl map[string]*template.Template

	ocrOnce sync.Once
	ocr     *sources.OCRReplacer
}

func newServer(db *pgxpool.Pool, tmpl map[string]*template.Template) *server {
	return &server{db: db, tmpl: tmpl}
}

// Page is the header every template receives: the title, the navbar section to
// mark as active, the breadcrumb trail, and notices about sections that could
// not be filled because a materialized view is not populated yet.
type Page struct {
	Title   string
	Section string
	Crumbs  []Crumb
	Notices []string
}

// Crumb is one step of a breadcrumb trail. The last crumb has no URL.
type Crumb struct {
	Label string
	URL   string
}

// unavailable records that a section reading the named view could not be
// filled, so the page can say so instead of failing.
func (p *Page) unavailable(view string) {
	p.Notices = append(p.Notices, fmt.Sprintf(
		"A section of this page reads %s, which is not populated. Run make db-maintenance.", view))
}

// optional turns the error of an optional section into a notice on the page
// when the section's view is missing or unpopulated, and returns every other
// error unchanged. It lets a page render without a section rather than fail.
func optional(err error, page *Page, view string) error {
	if err == nil {
		return nil
	}
	if isUnavailable(err) {
		slog.Warn("section unavailable", "view", view, "error", err)
		page.unavailable(view)
		return nil
	}
	return err
}

// isUnavailable reports whether a query failed because a materialized view it
// reads has not been populated (SQLSTATE 55000) or does not exist yet (42P01):
// the states between applying a migration and running make db-maintenance, or
// between deploying the app and applying the migration.
func isUnavailable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "55000" || pgErr.Code == "42P01"
}

// isNoRows reports whether a single-row query found nothing.
func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// ctx derives a request context with a deadline for the handler's queries.
func (s *server) ctx(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

// render executes a page template inside the base layout. The page is rendered
// into a buffer first, so a template error becomes a clean 500 rather than a
// half-written page.
func (s *server) render(w http.ResponseWriter, r *http.Request, name string, status int, data any) {
	t, ok := s.tmpl[name]
	if !ok {
		slog.Error("no such template", "template", name, "path", r.URL.Path)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "baseof", data); err != nil {
		slog.Error("error rendering template", "template", name, "path", r.URL.Path, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		slog.Debug("error writing response", "path", r.URL.Path, "error", err)
	}
}

// errorPage is the data of error.html.
type errorPage struct {
	Page
	Status  int
	Message string
}

// serverError logs a failure with its context and renders a generic 500. The
// error itself stays in the log.
func (s *server) serverError(w http.ResponseWriter, r *http.Request, err error, msg string, attrs ...any) {
	attrs = append(attrs, "path", r.URL.Path, "error", err)
	slog.Error(msg, attrs...)
	s.render(w, r, "error.html", http.StatusInternalServerError, errorPage{
		Page:    Page{Title: "Something went wrong"},
		Status:  http.StatusInternalServerError,
		Message: "Something went wrong while " + msg + ". The details are in the server log.",
	})
}

// notFound renders a 404 with a sentence saying what was looked for.
func (s *server) notFound(w http.ResponseWriter, r *http.Request, msg string) {
	slog.Debug("not found", "path", r.URL.Path, "what", msg)
	s.render(w, r, "error.html", http.StatusNotFound, errorPage{
		Page:    Page{Title: "Not found"},
		Status:  http.StatusNotFound,
		Message: msg,
	})
}

// badRequest renders a 400 with a sentence saying what was wrong.
func (s *server) badRequest(w http.ResponseWriter, r *http.Request, msg string) {
	slog.Debug("bad request", "path", r.URL.Path, "what", msg)
	s.render(w, r, "error.html", http.StatusBadRequest, errorPage{
		Page:    Page{Title: "Bad request"},
		Status:  http.StatusBadRequest,
		Message: msg,
	})
}

// ocrReplacer returns the OCR corrections the detector applies before it reads
// a page, loaded from legalhist.ocr_corrections on first use. A load failure is
// logged and leaves the text uncorrected, which only costs a few highlights.
func (s *server) ocrReplacer(ctx context.Context) *sources.OCRReplacer {
	s.ocrOnce.Do(func() {
		if s.db == nil {
			return
		}
		subs, err := sources.NewPgxStore(s.db).GetOCRSubstitutions(ctx)
		if err != nil {
			slog.Warn("could not load OCR corrections; page text will be shown uncorrected", "error", err)
			return
		}
		s.ocr = sources.NewOCRReplacer(subs)
		slog.Info("loaded OCR corrections", "count", len(subs))
	})
	return s.ocr
}

// collect runs a query and scans every row with scan. It wraps the two error
// paths every list query shares.
func collect[T any](ctx context.Context, db *pgxpool.Pool, sql string, args []any, scan func(pgx.Rows) (T, error)) ([]T, error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("querying: %w", err)
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating: %w", err)
	}
	return out, nil
}
