// Command llm-reader supports reading MOML pages for citations with Claude
// (issue #439). Claude reads each page itself and writes what it finds as JSON
// under llm-reader/results; this program lists the pages, prints a page's
// text, checks the records, shows progress, and loads the records into
// moml_citations.llm_pages and llm_links. See llm-reader/README.md.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/lmullen/legal-modernism/go/db"
	flag "github.com/spf13/pflag"
)

const (
	defaultManifest = "llm-reader/manifests/work-1107.tsv"
	defaultResults  = "llm-reader/results"
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: llm-reader <command> [flags]

commands:
  manifest  list a work's pages in reading order (needs the database)
  todo      list the next pages without a record
  page      print a page's OCR text (needs the database)
  validate  check the records, and with --db that their targets exist
  serve     show reading progress as a bar
  load      load the records into the database (needs write access: LAW_DBSTR)
`)
	os.Exit(2)
}

func main() {
	level := slog.LevelInfo
	if d := os.Getenv("LAW_DEBUG"); d == "debug" || d == "true" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	manifest := fs.String("manifest", defaultManifest, "manifest of pages to read")
	results := fs.String("results", defaultResults, "directory of page records")
	ctx := context.Background()

	var err error
	switch cmd {
	case "manifest":
		work := fs.Int("work", 1107, "moml.works id")
		fs.Parse(args)
		err = cmdManifest(ctx, *work, *manifest)
	case "todo":
		limit := fs.Int("limit", 25, "number of pages to list")
		jur := fs.String("jurisdiction", "", "only this jurisdiction (US or UK)")
		edition := fs.String("edition", "", "only this edition (bibliographicid)")
		fs.Parse(args)
		err = cmdTodo(*manifest, *results, *jur, *edition, *limit)
	case "page":
		fs.Parse(args)
		if fs.NArg() != 2 {
			fmt.Fprintln(os.Stderr, "usage: llm-reader page <psmid> <pageid>")
			os.Exit(2)
		}
		err = cmdPage(ctx, fs.Arg(0), fs.Arg(1))
	case "validate":
		withDB := fs.Bool("db", false, "also check that every target exists in the database")
		fs.Parse(args)
		err = cmdValidate(ctx, *results, *withDB)
	case "serve":
		port := fs.Int("port", 4314, "port to listen on")
		title := fs.String("title", "Reading Stephen on Pleading", "page heading")
		jur := fs.String("jurisdiction", "", "count only these jurisdictions, comma separated (default all)")
		fs.Parse(args)
		js := map[string]bool{}
		for _, j := range strings.Split(*jur, ",") {
			if j != "" {
				js[j] = true
			}
		}
		err = serve(*manifest, *results, *title, js, *port)
	case "load":
		fs.Parse(args)
		err = cmdLoad(ctx, *results)
	default:
		usage()
	}
	if err != nil {
		slog.Error(cmd+" failed", "error", err)
		os.Exit(1)
	}
}

func cmdManifest(ctx context.Context, work int, path string) error {
	pool, err := db.Connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	pages, err := buildManifest(ctx, pool, work)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := writeManifest(f, pages); err != nil {
		return err
	}
	slog.Info("wrote manifest", "path", path, "pages", len(pages))
	return nil
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}

func cmdTodo(manifest, results, jur, edition string, limit int) error {
	pages, err := readManifest(manifest)
	if err != nil {
		return err
	}
	n := 0
	for _, p := range pages {
		if (jur != "" && p.Jurisdiction != jur) || (edition != "" && p.Bibliographicid != edition) {
			continue
		}
		if done(results, p) {
			continue
		}
		fmt.Printf("%s\t%s\t%d\t%s\t%s\t%s\t%d\n", p.Psmid, p.Pageid, p.Year, p.Bibliographicid, p.Sourcepage, p.Type, p.Chars)
		n++
		if n >= limit {
			break
		}
	}
	return nil
}

func cmdPage(ctx context.Context, psmid, pageid string) error {
	pool, err := db.Connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	var year int
	var bib, sourcepage, typ, text string
	err = pool.QueryRow(ctx, `
		SELECT coalesce(v.year, 0), v.bibliographicid, coalesce(p.sourcepage, ''), coalesce(p.type, ''), coalesce(o.ocrtext, '')
		  FROM moml.page p JOIN moml.volumes v USING (psmid)
		  LEFT JOIN moml.page_ocrtext o USING (psmid, pageid)
		 WHERE p.psmid = $1 AND p.pageid = $2`, psmid, pageid).Scan(&year, &bib, &sourcepage, &typ, &text)
	if err != nil {
		return fmt.Errorf("reading page %s/%s: %w", psmid, pageid, err)
	}
	fmt.Printf("== %s/%s  edition %s (%d)  printed page %q  type %s\n\n%s\n", psmid, pageid, bib, year, sourcepage, typ, strings.TrimSpace(text))
	return nil
}

func cmdValidate(ctx context.Context, results string, withDB bool) error {
	recs, err := readAll(results)
	if err != nil {
		return err
	}
	links := 0
	for _, r := range recs {
		links += len(r.Citations)
	}
	if withDB {
		pool, err := db.Connect(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()
		problems, err := checkTargets(ctx, pool, recs)
		if err != nil {
			return err
		}
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, p)
		}
		if len(problems) > 0 {
			return fmt.Errorf("%d targets not found", len(problems))
		}
	}
	slog.Info("records are valid", "pages", len(recs), "citations", links, "checked_db", withDB)
	return nil
}

func cmdLoad(ctx context.Context, results string) error {
	recs, err := readAll(results)
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	return load(ctx, pool, recs)
}
