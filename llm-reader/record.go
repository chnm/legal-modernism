package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PageRecord is what a reader writes for one page it has read, as
// results/<psmid>/<pageid>.json. A page with no citations of cases is still
// written, with an empty Citations list, so that it counts as read.
type PageRecord struct {
	Psmid     string     `json:"psmid"`
	Pageid    string     `json:"pageid"`
	Model     string     `json:"model"`
	ReadAt    time.Time  `json:"read_at"`
	Note      string     `json:"note,omitempty"`
	Citations []Citation `json:"citations"`
}

// Citation is one citation of a case on a page, with the link the reader made.
type Citation struct {
	Seq            int    `json:"seq"`
	Raw            string `json:"raw"`
	CaseName       string `json:"case_name,omitempty"`
	Volume         *int   `json:"volume,omitempty"`
	ReporterAbbr   string `json:"reporter_abbr,omitempty"`
	Page           *int   `json:"page,omitempty"`
	Year           *int   `json:"year,omitempty"`
	Status         string `json:"status"`
	CapCaseID      *int64 `json:"cap_case_id,omitempty"`
	ErCaseID       string `json:"er_case_id,omitempty"`
	CodeReporterID *int64 `json:"code_reporter_id,omitempty"`
	StubCite       string `json:"stub_cite,omitempty"`
	Pincite        bool   `json:"pincite,omitempty"`
	Confidence     string `json:"confidence"`
	Note           string `json:"note,omitempty"`
}

var statuses = map[string]bool{
	"linked_cap": true, "linked_english_reports": true, "linked_code_reporter": true,
	"linked_stub": true, "no_match": true,
}

var confidences = map[string]bool{"high": true, "medium": true, "low": true}

// resultPath is where the record for a page lives.
func resultPath(dir, psmid, pageid string) string {
	return filepath.Join(dir, psmid, pageid+".json")
}

func readRecord(path string) (*PageRecord, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var r PageRecord
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &r, nil
}

// check reports what is wrong with a record on its own terms, without the
// database: the constraints the tables will enforce, and the file's name.
func (r *PageRecord) check(path string) []error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if r.Psmid == "" || r.Pageid == "" {
		add("psmid and pageid are required")
	} else if filepath.Base(filepath.Dir(path)) != r.Psmid || filepath.Base(path) != r.Pageid+".json" {
		add("file name does not match psmid %s / pageid %s", r.Psmid, r.Pageid)
	}
	if r.Model == "" {
		add("model is required")
	}
	if r.ReadAt.IsZero() {
		add("read_at is required")
	}
	if r.Citations == nil {
		add(`citations must be present (use [] for a page with none)`)
	}
	for i, c := range r.Citations {
		at := fmt.Sprintf("citation %d", i+1)
		if c.Seq != i+1 {
			add("%s: seq is %d, want %d (number citations 1, 2, 3 in page order)", at, c.Seq, i+1)
		}
		if strings.TrimSpace(c.Raw) == "" {
			add("%s: raw is required", at)
		}
		if !statuses[c.Status] {
			add("%s: unknown status %q", at, c.Status)
		}
		if !confidences[c.Confidence] {
			add("%s: confidence must be high, medium or low, not %q", at, c.Confidence)
		}
		targets := 0
		if c.CapCaseID != nil {
			targets++
		}
		if c.ErCaseID != "" {
			targets++
		}
		if c.CodeReporterID != nil {
			targets++
		}
		if c.StubCite != "" {
			targets++
		}
		want := map[string]bool{
			"linked_cap":             c.CapCaseID != nil,
			"linked_english_reports": c.ErCaseID != "",
			"linked_code_reporter":   c.CodeReporterID != nil,
			"linked_stub":            c.StubCite != "",
			"no_match":               targets == 0,
		}
		if c.Status == "no_match" && targets != 0 {
			add("%s: no_match must name no target", at)
		} else if c.Status != "no_match" && (targets != 1 || !want[c.Status]) {
			add("%s: status %s must name exactly its own target", at, c.Status)
		}
	}
	return errs
}
