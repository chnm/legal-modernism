package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Page sizes for the browse lists.
const (
	treatisesPageSize  = 100
	casesPageSize      = 100
	normalizedPageSize = 200
)

// Pagination holds the prev/next state for a paginated list, preserving the
// request's other query parameters.
type Pagination struct {
	Page    int
	HasPrev bool
	HasNext bool
	PrevURL string
	NextURL string
}

// buildPagination derives prev/next links for the current request. HasNext is a
// heuristic: a full page of rows implies there may be more. gotRows is how many
// rows this page returned, pageSize the requested limit.
func buildPagination(r *http.Request, page, gotRows, pageSize int) Pagination {
	p := Pagination{Page: page, HasPrev: page > 1, HasNext: gotRows >= pageSize}
	if p.HasPrev {
		p.PrevURL = pageURL(r, page-1)
	}
	if p.HasNext {
		p.NextURL = pageURL(r, page+1)
	}
	return p
}

// pageURL rewrites the request URL with a new page number, keeping other params.
func pageURL(r *http.Request, page int) string {
	q := r.URL.Query()
	q.Set("page", strconv.Itoa(page))
	return r.URL.Path + "?" + q.Encode()
}

// handleTreatises renders the U.S. treatises browse list.
func handleTreatises(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "treatises")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sort := NormalizeTreatiseSort(r.URL.Query().Get("sort"))
	page := parsePageParam(r.URL.Query().Get("page"))

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	items, err := getUSTreatises(ctx, pool, q, sort, treatisesPageSize, (page-1)*treatisesPageSize)
	if err != nil {
		slog.Error("error querying treatises", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Debug("rendering treatises", "count", len(items), "page", page)
	data := struct {
		Q     string
		Sort  string
		Items []TreatiseListItem
		Nav   Pagination
	}{Q: q, Sort: sort, Items: items, Nav: buildPagination(r, page, len(items), treatisesPageSize)}
	if err := tmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering treatises", "error", err)
	}
}

// handleTreatise renders one treatise work: its volumes and citation-bearing pages.
func handleTreatise(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "treatise")
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Redirect(w, r, "/treatises", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	d, err := getTreatiseDetail(ctx, pool, id)
	if err != nil {
		slog.Error("error querying treatise", "id", id, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if d == nil {
		http.NotFound(w, r)
		return
	}

	slog.Debug("rendering treatise", "id", id, "volumes", len(d.Volumes))
	data := struct{ T *TreatiseDetail }{T: d}
	if err := tmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering treatise", "id", id, "error", err)
	}
}

// handleTreatisePage renders the citations detected on one treatise page.
func handleTreatisePage(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "treatise-page")
	q := r.URL.Query()
	psmid := q.Get("psmid")
	pageid := q.Get("pageid")
	if psmid == "" || pageid == "" {
		http.Redirect(w, r, "/treatises", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	v, err := getTreatisePage(ctx, pool, psmid, pageid)
	if err != nil {
		slog.Error("error querying treatise page", "psmid", psmid, "pageid", pageid, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if v == nil {
		http.NotFound(w, r)
		return
	}

	slog.Debug("rendering treatise page", "psmid", psmid, "pageid", pageid, "citations", len(v.Citations))
	data := struct{ Page *TreatisePageView }{Page: v}
	if err := tmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering treatise page", "psmid", psmid, "pageid", pageid, "error", err)
	}
}

// handlePageText serves a treatise page's OCR text as JSON, on demand.
func handlePageText(w http.ResponseWriter, r *http.Request) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "page-text")
	q := r.URL.Query()
	psmid := q.Get("psmid")
	pageid := q.Get("pageid")
	if psmid == "" || pageid == "" {
		http.Error(w, "psmid and pageid are required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	text, err := getPageOCRText(ctx, pool, psmid, pageid)
	if err != nil {
		slog.Error("error querying page OCR text", "psmid", psmid, "pageid", pageid, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "max-age=3600")
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Text string `json:"text"`
	}{Text: text}); err != nil {
		slog.Error("error encoding page text JSON", "error", err)
	}
}

// handleCases renders the cases browse list, ranked by treatise-page citations.
func handleCases(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "cases")
	page := parsePageParam(r.URL.Query().Get("page"))

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	items, err := getTopCases(ctx, pool, casesPageSize, (page-1)*casesPageSize)
	if err != nil {
		slog.Error("error querying cases", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Debug("rendering cases", "count", len(items), "page", page)
	data := struct {
		Items []CaseListItem
		Nav   Pagination
	}{Items: items, Nav: buildPagination(r, page, len(items), casesPageSize)}
	if err := tmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering cases", "error", err)
	}
}

// handleCase renders one case and the treatise pages that cite it.
func handleCase(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "case")
	q := r.URL.Query()
	source := q.Get("source")
	id := q.Get("id")
	if !ValidCaseSource(source) || id == "" {
		http.Redirect(w, r, "/cases", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	d, err := getCaseDetail(ctx, pool, source, id)
	if err != nil {
		slog.Error("error querying case", "source", source, "id", id, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Debug("rendering case", "source", source, "id", id, "pages", d.PageCount)
	data := struct {
		Case      *CaseDetail
		Truncated bool
	}{Case: d, Truncated: d.PageCount >= caseDetailCitingLimit}
	if err := tmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering case", "source", source, "id", id, "error", err)
	}
}

// handleNormalized renders the normalized-citations browse list.
func handleNormalized(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "normalized")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	page := parsePageParam(r.URL.Query().Get("page"))

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	items, err := getTopNormalized(ctx, pool, query, normalizedPageSize, (page-1)*normalizedPageSize)
	if err != nil {
		slog.Error("error querying normalized citations", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Debug("rendering normalized citations", "count", len(items), "page", page)
	data := struct {
		Q     string
		Items []NormalizedListItem
		Nav   Pagination
	}{Q: query, Items: items, Nav: buildPagination(r, page, len(items), normalizedPageSize)}
	if err := tmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering normalized citations", "error", err)
	}
}

// handleNormalizedCite renders the treatise-page instances of one normalized citation.
func handleNormalizedCite(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	slog.Debug("handling request", "path", r.URL.Path, "handler", "normalized-cite")
	cite := r.URL.Query().Get("c")
	if cite == "" {
		http.Redirect(w, r, "/normalized", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	cites, total, err := getNormalizedCites(ctx, pool, cite)
	if err != nil {
		slog.Error("error querying normalized citation instances", "cite", cite, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Debug("rendering normalized cite", "cite", cite, "shown", len(cites), "total", total)
	data := struct {
		Cite      string
		Cites     []NormalizedCite
		Shown     int
		Total     int
		Truncated bool
	}{Cite: cite, Cites: cites, Shown: len(cites), Total: total, Truncated: total > len(cites)}
	if err := tmpl.ExecuteTemplate(w, "baseof", data); err != nil {
		slog.Error("error rendering normalized cite", "cite", cite, "error", err)
	}
}
