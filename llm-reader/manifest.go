package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Page is one line of the manifest: a page to be read, in reading order.
type Page struct {
	Seq             int
	Jurisdiction    string
	Bibliographicid string
	Year            int
	Psmid           string
	Pageid          string
	Sourcepage      string
	Type            string
	Chars           int
}

var manifestHeader = []string{"seq", "jurisdiction", "bibliographicid", "year", "psmid", "pageid", "sourcepage", "type", "chars"}

// buildManifest lists every page of a work's editions, U.S. editions first,
// each edition in order of its year and each volume's pages in order.
func buildManifest(ctx context.Context, pool *pgxpool.Pool, workID int) ([]Page, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.jurisdiction, v.bibliographicid, coalesce(v.year, 0), p.psmid, p.pageid,
		       coalesce(p.sourcepage, ''), coalesce(p.type, ''), coalesce(length(o.ocrtext), 0)
		  FROM moml.editions e
		  JOIN moml.treatises t USING (bibliographicid)
		  JOIN moml.volumes v USING (bibliographicid)
		  JOIN moml.page p USING (psmid)
		  LEFT JOIN moml.page_ocrtext o USING (psmid, pageid)
		 WHERE e.work_id = $1
		 ORDER BY t.jurisdiction = 'UK', min(v.year) OVER (PARTITION BY v.bibliographicid),
		          v.bibliographicid, v.current_volume, p.psmid, p.pageid`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying pages of work %d: %w", workID, err)
	}
	defer rows.Close()
	var pages []Page
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.Jurisdiction, &p.Bibliographicid, &p.Year, &p.Psmid, &p.Pageid,
			&p.Sourcepage, &p.Type, &p.Chars); err != nil {
			return nil, err
		}
		p.Seq = len(pages) + 1
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

func writeManifest(w io.Writer, pages []Page) error {
	cw := csv.NewWriter(w)
	cw.Comma = '\t'
	if err := cw.Write(manifestHeader); err != nil {
		return err
	}
	for _, p := range pages {
		if err := cw.Write([]string{strconv.Itoa(p.Seq), p.Jurisdiction, p.Bibliographicid,
			strconv.Itoa(p.Year), p.Psmid, p.Pageid, p.Sourcepage, p.Type, strconv.Itoa(p.Chars)}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func readManifest(path string) ([]Page, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.Comma = '\t'
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}
	var pages []Page
	for _, r := range recs[1:] {
		seq, _ := strconv.Atoi(r[0])
		year, _ := strconv.Atoi(r[3])
		chars, _ := strconv.Atoi(r[8])
		pages = append(pages, Page{seq, r[1], r[2], year, r[4], r[5], r[6], r[7], chars})
	}
	return pages, nil
}

// done reports whether a page has a record.
func done(dir string, p Page) bool {
	_, err := os.Stat(resultPath(dir, p.Psmid, p.Pageid))
	return err == nil
}
