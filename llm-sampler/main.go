// Command llm-sampler reads random body pages of the MOML treatises for
// citations with Claude, as two stateless requests per page: one that detects
// the citations, one that links them to candidate cases code looked up in
// between (issue #442). It draws the pages, writes each request, checks each
// response, writes the final records under its data directory, and shows
// progress. See llm-sampler/README.md.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lmullen/legal-modernism/go/db"
	flag "github.com/spf13/pflag"
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: llm-sampler <command> [flags]

commands:
  sample      draw the next batch of pages and write its detect requests (needs the database)
  prompts     rewrite the system prompts in the data directory (needs the database)
  candidates  check a batch's detect responses and write its link requests (needs the database)
  finish      check a batch's link responses and write the final records
  serve       show progress
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
	dir := fs.String("dir", "tmp/llm-sampler", "data directory: batches, prompts and results")
	batch := fs.Int("batch", 0, "batch number")
	ctx := context.Background()

	connect := func() *pgxpool.Pool {
		pool, err := db.Connect(ctx)
		if err != nil {
			slog.Error("connecting to the database", "error", err)
			os.Exit(1)
		}
		return pool
	}

	var err error
	switch cmd {
	case "sample":
		n := fs.Int("n", 10, "pages in the batch")
		seed := fs.String("seed", "legal-modernism-474", "seed of the random draws")
		minCites := fs.Int("min-cites", 10, "fewest pipeline citations an edition must have to be drawn")
		weight := fs.Int("cited-weight", 5, "how many times likelier a page with a pipeline citation is than one without")
		fs.Parse(args)
		pool := connect()
		defer pool.Close()
		err = cmdSample(ctx, pool, *dir, *seed, *n, max(1, *weight), *minCites)
	case "prompts":
		fs.Parse(args)
		pool := connect()
		defer pool.Close()
		err = writeSystemPrompts(ctx, pool, *dir)
	case "candidates":
		fs.Parse(args)
		pool := connect()
		defer pool.Close()
		err = cmdCandidates(ctx, pool, *dir, *batch)
	case "finish":
		fs.Parse(args)
		err = cmdFinish(*dir, *batch)
	case "serve":
		port := fs.Int("port", 4313, "port to listen on")
		fs.Parse(args)
		err = serve(*dir, *port)
	default:
		usage()
	}
	if err != nil {
		slog.Error(cmd+" failed", "error", err)
		os.Exit(1)
	}
}

// cmdFinish checks the second responses of a batch against the candidates
// offered, writes the final record of each page, and closes the batch.
func cmdFinish(dir string, n int) error {
	batch, err := loadBatch(dir, n)
	if err != nil {
		return err
	}
	bdir := batchDir(dir, n)
	now := time.Now().UTC()
	var problems []string
	var records []*PageRecord
	for _, p := range batch.Pages {
		var dreq DetectRequest
		var dresp DetectResponse
		var lreq LinkRequest
		var lresp LinkResponse
		if err := readJSON(filepath.Join(bdir, "detect", p.id()+".request.json"), &dreq); err != nil {
			return err
		}
		if err := readJSON(filepath.Join(bdir, "detect", p.id()+".response.json"), &dresp); err != nil {
			return err
		}
		if err := readJSON(filepath.Join(bdir, "link", p.id()+".request.json"), &lreq); err != nil {
			return fmt.Errorf("run candidates --batch %d first: %w", n, err)
		}
		if err := readJSON(filepath.Join(bdir, "link", p.id()+".response.json"), &lresp); err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if errs := lresp.check(&lreq); len(errs) > 0 {
			problems = append(problems, errs...)
			continue
		}
		rec := &PageRecord{
			Psmid: p.Psmid, Pageid: p.Pageid, Bibliographicid: p.Bibliographicid,
			Title: dreq.Edition.Title, Year: dreq.Edition.Year, Jurisdiction: dreq.Edition.Jurisdiction,
			Batch: n, Draw: p.Draw, Model: Model, ReadAt: now, Note: dresp.Note,
			PipelineCount: len(dreq.Pipeline), PipelineRejected: dresp.PipelineRejected,
			Citations: []Citation{},
		}
		for i, lc := range lreq.Citations {
			l := lresp.Links[i]
			c := Citation{
				Seq: lc.Seq, Raw: lc.Raw, CaseName: lc.CaseName, Volume: lc.Volume,
				ReporterAbbr: lc.ReporterAbbr, ReporterStandard: lc.ReporterStandard, Page: lc.Page, Year: lc.Year,
				Pipeline: lc.Pipeline, PipelineN: lc.PipelineN, ReadConfidence: lc.Confidence, ReadNote: lc.Note,
				Status: "no_match", Pincite: l.Pincite, Confidence: l.Confidence,
				Candidates: len(lc.Candidates), Note: l.Note,
			}
			if c.PipelineN > 0 && c.PipelineN <= len(dreq.Pipeline) {
				c.PipelineStatus = dreq.Pipeline[c.PipelineN-1].Status
			}
			for _, cand := range lc.Candidates {
				if cand.ID != l.Candidate {
					continue
				}
				c.Status = statusOf[cand.Source]
				c.LinkedCite, c.LinkedName, c.PipelineLink = cand.Cite, cand.Name, cand.PipelineLink
				if len(cand.How) > 0 {
					c.LinkHow = cand.How[0]
				}
				switch cand.Source {
				case "cap", "code_reporter":
					id, err := strconv.ParseInt(cand.CaseID, 10, 64)
					if err != nil {
						return fmt.Errorf("%s seq %d: candidate id %q: %w", p.id(), c.Seq, cand.CaseID, err)
					}
					if cand.Source == "cap" {
						c.CapCaseID = &id
					} else {
						c.CodeReporterID = &id
					}
				case "english_reports":
					c.ErCaseID = cand.CaseID
				case "stub":
					c.StubCite = cand.CaseID
				}
			}
			rec.Citations = append(rec.Citations, c)
		}
		records = append(records, rec)
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, p)
		}
		return fmt.Errorf("%d problems in the link responses of batch %d", len(problems), n)
	}
	cites, linked := 0, 0
	for _, rec := range records {
		if err := writeJSON(filepath.Join(dir, "results", rec.Psmid, rec.Pageid+".json"), rec); err != nil {
			return err
		}
		for _, c := range rec.Citations {
			cites++
			if c.Status != "no_match" {
				linked++
			}
		}
	}
	if batch.FinishedAt == nil {
		batch.FinishedAt = &now
		if err := writeJSON(filepath.Join(bdir, "batch.json"), batch); err != nil {
			return err
		}
	}
	slog.Info("finished batch", "batch", n, "pages", len(records), "citations", cites, "linked", linked)
	return nil
}
