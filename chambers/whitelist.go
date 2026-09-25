package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/agnivade/levenshtein"
	"github.com/jackc/pgx/v4"
)

// The whitelist extender walks through the reporter spellings the detector
// found most often that the whitelist does not know, and offers the closest
// reporter standards for each, producing CSVs for the seed migrations. It
// writes nothing itself.

// UnwhitelistedReporter is a reporter abbreviation not found in the whitelist,
// with a count of how many citations reference it and potential matches.
type UnwhitelistedReporter struct {
	ReporterAbbr string          `json:"reporterAbbr"`
	Count        int             `json:"count"`
	Matches      []ReporterMatch `json:"matches"`
}

// ReporterMatch is a potential reporter_standard match with its CAP info.
type ReporterMatch struct {
	Standard    string `json:"standard"`
	ReporterCap string `json:"reporterCap"`
	Score       int    `json:"score"`
}

// getUnwhitelistedReporters returns the 250 spellings with the most citations
// that the whitelist lacks, from the legalhist.top_reporters view of spelling
// counts.
func (s *server) getUnwhitelistedReporters(ctx context.Context) ([]UnwhitelistedReporter, error) {
	slog.Debug("querying unwhitelisted reporters")
	items, err := collect(ctx, s.db, `
		SELECT t.reporter_abbr, t.n
		FROM legalhist.top_reporters t
		LEFT JOIN legalhist.whitelist wl ON wl.reporter_found = t.reporter_abbr
		WHERE wl.reporter_found IS NULL
		ORDER BY t.n DESC
		LIMIT 250`, nil,
		func(rows pgx.Rows) (UnwhitelistedReporter, error) {
			var r UnwhitelistedReporter
			return r, rows.Scan(&r.ReporterAbbr, &r.Count)
		})
	if err != nil {
		return nil, fmt.Errorf("unwhitelisted reporters: %w", err)
	}
	return items, nil
}

// getDistinctReporterStandards returns the canonical list of reporter_standard
// values from legalhist.reporters.
func (s *server) getDistinctReporterStandards(ctx context.Context) ([]string, error) {
	items, err := collect(ctx, s.db, `SELECT reporter_standard FROM legalhist.reporters ORDER BY reporter_standard`, nil,
		func(rows pgx.Rows) (string, error) {
			var v string
			return v, rows.Scan(&v)
		})
	if err != nil {
		return nil, fmt.Errorf("reporter standards: %w", err)
	}
	return items, nil
}

// getCapInfoMap returns a map of reporter_standard → reporter_cap for standards
// that have a non-empty reporter_cap value.
func (s *server) getCapInfoMap(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT reporter_standard, reporter_cap FROM legalhist.reporters
		WHERE reporter_cap IS NOT NULL AND reporter_cap != ''`)
	if err != nil {
		return nil, fmt.Errorf("querying cap info: %w", err)
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var std, cap string
		if err := rows.Scan(&std, &cap); err != nil {
			return nil, fmt.Errorf("scanning cap info: %w", err)
		}
		m[std] = cap
	}
	return m, rows.Err()
}

// normalizeReporter strips periods, commas, and extra whitespace, then lowercases.
func normalizeReporter(s string) string {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// computeMatches finds the best reporter_standard matches for an abbreviation
// using Levenshtein distance on normalized forms.
func computeMatches(abbr string, standards []string, capMap map[string]string) []ReporterMatch {
	normAbbr := normalizeReporter(abbr)
	if normAbbr == "" {
		return nil
	}

	type scored struct {
		standard string
		score    int
	}

	var candidates []scored
	for _, std := range standards {
		normStd := normalizeReporter(std)
		if normStd == "" {
			continue
		}

		var score int
		if normAbbr == normStd {
			score = 100
		} else if strings.HasPrefix(normAbbr, normStd) || strings.HasPrefix(normStd, normAbbr) {
			score = 90
		} else {
			dist := levenshtein.ComputeDistance(normAbbr, normStd)
			maxLen := max(len(normAbbr), len(normStd))
			score = int((1.0 - float64(dist)/float64(maxLen)) * 100)
		}

		if score >= 30 {
			candidates = append(candidates, scored{standard: std, score: score})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].standard < candidates[j].standard
	})

	if len(candidates) > 20 {
		candidates = candidates[:20]
	}

	matches := make([]ReporterMatch, len(candidates))
	for i, c := range candidates {
		m := ReporterMatch{Standard: c.standard, Score: c.score}
		if cap, ok := capMap[c.standard]; ok {
			m.ReporterCap = cap
		}
		matches[i] = m
	}
	return matches
}

func (s *server) handleWhitelist(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "linking_whitelist.html", http.StatusOK, s.linkingPage("Whitelist extender", "Whitelist extender"))
}

func (s *server) handleWhitelistAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r, 2*time.Minute)
	defer cancel()

	reporters, err := s.getUnwhitelistedReporters(ctx)
	if err != nil {
		slog.Error("error querying unwhitelisted reporters", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	standards, err := s.getDistinctReporterStandards(ctx)
	if err != nil {
		slog.Error("error querying reporter standards", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	capMap, err := s.getCapInfoMap(ctx)
	if err != nil {
		slog.Error("error querying cap info", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	for i := range reporters {
		reporters[i].Matches = computeMatches(reporters[i].ReporterAbbr, standards, capMap)
	}
	if reporters == nil {
		reporters = []UnwhitelistedReporter{}
	}
	writeJSON(w, reporters)
}
