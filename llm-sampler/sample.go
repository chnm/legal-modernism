package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed system-detect.md
var systemDetect string

//go:embed system-link.md
var systemLink string

// weighted is a page that may be drawn, and whether the pipeline found a
// citation on it.
type weighted struct {
	Psmid, Pageid string
	Cited         bool
}

// pair is a row of two text columns.
type pair struct{ A, B string }

// The edges of the neighbouring pages a request carries, in characters.
const (
	tailChars = 1500
	headChars = 800
)

// pick returns a number in [0, n) fixed by the seed, the draw and a label.
func pick(seed string, draw int, label string, n int) int {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", seed, draw, label)))
	return int(binary.BigEndian.Uint64(h[:8]) % uint64(n))
}

// cmdSample draws the next pages and writes the batch with the first request
// for each page. An edition is drawn uniformly (a random official copy in
// moml.treatises with at least minCites pipeline citations), then a page among its body pages, a page on which the
// pipeline found a citation being citedWeight times as likely as one on which
// it found none. Each draw records its weight and probability within the
// edition, so that counts can be weighted back. The draws follow from the
// seed alone, so the sample can be rebuilt or continued in order. HathiTrust
// pages carry no type, so any page of a HathiTrust volume may be drawn.
func cmdSample(ctx context.Context, pool *pgxpool.Pool, dir, seed string, n, citedWeight, minCites int) error {
	batches, err := readBatches(dir)
	if err != nil {
		return err
	}
	draw, next := 0, 1
	seen := map[string]bool{}
	for _, b := range batches {
		if b.FinishedAt == nil {
			return fmt.Errorf("batch %d is not finished: run finish --batch %d first", b.Batch, b.Batch)
		}
		next = b.Batch + 1
		for _, p := range b.Pages {
			seen[p.id()] = true
			draw = max(draw, p.Draw)
		}
	}

	var editions []string
	rows, err := pool.Query(ctx, `SELECT bibliographicid FROM moml.treatises ORDER BY bibliographicid`)
	if err != nil {
		return fmt.Errorf("listing editions: %w", err)
	}
	if editions, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return fmt.Errorf("listing editions: %w", err)
	}

	batch := &Batch{Batch: next, StartedAt: time.Now().UTC(), CitedWeight: citedWeight, MinCites: minCites}
	bdir := batchDir(dir, next)
	for len(batch.Pages) < n {
		draw++
		bib := editions[pick(seed, draw, "edition", len(editions))]
		// An edition with too few pipeline citations is passed over: the view
		// of treatises still holds histories, political tracts and the like,
		// which cite no cases. Rejecting a draw leaves the editions that pass
		// equally likely.
		var cites int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM (
			  SELECT 1 FROM moml_citations.citations_unlinked u
			    JOIN moml_citations.citation_links l ON l.citation_id = u.id
			    JOIN moml.volumes v ON v.psmid = u.moml_treatise
			   WHERE v.bibliographicid = $1 AND l.status NOT LIKE 'skipped%' LIMIT $2) x`, bib, minCites).Scan(&cites); err != nil {
			return fmt.Errorf("counting citations of %s: %w", bib, err)
		}
		if cites < minCites {
			slog.Debug("edition has too few citations", "bibliographicid", bib, "draw", draw, "citations", cites)
			continue
		}

		// A page counts as cited when the detector found a citation on it
		// that the linker did not skip as junk, a statute or an unknown
		// spelling.
		rows, err := pool.Query(ctx, `
			SELECT p.psmid, p.pageid, EXISTS (
			         SELECT 1 FROM moml_citations.citations_unlinked u
			           JOIN moml_citations.citation_links l ON l.citation_id = u.id
			          WHERE u.moml_treatise = p.psmid AND u.moml_page = p.pageid AND l.status NOT LIKE 'skipped%')
			  FROM moml.page p JOIN moml.volumes v USING (psmid)
			 WHERE v.bibliographicid = $1 AND (p.type = 'bodyPage' OR p.psmid LIKE 'hathi%')
			 ORDER BY p.psmid, p.pageid`, bib)
		if err != nil {
			return fmt.Errorf("listing pages of %s: %w", bib, err)
		}
		pages, err := pgx.CollectRows(rows, pgx.RowToStructByPos[weighted])
		if err != nil {
			return fmt.Errorf("listing pages of %s: %w", bib, err)
		}
		if len(pages) == 0 {
			slog.Debug("edition has no body pages", "bibliographicid", bib, "draw", draw)
			continue
		}
		cited := 0
		for _, pg := range pages {
			if pg.Cited {
				cited++
			}
		}
		total := len(pages) + cited*(citedWeight-1)
		at := pick(seed, draw, "page", total)
		var pg weighted
		for _, pg = range pages {
			w := 1
			if pg.Cited {
				w = citedWeight
			}
			if at < w {
				break
			}
			at -= w
		}
		p := BatchPage{Draw: draw, Bibliographicid: bib, Psmid: pg.Psmid, Pageid: pg.Pageid,
			Weight: 1, EditionPages: len(pages), EditionCited: cited}
		if pg.Cited {
			p.Weight = citedWeight
		}
		p.Probability = float64(p.Weight) / float64(total)
		if seen[p.id()] {
			continue
		}
		seen[p.id()] = true
		req, err := detectRequest(ctx, pool, p)
		if err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(bdir, "detect", p.id()+".request.json"), req); err != nil {
			return err
		}
		batch.Pages = append(batch.Pages, p)
	}

	if err := writeSystemPrompts(ctx, pool, dir); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(bdir, "batch.json"), batch); err != nil {
		return err
	}
	slog.Info("drew batch", "batch", next, "pages", len(batch.Pages), "dir", bdir)
	fmt.Println(next)
	return nil
}

// writeSystemPrompts writes the two system prompts, the cached prefix of
// every request: the instructions, and for the first the table of reporters.
func writeSystemPrompts(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	rows, err := pool.Query(ctx, `
		SELECT reporter_standard || ' | ' || coalesce(reporter_title, '') || ' | ' || coalesce(jurisdiction, '') || ' | ' ||
		       coalesce(year_start::text, '') || '–' || coalesce(year_end::text, '')
		  FROM legalhist.reporters ORDER BY jurisdiction, reporter_standard`)
	if err != nil {
		return fmt.Errorf("listing reporters: %w", err)
	}
	lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("listing reporters: %w", err)
	}
	detect := systemDetect + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "system-detect.md"), []byte(detect), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "system-link.md"), []byte(systemLink), 0o644)
}

// detectRequest builds the first request for a page.
func detectRequest(ctx context.Context, pool *pgxpool.Pool, p BatchPage) (*DetectRequest, error) {
	req := &DetectRequest{CustomID: p.id()}
	ed, page, err := describe(ctx, pool, p)
	if err != nil {
		return nil, err
	}
	req.Edition, req.Page = *ed, *page

	// The page with its neighbours, in order.
	rows, err := pool.Query(ctx, `
		(SELECT pageid, coalesce(ocrtext, '') FROM moml.page_ocrtext WHERE psmid = $1 AND pageid < $2 ORDER BY pageid DESC LIMIT 1)
		UNION ALL
		(SELECT pageid, coalesce(ocrtext, '') FROM moml.page_ocrtext WHERE psmid = $1 AND pageid >= $2 ORDER BY pageid LIMIT 2)`,
		p.Psmid, p.Pageid)
	if err != nil {
		return nil, fmt.Errorf("reading text of %s: %w", p.id(), err)
	}
	texts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[pair])
	if err != nil {
		return nil, fmt.Errorf("reading text of %s: %w", p.id(), err)
	}
	for _, t := range texts {
		text := strings.TrimSpace(t.B)
		switch {
		case t.A == p.Pageid:
			req.Text = text
		case t.A < p.Pageid:
			if r := []rune(text); len(r) > tailChars {
				text = "…" + string(r[len(r)-tailChars:])
			}
			req.PreviousPageTail = text
		default:
			if r := []rune(text); len(r) > headChars {
				text = string(r[:headChars]) + "…"
			}
			req.NextPageHead = text
		}
	}

	if req.Pipeline, err = pipelineFor(ctx, pool, p.Psmid, p.Pageid, req.Text); err != nil {
		return nil, err
	}
	return req, nil
}

// describe returns what a request says about a page's edition and the page.
func describe(ctx context.Context, pool *pgxpool.Pool, p BatchPage) (*Edition, *PageRef, error) {
	ed := &Edition{Bibliographicid: p.Bibliographicid}
	page := &PageRef{Psmid: p.Psmid, Pageid: p.Pageid}
	err := pool.QueryRow(ctx, `
		SELECT v.full_title, coalesce(e.author, ''), coalesce(v.year, 0), t.jurisdiction,
		       coalesce(v.edition_statement, ''), coalesce(v.imprint, ''), v.current_volume,
		       coalesce(e.total_volumes, 0), coalesce(pg.sourcepage, ''),
		       coalesce((SELECT array_agg(s.subject ORDER BY s.position) FROM moml.edition_subjects s
		                  WHERE s.bibliographicid = e.bibliographicid), '{}')
		  FROM moml.page pg
		  JOIN moml.volumes v USING (psmid)
		  JOIN moml.editions e USING (bibliographicid)
		  JOIN moml.treatises t USING (bibliographicid)
		 WHERE pg.psmid = $1 AND pg.pageid = $2`, p.Psmid, p.Pageid).Scan(
		&ed.Title, &ed.Author, &ed.Year, &ed.Jurisdiction, &ed.EditionStatement, &ed.Imprint,
		&ed.Volume, &ed.Volumes, &page.PrintedPage, &ed.Subjects)
	if err != nil {
		return nil, nil, fmt.Errorf("describing %s: %w", p.id(), err)
	}
	return ed, page, nil
}

// pipelineFor returns what the detector found on a page and what the linker
// made of each citation, in the order the citations occur in the text.
func pipelineFor(ctx context.Context, pool *pgxpool.Pool, psmid, pageid, text string) ([]PipelineCitation, error) {
	rows, err := pool.Query(ctx, `
		SELECT u.raw, coalesce(u.case_name, ''), u.volume, u.reporter_abbr, u.page, u.year,
		       CASE WHEN w.reporter_found IS NULL THEN 'not a known spelling'
		            WHEN w.junk THEN 'junk'
		            ELSE w.reporter_standard || ': ' || coalesce(r.reporter_title, '') || ' (' || coalesce(r.jurisdiction, '') || ', ' ||
		                 coalesce(r.year_start::text, '') || '–' || coalesce(r.year_end::text, '') || ')' ||
		                 coalesce('; or ' || w.fallback_standard, '') END,
		       coalesce(l.status, 'not linked'), coalesce(l.match_tier, ''),
		       coalesce(l.cap_case_id::text, l.er_case_id, l.code_reporter_id::text, l.stub_cite, ''),
		       CASE WHEN l.cap_case_id IS NOT NULL THEN 'cap' WHEN l.er_case_id IS NOT NULL THEN 'english_reports'
		            WHEN l.code_reporter_id IS NOT NULL THEN 'code_reporter' WHEN l.stub_cite IS NOT NULL THEN 'stub' ELSE '' END,
		       coalesce(c.name_abbreviation || ' (' || c.decision_year || '), ' || l.cite_linked,
		                er.er_name || ' (' || er.er_year || '), ' || er.er_parallel_cite,
		                cr.name_abbreviation || ' (' || cr.decision_year || '), ' || cr.official_citation,
		                'stub ' || l.stub_cite, '')
		  FROM moml_citations.citations_unlinked u
		  LEFT JOIN moml_citations.citation_links l ON l.citation_id = u.id
		  LEFT JOIN legalhist.whitelist w ON w.reporter_found = u.reporter_abbr
		  LEFT JOIN legalhist.reporters r ON r.reporter_standard = w.reporter_standard
		  LEFT JOIN cap.cases c ON c.id = l.cap_case_id
		  LEFT JOIN english_reports.cases er ON er.id = l.er_case_id
		  LEFT JOIN legalhist.code_reporter cr ON cr.id = l.code_reporter_id
		 WHERE u.moml_treatise = $1 AND u.moml_page = $2`, psmid, pageid)
	if err != nil {
		return nil, fmt.Errorf("reading pipeline citations of %s/%s: %w", psmid, pageid, err)
	}
	defer rows.Close()
	out := []PipelineCitation{}
	for rows.Next() {
		var c PipelineCitation
		if err := rows.Scan(&c.Raw, &c.CaseName, &c.Volume, &c.ReporterAbbr, &c.Page, &c.Year,
			&c.ReporterReading, &c.Status, &c.Tier, &c.caseID, &c.source, &c.Linked); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	at := func(c PipelineCitation) int {
		if i := strings.Index(text, c.Raw); i >= 0 {
			return i
		}
		return len(text)
	}
	sort.SliceStable(out, func(i, j int) bool { return at(out[i]) < at(out[j]) })
	return out, nil
}

// loadBatch reads a batch's record.
func loadBatch(dir string, n int) (*Batch, error) {
	var b Batch
	if err := readJSON(filepath.Join(batchDir(dir, n), "batch.json"), &b); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no batch %d in %s", n, dir)
		}
		return nil, err
	}
	return &b, nil
}
