package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
)

// Reporters are the series of law reports the treatises cite
// (legalhist.reporters, keyed by reporter_standard). A reporter connects to
// cases only through the citations: the spelling a treatise used is mapped to
// a standard by the whitelist, and the linker resolves the cite.

// ReporterListItem is one reporter on the list.
type ReporterListItem struct {
	Standard        string
	Title           *string
	Level           *string
	Jurisdiction    *string
	Type            *string
	YearStart       *int
	YearEnd         *int
	SingleVol       *bool
	CitedByYearFrom *int
	Spellings       int
	Linked          int
	NoMatch         int
	Unprocessed     int
}

func (r ReporterListItem) URL() string { return reporterURL(r.Standard) }

// Cites is the number of citations the linker probed for this reporter.
func (r ReporterListItem) Cites() int { return r.Linked + r.NoMatch + r.Unprocessed }

func (r ReporterListItem) LinkedPct() string { return pct(r.Linked, r.Cites()) }
func (r ReporterListItem) Years() string     { return yearSpan(r.YearStart, r.YearEnd) }

// Side is US or UK from the jurisdiction ("us:ma", "uk:kb").
func (r ReporterListItem) Side() string {
	if r.Jurisdiction == nil {
		return ""
	}
	side, _, _ := strings.Cut(*r.Jurisdiction, ":")
	return strings.ToUpper(side)
}

// Place is the part of the jurisdiction after the side: the state, or the
// English court.
func (r ReporterListItem) Place() string {
	if r.Jurisdiction == nil {
		return ""
	}
	_, place, _ := strings.Cut(*r.Jurisdiction, ":")
	return strings.TrimSpace(place)
}

var reporterSorts = map[string]string{
	"cites":    "COALESCE(d.linked, 0) + COALESCE(d.no_match, 0) + COALESCE(d.unprocessed, 0) DESC, r.reporter_standard",
	"name":     "r.reporter_standard",
	"title":    "r.reporter_title NULLS LAST, r.reporter_standard",
	"earliest": "r.year_start NULLS LAST, r.reporter_standard",
	"linked":   "CASE WHEN COALESCE(d.linked, 0) + COALESCE(d.no_match, 0) = 0 THEN NULL ELSE d.linked::float / (d.linked + d.no_match) END DESC NULLS LAST, r.reporter_standard",
}

func normalizeReporterSort(sort string) string {
	if _, ok := reporterSorts[sort]; ok {
		return sort
	}
	return "cites"
}

var reporterTypes = map[string]bool{"official": true, "nominate": true, "specialized": true, "statute": true}

