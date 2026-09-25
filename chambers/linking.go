package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"
)

// Detecting and linking: the dashboard of how the linker fared over the whole
// corpus, and the match tiers charted. Both pages fetch their data as JSON and
// draw it with Observable Plot; the vocabulary of statuses and tiers is handed
// to the page from vocabulary.go.

// linkingPage is the data of the two chart pages.
type linkingPage struct {
	Page
	Tiers        []TierInfo
	StatusOrder  []string
	StatusLabels map[string]string
}

func (s *server) linkingPage(title, subsection string) linkingPage {
	return linkingPage{
		Page:         Page{Title: title, Section: "linking", Crumbs: []Crumb{{Label: "Detecting & linking", URL: "/linking"}, {Label: subsection}}},
		Tiers:        tierVocabulary,
		StatusOrder:  statusOrder,
		StatusLabels: statusLabels,
	}
}

func (s *server) handleLinking(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "linking.html", http.StatusOK, s.linkingPage("Linking dashboard", "Dashboard"))
}

func (s *server) handleLinkingTiers(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "linking_tiers.html", http.StatusOK, s.linkingPage("Match tiers", "Match tiers"))
}

// ReporterStats holds linked and no-match counts for a single reporter_standard.
type ReporterStats struct {
	Reporter    string `json:"reporter"`
	Linked      int    `json:"linked"`
	NoMatch     int    `json:"noMatch"`
	Unprocessed int    `json:"unprocessed"`
	UK          bool   `json:"uk"`
}

// DashboardData holds aggregated linking status data for the dashboard.
type DashboardData struct {
	LinkedCAP            int
	LinkedEnglishReports int
	LinkedCodeReporter   int
	// LinkedStub counts links to legalhist.stub_cases (issue #248). It is part
	// of TotalLinked: the dashboard measures citations found, and a stub link
	// is one, with match_tier splitting it out where the distinction matters.
	LinkedStub            int
	SkippedNotWhiteListed int
	NoMatch               int
	SkippedJunk           int
	SkippedStatute        int
	TotalRawCites         int
	// Unprocessed is the citations detected but not yet linked. It is normally
	// zero; anything else means the linker has not finished, and every
	// percentage on the dashboard is being computed over a partial run
	// (issue #296).
	Unprocessed   int
	Reporters     []ReporterStats    `json:"Reporters,omitempty"`
	Tiers         []TierStat         `json:"Tiers,omitempty"`
	ReporterTiers []ReporterTierStat `json:"ReporterTiers,omitempty"`
}

// TotalLinked returns the sum of all linked statuses.
func (d *DashboardData) TotalLinked() int {
	return d.LinkedCAP + d.LinkedEnglishReports + d.LinkedCodeReporter + d.LinkedStub
}

