package main

import (
	"net/http"
	"strconv"
)

// Pagination is the prev/next state of a paginated list. Total is the number
// of rows across every page when the query counted them, or -1 when it did
// not, in which case a full page is taken to mean there may be more.
type Pagination struct {
	Page     int
	PageSize int
	Shown    int
	Total    int
	HasPrev  bool
	HasNext  bool
	PrevURL  string
	NextURL  string
}

// paginate derives the prev/next links for the current request, keeping its
// other query parameters.
func paginate(r *http.Request, page, size, shown, total int) Pagination {
	p := Pagination{Page: page, PageSize: size, Shown: shown, Total: total, HasPrev: page > 1}
	if total >= 0 {
		p.HasNext = page*size < total
	} else {
		p.HasNext = shown >= size
	}
	if p.HasPrev {
		p.PrevURL = pageNumberURL(r, page-1)
	}
	if p.HasNext {
		p.NextURL = pageNumberURL(r, page+1)
	}
	return p
}

// From is the 1-based rank of the first row on this page.
func (p Pagination) From() int {
	if p.Shown == 0 {
		return 0
	}
	return (p.Page-1)*p.PageSize + 1
}

// To is the rank of the last row on this page.
func (p Pagination) To() int {
	return (p.Page-1)*p.PageSize + p.Shown
}

// Offset is the number of rows to skip for this page.
func (p Pagination) Offset() int {
	return (p.Page - 1) * p.PageSize
}

// pageNumberURL rewrites the request URL with a new page number, keeping the
// other parameters.
func pageNumberURL(r *http.Request, page int) string {
	q := r.URL.Query()
	q.Set("page", strconv.Itoa(page))
	return r.URL.Path + "?" + q.Encode()
}

// parsePage reads a 1-based ?page= value, defaulting to 1.
func parsePage(s string) int {
	if s == "" {
		return 1
	}
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 {
		return 1
	}
	return p
}