// getReporters lists the reporters with their linking totals from
// linking_dashboard_reporters and their spelling counts from the whitelist.
func (s *server) getReporters(ctx context.Context, side, typ, q, sort string) ([]ReporterListItem, error) {
	slog.Debug("querying reporters", "side", side, "type", typ, "q", q, "sort", sort)
	query := `
	SELECT r.reporter_standard, r.reporter_title, r.level, r.jurisdiction, r.type, r.year_start, r.year_end,
	       r.single_vol, r.cited_by_year_from,
	       COALESCE(w.n, 0), COALESCE(d.linked, 0), COALESCE(d.no_match, 0), COALESCE(d.unprocessed, 0)
	FROM legalhist.reporters r
	LEFT JOIN moml_citations.linking_dashboard_reporters d ON d.reporter_standard = r.reporter_standard
	LEFT JOIN (SELECT reporter_standard, count(*) AS n FROM legalhist.whitelist
	           WHERE reporter_standard IS NOT NULL GROUP BY reporter_standard) w ON w.reporter_standard = r.reporter_standard
	WHERE ($1 = '' OR split_part(r.jurisdiction, ':', 1) = $1)
	  AND ($2 = '' OR r.type = $2)
	  AND ($3 = '' OR r.reporter_standard ILIKE '%' || $3 || '%' OR r.reporter_title ILIKE '%' || $3 || '%')
	ORDER BY ` + reporterSorts[normalizeReporterSort(sort)]
	items, err := collect(ctx, s.db, query, []any{side, typ, q}, func(rows pgx.Rows) (ReporterListItem, error) {
		var r ReporterListItem
		err := rows.Scan(&r.Standard, &r.Title, &r.Level, &r.Jurisdiction, &r.Type, &r.YearStart, &r.YearEnd,
			&r.SingleVol, &r.CitedByYearFrom, &r.Spellings, &r.Linked, &r.NoMatch, &r.Unprocessed)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("reporters: %w", err)
	}
	return items, nil
}

func (s *server) handleReporters(w http.ResponseWriter, r *http.Request) {
	side := strings.ToLower(r.URL.Query().Get("jur"))
	if side != "us" && side != "uk" {
		side = ""
	}
	typ := r.URL.Query().Get("type")
	if !reporterTypes[typ] {
		typ = ""
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sort := normalizeReporterSort(r.URL.Query().Get("sort"))

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	data := struct {
		Page
		Side  string
		Type  string
		Q     string
		Sort  string
		Items []ReporterListItem
	}{Page: Page{Title: "Reporters", Section: "reporters"}, Side: side, Type: typ, Q: q, Sort: sort}

	items, err := s.getReporters(ctx, side, typ, q, sort)
	if err := optional(err, &data.Page, "moml_citations.linking_dashboard_reporters"); err != nil {
		s.serverError(w, r, err, "listing reporters")
		return
	}
	data.Items = items
	s.render(w, r, "reporters.html", http.StatusOK, data)
}

// ReporterDetail is the reporter page.
type ReporterDetail struct {
	Page
	ReporterListItem
	ReporterCap *string
	Spellings   []Spelling
	AltAbbrs    []string
	DiffVols    []DiffVol
	Outcomes    []Outcome
	TopCases    TopCasesView
	TopEditions []ReporterEdition
	Unmatched   []UnmatchedCite
}

func (d *ReporterDetail) CitationsURL() string {
	return citationsURL(url.Values{"reporter": {d.Standard}})
}

// Spelling is one whitelisted spelling of the reporter and how often the
// detector found it.
type Spelling struct {
	Found string
	N     int
}

// DiffVol is one row of the reporter's volume renumbering into CAP.
type DiffVol struct {
	Vol         *int
	CAPVol      int
	CAPReporter string
}

// Outcome is one status and tier of the reporter's citations, from
// linking_dashboard_tiers.
type Outcome struct {
	Chip Chip
	N    int
	URL  string
}

// ReporterEdition is one edition that cites the reporter.
type ReporterEdition struct {
	BiblioID     string
	Title        string
	Year         *int
	Jurisdiction string
	WorkID       int
	WorkTitle    string
	Cites        int
	Linked       int
}

func (e ReporterEdition) URL() string       { return editionURL(e.BiblioID) }
func (e ReporterEdition) WorkURL() string   { return workURL(e.WorkID) }
func (e ReporterEdition) LinkedPct() string { return pct(e.Linked, e.Cites) }

// UnmatchedCite is one cite string of the reporter the linker could not
// match, with how often it recurs.
type UnmatchedCite struct {
	Volume *int
	Page   int
	N      int
	URL    string
}

func (u UnmatchedCite) Cite(reporter string) string {
	if u.Volume == nil {
		return fmt.Sprintf("%s %d", reporter, u.Page)
	}
	return fmt.Sprintf("%d %s %d", *u.Volume, reporter, u.Page)
}

// getReporter loads the reporter's record and totals. Returns (nil, nil) when
// there is no such standard.
func (s *server) getReporter(ctx context.Context, standard string) (*ReporterDetail, error) {
	d := &ReporterDetail{}
	err := s.db.QueryRow(ctx, `
		SELECT r.reporter_standard, r.reporter_title, r.level, r.jurisdiction, r.type, r.year_start, r.year_end,
		       r.single_vol, r.cited_by_year_from, r.reporter_cap
		FROM legalhist.reporters r WHERE r.reporter_standard = $1`, standard).Scan(
		&d.Standard, &d.ReporterListItem.Title, &d.Level, &d.Jurisdiction, &d.Type, &d.YearStart, &d.YearEnd,
		&d.SingleVol, &d.CitedByYearFrom, &d.ReporterCap)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reporter: %w", err)
	}
	return d, nil
}

// getReporterTotals reads the reporter's row of linking_dashboard_reporters.
func (s *server) getReporterTotals(ctx context.Context, d *ReporterDetail) error {
	err := s.db.QueryRow(ctx, `
		SELECT linked, no_match, unprocessed FROM moml_citations.linking_dashboard_reporters
		WHERE reporter_standard = $1`, d.Standard).Scan(&d.Linked, &d.NoMatch, &d.Unprocessed)
	if isNoRows(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reporter totals: %w", err)
	}
	return nil
}

// getReporterSpellings lists the whitelist spellings that map to the
// reporter, with how often the detector found each (legalhist.top_reporters).
func (s *server) getReporterSpellings(ctx context.Context, standard string) ([]Spelling, error) {
	items, err := collect(ctx, s.db, `
		SELECT wl.reporter_found, COALESCE(t.n, 0)
		FROM legalhist.whitelist wl
		LEFT JOIN legalhist.top_reporters t ON t.reporter_abbr = wl.reporter_found
		WHERE wl.reporter_standard = $1
		ORDER BY COALESCE(t.n, 0) DESC, wl.reporter_found`, []any{standard},
		func(rows pgx.Rows) (Spelling, error) {
			var sp Spelling
			return sp, rows.Scan(&sp.Found, &sp.N)
		})
	if err != nil {
		return nil, fmt.Errorf("reporter spellings: %w", err)
	}
	return items, nil
}

func (s *server) getReporterAltAbbrs(ctx context.Context, standard string) ([]string, error) {
	items, err := collect(ctx, s.db, `
		SELECT alt_abbr FROM legalhist.reporters_abbreviations WHERE reporter_standard = $1 ORDER BY alt_abbr`,
		[]any{standard}, func(rows pgx.Rows) (string, error) {
			var v string
			return v, rows.Scan(&v)
		})
	if err != nil {
		return nil, fmt.Errorf("reporter alternate abbreviations: %w", err)
	}
	return items, nil
}

func (s *server) getReporterDiffVols(ctx context.Context, standard string) ([]DiffVol, error) {
	items, err := collect(ctx, s.db, `
		SELECT vol, cap_vol, cap_reporter FROM legalhist.reporters_diffvols
		WHERE reporter_standard = $1 ORDER BY vol NULLS FIRST, cap_vol`,
		[]any{standard}, func(rows pgx.Rows) (DiffVol, error) {
			var d DiffVol
			return d, rows.Scan(&d.Vol, &d.CAPVol, &d.CAPReporter)
		})
	if err != nil {
		return nil, fmt.Errorf("reporter diffvols: %w", err)
	}
	return items, nil
}

// getReporterOutcomes reads the reporter's citations by status and tier, each
// linking to the matching citations.
func (s *server) getReporterOutcomes(ctx context.Context, standard string) ([]Outcome, error) {
	items, err := collect(ctx, s.db, `
		SELECT status, match_tier, n FROM moml_citations.linking_dashboard_tiers
		WHERE reporter_standard = $1 ORDER BY n DESC`, []any{standard},
		func(rows pgx.Rows) (Outcome, error) {
			var o Outcome
			var status, tier *string
			err := rows.Scan(&status, &tier, &o.N)
			o.Chip = chipFor(status, tier)
			v := url.Values{"reporter": {standard}, "status": {o.Chip.Key()}}
			if o.Chip.Tier != "" {
				v.Set("status", o.Chip.Status)
				v.Set("tier", o.Chip.Tier)
			}
			o.URL = citationsURL(v)
			return o, err
		})
	if err != nil {
		return nil, fmt.Errorf("reporter outcomes: %w", err)
	}
	return items, nil
}

// getReporterTopCases ranks the cases reached through citations to the
// reporter by the treatise editions citing them.
func (s *server) getReporterTopCases(ctx context.Context, standard string, limit int) ([]TopCase, error) {
	query := `
	SELECT rc.editions, rc.cites, ` + caseMetaColumns("rc") + `
	FROM moml_citations.reporter_case_citations rc ` + caseMetaJoins("rc") + `
	WHERE rc.reporter_standard = $1
	ORDER BY rc.editions DESC, rc.cites DESC
	LIMIT $2`
	items, err := collect(ctx, s.db, query, []any{standard, limit}, func(rows pgx.Rows) (TopCase, error) {
		var t TopCase
		var source, id, name, cite *string
		var year *int
		err := rows.Scan(&t.Editions, &t.Cites, &source, &id, &name, &year, &cite)
		if ref := caseRefFrom(source, id, name, year, cite); ref != nil {
			t.CaseRef = *ref
		}
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("reporter top cases: %w", err)
	}
	return items, nil
}

// getReporterTopEditions lists the treatise editions that cite the reporter
// most.
func (s *server) getReporterTopEditions(ctx context.Context, standard string, limit int) ([]ReporterEdition, error) {
	items, err := collect(ctx, s.db, `
		SELECT erc.bibliographicid, ec.title, ec.year, ec.jurisdiction, ec.work_id, w.title, erc.cites, erc.linked
		FROM moml_citations.edition_reporter_citations erc
		JOIN moml_citations.edition_citation_counts ec ON ec.bibliographicid = erc.bibliographicid
		JOIN moml.works w ON w.work_id = ec.work_id
		WHERE erc.reporter_standard = $1
		ORDER BY erc.cites DESC, ec.year
		LIMIT $2`, []any{standard, limit},
		func(rows pgx.Rows) (ReporterEdition, error) {
			var e ReporterEdition
			return e, rows.Scan(&e.BiblioID, &e.Title, &e.Year, &e.Jurisdiction, &e.WorkID, &e.WorkTitle, &e.Cites, &e.Linked)
		})
	if err != nil {
		return nil, fmt.Errorf("reporter top editions: %w", err)
	}
	return items, nil
}

// getReporterUnmatched lists the cite strings of the reporter that recur most
// among the citations the linker could not match.
func (s *server) getReporterUnmatched(ctx context.Context, standard string, limit int) ([]UnmatchedCite, error) {
	items, err := collect(ctx, s.db, `
		SELECT volume, page, n FROM moml_citations.citations_unmatched_top
		WHERE reporter_standard = $1 ORDER BY n DESC, volume, page LIMIT $2`, []any{standard, limit},
		func(rows pgx.Rows) (UnmatchedCite, error) {
			var u UnmatchedCite
			err := rows.Scan(&u.Volume, &u.Page, &u.N)
			v := url.Values{"reporter": {standard}, "page": {strconv.Itoa(u.Page)}}
			if u.Volume != nil {
				v.Set("volume", strconv.Itoa(*u.Volume))
			}
			u.URL = citationsURL(v)
			return u, err
		})
	if err != nil {
		return nil, fmt.Errorf("reporter unmatched cites: %w", err)
	}
	return items, nil
}

func (s *server) handleReporter(w http.ResponseWriter, r *http.Request) {
	standard := r.PathValue("standard")

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	d, err := s.getReporter(ctx, standard)
	if err != nil {
		s.serverError(w, r, err, "loading the reporter", "reporter", standard)
		return
	}
	if d == nil {
		s.notFound(w, r, "There is no reporter with the standard abbreviation "+standard+".")
		return
	}
	d.Page = Page{
		Title:   d.Standard,
		Section: "reporters",
		Crumbs:  []Crumb{{Label: "Reporters", URL: "/reporters"}, {Label: d.Standard}},
	}

	if err := optional(s.getReporterTotals(ctx, d), &d.Page, "moml_citations.linking_dashboard_reporters"); err != nil {
		s.serverError(w, r, err, "loading the reporter's totals", "reporter", standard)
		return
	}
	if d.Spellings, err = s.getReporterSpellings(ctx, standard); err != nil {
		if !isUnavailable(err) {
			s.serverError(w, r, err, "loading the reporter's spellings", "reporter", standard)
			return
		}
		d.Page.unavailable("legalhist.top_reporters")
	}
	if d.AltAbbrs, err = s.getReporterAltAbbrs(ctx, standard); err != nil {
		s.serverError(w, r, err, "loading the reporter's alternate abbreviations", "reporter", standard)
		return
	}
	if d.DiffVols, err = s.getReporterDiffVols(ctx, standard); err != nil {
		s.serverError(w, r, err, "loading the reporter's volume renumbering", "reporter", standard)
		return
	}

	outcomes, err := s.getReporterOutcomes(ctx, standard)
	if err := optional(err, &d.Page, "moml_citations.linking_dashboard_tiers"); err != nil {
		s.serverError(w, r, err, "loading the reporter's linking outcomes", "reporter", standard)
		return
	}
	d.Outcomes = outcomes

	top, err := s.getReporterTopCases(ctx, standard, 25)
	if err := optional(err, &d.Page, "moml_citations.reporter_case_citations"); err != nil {
		s.serverError(w, r, err, "ranking the reporter's cases", "reporter", standard)
		return
	}
	d.TopCases = TopCasesView{Rows: top, ShowEditions: true, Empty: "No linked citations reach a case through this reporter."}

	editions, err := s.getReporterTopEditions(ctx, standard, 25)
	if err := optional(err, &d.Page, "moml_citations.edition_reporter_citations"); err != nil {
		s.serverError(w, r, err, "ranking the reporter's citing editions", "reporter", standard)
		return
	}
	d.TopEditions = editions

	unmatched, err := s.getReporterUnmatched(ctx, standard, 50)
	if err := optional(err, &d.Page, "moml_citations.citations_unmatched_top"); err != nil {
		s.serverError(w, r, err, "listing the reporter's unmatched cites", "reporter", standard)
		return
	}
	d.Unmatched = unmatched

	s.render(w, r, "reporter.html", http.StatusOK, d)
}