// loadReporterStats fills in d.Reporters from the per-reporter materialized
// view, ordered by total citations descending (linked + no_match + unprocessed).
func (s *server) loadReporterStats(ctx context.Context, d *DashboardData) error {
	rows, err := s.db.Query(ctx, `
		SELECT reporter_standard, linked, no_match, unprocessed, uk
		FROM moml_citations.linking_dashboard_reporters
		ORDER BY linked + no_match + unprocessed DESC
	`)
	if err != nil {
		return fmt.Errorf("querying reporter stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r ReporterStats
		if err := rows.Scan(&r.Reporter, &r.Linked, &r.NoMatch, &r.Unprocessed, &r.UK); err != nil {
			return fmt.Errorf("scanning reporter stats: %w", err)
		}
		d.Reporters = append(d.Reporters, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating reporter stats: %w", err)
	}
	slog.Debug("fetched reporter stats", "count", len(d.Reporters))
	return nil
}

func (s *server) getDashboardData(ctx context.Context) (*DashboardData, error) {
	d := &DashboardData{}

	// Get summary metrics (total raw cites, per-status counts, and how many
	// citations are not yet linked at all) from the precomputed materialized
	// view. The view is refreshed by db/maintenance.sh, not by the linker;
	// reading it here is a small indexed scan instead of aggregating the ~62M-row
	// citations_unlinked and citation_links tables on every request.
	slog.Debug("querying linking dashboard summary view")
	rows, err := s.db.Query(ctx, `SELECT metric, n FROM moml_citations.linking_dashboard_summary`)
	if err != nil {
		return nil, fmt.Errorf("querying dashboard summary: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var metric string
		var n int
		if err := rows.Scan(&metric, &n); err != nil {
			return nil, fmt.Errorf("scanning dashboard summary: %w", err)
		}
		switch metric {
		case "total_raw_cites":
			d.TotalRawCites = n
		case "linked_cap":
			d.LinkedCAP = n
		case "linked_english_reports":
			d.LinkedEnglishReports = n
		case "linked_code_reporter":
			d.LinkedCodeReporter = n
		case "linked_stub":
			d.LinkedStub = n
		case "skipped_not_whitelisted":
			d.SkippedNotWhiteListed = n
		case "no_match":
			d.NoMatch = n
		case "skipped_junk":
			d.SkippedJunk = n
		case "skipped_statute":
			d.SkippedStatute = n
		case "unprocessed":
			d.Unprocessed = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating dashboard summary: %w", err)
	}

	// The three sections below each read their own materialized view, and a
	// failure in any of them is not fatal. A view that a migration has just
	// recreated stays unpopulated until the next make db-maintenance and errors
	// when queried; the summary above is the substance of the page, so a section
	// that cannot be read degrades to empty rather than taking the page down.
	if err := s.loadReporterStats(ctx, d); err != nil {
		slog.Warn("reporter stats unavailable, rendering dashboard without them", "error", err)
	}
	if d.Tiers, err = s.getTierSummary(ctx); err != nil {
		slog.Warn("tier summary unavailable, rendering dashboard without it", "error", err)
	}
	if d.ReporterTiers, err = s.getReporterTiers(ctx); err != nil {
		slog.Warn("reporter tiers unavailable, rendering dashboard without them", "error", err)
	}

	slog.Debug("dashboard data complete", "total_raw_cites", d.TotalRawCites, "total_linked", d.TotalLinked(),
		"reporters", len(d.Reporters), "tiers", len(d.Tiers), "reporter_tiers", len(d.ReporterTiers))
	return d, nil
}

// TierStat is one row of moml_citations.linking_tier_summary: a linking status
// paired with the match_tier that says how far the linker got, its count, and
// its share of that status. Tier is empty for the statuses that never reach a
// probe and so carry no tier.
type TierStat struct {
	Status      string  `json:"status"`
	Tier        string  `json:"tier"`
	N           int     `json:"n"`
	PctOfStatus float64 `json:"pctOfStatus"`
}

// getTierSummary reads the corpus-wide status × match_tier breakdown. The view
// aggregates moml_citations.linking_dashboard_tiers, which joins the whitelist,
// so it covers exactly the citations the linker probes — linked, no_match,
// skipped_statute, and unprocessed — and omits skipped_junk and
// skipped_not_whitelisted, which are turned away before any target is consulted.
func (s *server) getTierSummary(ctx context.Context) ([]TierStat, error) {
	slog.Debug("querying linking tier summary view")
	rows, err := s.db.Query(ctx, `
		SELECT status, match_tier, n, pct_of_status
		FROM moml_citations.linking_tier_summary
		ORDER BY n DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying tier summary: %w", err)
	}
	defer rows.Close()

	var results []TierStat
	for rows.Next() {
		var t TierStat
		var tier *string
		if err := rows.Scan(&t.Status, &tier, &t.N, &t.PctOfStatus); err != nil {
			return nil, fmt.Errorf("scanning tier summary: %w", err)
		}
		if tier != nil {
			t.Tier = *tier
		}
		results = append(results, t)
	}
	slog.Debug("fetched tier summary", "count", len(results))
	return results, rows.Err()
}

// ReporterTierStat holds one reporter_standard's no_match citations broken down
// by the tier that says where the match failed. Tiers is keyed by match_tier.
type ReporterTierStat struct {
	Reporter string         `json:"reporter"`
	NoMatch  int            `json:"noMatch"`
	Tiers    map[string]int `json:"tiers"`
}

// getReporterTiers reads the failure tiers of every reporter's no_match pool and
// pivots them into one row per reporter, so the page can compare the shape of a
// reporter's failures — mostly volume_absent, mostly page_absent — rather than
// only their total.
func (s *server) getReporterTiers(ctx context.Context) ([]ReporterTierStat, error) {
	slog.Debug("querying per-reporter failure tiers")
	rows, err := s.db.Query(ctx, `
		SELECT reporter_standard, match_tier, n
		FROM moml_citations.linking_dashboard_tiers
		WHERE status = 'no_match' AND match_tier IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("querying reporter tiers: %w", err)
	}
	defer rows.Close()

	byReporter := make(map[string]*ReporterTierStat)
	for rows.Next() {
		var reporter, tier string
		var n int
		if err := rows.Scan(&reporter, &tier, &n); err != nil {
			return nil, fmt.Errorf("scanning reporter tier: %w", err)
		}
		r, ok := byReporter[reporter]
		if !ok {
			r = &ReporterTierStat{Reporter: reporter, Tiers: make(map[string]int)}
			byReporter[reporter] = r
		}
		r.Tiers[tier] += n
		r.NoMatch += n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating reporter tiers: %w", err)
	}

	results := make([]ReporterTierStat, 0, len(byReporter))
	for _, r := range byReporter {
		results = append(results, *r)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].NoMatch != results[j].NoMatch {
			return results[i].NoMatch > results[j].NoMatch
		}
		return results[i].Reporter < results[j].Reporter
	})
	slog.Debug("fetched reporter tiers", "count", len(results))
	return results, nil
}

// ReporterTierRow is one (reporter, status, tier) cell of
// moml_citations.linking_dashboard_tiers in the tidy shape the tiers page
// stacks into a bar per reporter. Tier is empty for the statuses that never
// reach a probe and so carry no tier.
type ReporterTierRow struct {
	Reporter string `json:"reporter"`
	Status   string `json:"status"`
	Tier     string `json:"tier"`
	N        int    `json:"n"`
}

// TiersData is everything the tiers page shows: the corpus-wide status ×
// match_tier breakdown, and the same breakdown for every reporter.
type TiersData struct {
	Tiers     []TierStat        `json:"tiers"`
	Reporters []ReporterTierRow `json:"reporters"`
}

// getReporterTierRows reads every reporter's citations grouped by status and
// match tier. Unlike getReporterTiers, which keeps only the no_match pool for
// the dashboard's failure table, this covers every status the view holds, so a
// reporter's bar shows its links and its failures side by side. Rows come back
// with the heaviest reporter first and each reporter's cells together, largest
// first, which is the order the page draws them in.
func (s *server) getReporterTierRows(ctx context.Context) ([]ReporterTierRow, error) {
	slog.Debug("querying per-reporter tier rows")
	rows, err := s.db.Query(ctx, `
		SELECT reporter_standard, COALESCE(status, 'unprocessed'), COALESCE(match_tier, ''), n
		FROM moml_citations.linking_dashboard_tiers
		ORDER BY sum(n) OVER (PARTITION BY reporter_standard) DESC, reporter_standard, n DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying reporter tier rows: %w", err)
	}
	defer rows.Close()

	var results []ReporterTierRow
	for rows.Next() {
		var r ReporterTierRow
		if err := rows.Scan(&r.Reporter, &r.Status, &r.Tier, &r.N); err != nil {
			return nil, fmt.Errorf("scanning reporter tier row: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating reporter tier rows: %w", err)
	}
	slog.Debug("fetched reporter tier rows", "count", len(results))
	return results, nil
}

// getTiersData assembles the tiers page from linking_tier_summary and
// linking_dashboard_tiers. Both read the same materialized view, so unlike the
// dashboard's optional sections there is no partial page worth rendering: if
// the view is unpopulated, the page has nothing to show.
func (s *server) getTiersData(ctx context.Context) (*TiersData, error) {
	tiers, err := s.getTierSummary(ctx)
	if err != nil {
		return nil, err
	}
	reporters, err := s.getReporterTierRows(ctx)
	if err != nil {
		return nil, err
	}
	return &TiersData{Tiers: tiers, Reporters: reporters}, nil
}

// writeJSON sends v as a JSON response cached for an hour, the refresh
// cadence of the views behind it.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Cache-Control", "max-age=3600")
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("error encoding JSON", "error", err)
	}
}

func (s *server) handleLinkingAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()

	data, err := s.getDashboardData(ctx)
	if err != nil {
		slog.Error("error querying dashboard data", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}

func (s *server) handleTiersAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()

	data, err := s.getTiersData(ctx)
	if err != nil {
		slog.Error("error querying tiers data", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}
