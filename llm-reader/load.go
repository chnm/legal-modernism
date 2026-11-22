package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// recordFiles lists every record under the results directory, in order.
func recordFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// checkTargets reports the records' targets that are missing from the
// database, which would make the load fail on a foreign key.
func checkTargets(ctx context.Context, pool *pgxpool.Pool, recs []*PageRecord) ([]string, error) {
	var problems []string
	for _, r := range recs {
		var page int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM moml.page WHERE psmid = $1 AND pageid = $2`,
			r.Psmid, r.Pageid).Scan(&page); err != nil {
			return nil, err
		}
		if page == 0 {
			problems = append(problems, fmt.Sprintf("%s/%s: no such page in moml.page", r.Psmid, r.Pageid))
		}
		for _, c := range r.Citations {
			var q string
			var arg any
			switch {
			case c.CapCaseID != nil:
				q, arg = `SELECT count(*) FROM cap.cases WHERE id = $1`, *c.CapCaseID
			case c.ErCaseID != "":
				q, arg = `SELECT count(*) FROM english_reports.cases WHERE id = $1`, c.ErCaseID
			case c.CodeReporterID != nil:
				q, arg = `SELECT count(*) FROM legalhist.code_reporter WHERE id = $1`, *c.CodeReporterID
			case c.StubCite != "":
				q, arg = `SELECT count(*) FROM legalhist.stub_cases WHERE cite = $1`, c.StubCite
			default:
				continue
			}
			var n int
			if err := pool.QueryRow(ctx, q, arg).Scan(&n); err != nil {
				return nil, err
			}
			if n == 0 {
				problems = append(problems, fmt.Sprintf("%s/%s citation %d: target %v (%s) not found", r.Psmid, r.Pageid, c.Seq, arg, c.Status))
			}
		}
	}
	return problems, nil
}

// load replaces each page's rows with its record, one transaction per volume,
// so that loading again after more pages are read, or after a page is reread,
// is safe.
func load(ctx context.Context, pool *pgxpool.Pool, recs []*PageRecord) error {
	byVol := map[string][]*PageRecord{}
	var vols []string
	for _, r := range recs {
		if _, ok := byVol[r.Psmid]; !ok {
			vols = append(vols, r.Psmid)
		}
		byVol[r.Psmid] = append(byVol[r.Psmid], r)
	}
	pagesLoaded, linksLoaded := 0, 0
	for _, v := range vols {
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			b := &pgx.Batch{}
			for _, r := range byVol[v] {
				b.Queue(`DELETE FROM moml_citations.llm_pages WHERE psmid = $1 AND pageid = $2`, r.Psmid, r.Pageid)
				b.Queue(`INSERT INTO moml_citations.llm_pages (psmid, pageid, model, read_at, citation_count, note)
				         VALUES ($1, $2, $3, $4, $5, nullif($6, ''))`,
					r.Psmid, r.Pageid, r.Model, r.ReadAt, len(r.Citations), r.Note)
				for _, c := range r.Citations {
					b.Queue(`INSERT INTO moml_citations.llm_links
					           (psmid, pageid, seq, raw, case_name, volume, reporter_abbr, page, year, status,
					            cap_case_id, er_case_id, code_reporter_id, stub_cite, pincite, confidence, note)
					         VALUES ($1, $2, $3, $4, nullif($5, ''), $6, nullif($7, ''), $8, $9, $10,
					                 $11, nullif($12, ''), $13, nullif($14, ''), $15, $16, nullif($17, ''))`,
						r.Psmid, r.Pageid, c.Seq, c.Raw, c.CaseName, c.Volume, c.ReporterAbbr, c.Page, c.Year, c.Status,
						c.CapCaseID, c.ErCaseID, c.CodeReporterID, c.StubCite, c.Pincite, c.Confidence, c.Note)
					linksLoaded++
				}
				pagesLoaded++
			}
			return tx.SendBatch(ctx, b).Close()
		})
		if err != nil {
			return fmt.Errorf("loading volume %s: %w", v, err)
		}
		slog.Debug("loaded volume", "psmid", v, "pages", len(byVol[v]))
	}
	slog.Info("loaded records", "pages", pagesLoaded, "links", linksLoaded)
	return nil
}

// readAll reads and checks every record, failing on the first bad one only
// after reporting all of them.
func readAll(dir string) ([]*PageRecord, error) {
	files, err := recordFiles(dir)
	if err != nil {
		return nil, err
	}
	var recs []*PageRecord
	bad := 0
	for _, f := range files {
		r, err := readRecord(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			bad++
			continue
		}
		for _, e := range r.check(f) {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f, e)
			bad++
		}
		recs = append(recs, r)
	}
	if bad > 0 {
		return nil, fmt.Errorf("%d problems in %d records", bad, len(files))
	}
	return recs, nil
}
