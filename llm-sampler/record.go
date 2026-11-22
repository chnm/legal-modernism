package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Model is the model the requests are written for.
const Model = "claude-opus-5-5"

// Batch is the record of one batch of pages: what was drawn and when the
// batch was worked on. It is batches/<NNNN>/batch.json.
type Batch struct {
	Batch      int        `json:"batch"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// CitedWeight is how many times likelier a page with a pipeline citation
	// was than one without; absent (1) for the first batches, drawn uniformly.
	CitedWeight int         `json:"cited_weight,omitempty"`
	Pages       []BatchPage `json:"pages"`
}

// BatchPage is one page of a batch, with the draw that chose it.
type BatchPage struct {
	Draw            int    `json:"draw"`
	Bibliographicid string `json:"bibliographicid"`
	Psmid           string `json:"psmid"`
	Pageid          string `json:"pageid"`

	// How the page was drawn within its edition: its weight, the probability
	// that gave it, and the edition's body pages and how many of them the
	// pipeline found a citation on. Absent for pages drawn uniformly.
	Weight       int     `json:"weight,omitempty"`
	Probability  float64 `json:"probability,omitempty"`
	EditionPages int     `json:"edition_pages,omitempty"`
	EditionCited int     `json:"edition_cited,omitempty"`
}

func (p BatchPage) id() string { return p.Psmid + "-" + p.Pageid }

// Edition is what a request says about the book a page comes from.
type Edition struct {
	Bibliographicid  string   `json:"bibliographicid"`
	Title            string   `json:"title"`
	Author           string   `json:"author,omitempty"`
	Year             int      `json:"year,omitempty"`
	Jurisdiction     string   `json:"jurisdiction"`
	EditionStatement string   `json:"edition_statement,omitempty"`
	Imprint          string   `json:"imprint,omitempty"`
	Volume           int      `json:"volume,omitempty"`
	Volumes          int      `json:"volumes,omitempty"`
	Subjects         []string `json:"subjects,omitempty"`
}

// PageRef names the page of a request.
type PageRef struct {
	Psmid       string `json:"psmid"`
	Pageid      string `json:"pageid"`
	PrintedPage string `json:"printed_page,omitempty"`
}

// PipelineCitation is one citation the regex detector found on the page,
// with what the linker made of it.
type PipelineCitation struct {
	Raw             string `json:"raw"`
	CaseName        string `json:"case_name,omitempty"`
	Volume          *int   `json:"volume,omitempty"`
	ReporterAbbr    string `json:"reporter_abbr"`
	Page            int    `json:"page"`
	Year            *int   `json:"year,omitempty"`
	ReporterReading string `json:"reporter_reading"`
	Status          string `json:"status"`
	Tier            string `json:"tier,omitempty"`
	Linked          string `json:"linked,omitempty"`

	// The link's target, kept out of the request: the candidates step uses
	// it to mark the candidate the pipeline chose.
	source string
	caseID string
}

// DetectRequest is the user message of the first request for a page.
type DetectRequest struct {
	CustomID         string             `json:"custom_id"`
	Edition          Edition            `json:"edition"`
	Page             PageRef            `json:"page"`
	PreviousPageTail string             `json:"previous_page_tail,omitempty"`
	Text             string             `json:"text"`
	NextPageHead     string             `json:"next_page_head,omitempty"`
	Pipeline         []PipelineCitation `json:"pipeline_detections"`
}

// Reading is another way the numbers of a citation may be read.
type Reading struct {
	Volume *int `json:"volume,omitempty"`
	Page   *int `json:"page,omitempty"`
}

// Detected is one citation as the first request returns it.
type Detected struct {
	Seq              int       `json:"seq"`
	Raw              string    `json:"raw"`
	CaseName         string    `json:"case_name,omitempty"`
	Volume           *int      `json:"volume,omitempty"`
	ReporterAbbr     string    `json:"reporter_abbr,omitempty"`
	ReporterStandard string    `json:"reporter_standard,omitempty"`
	Page             *int      `json:"page,omitempty"`
	Year             *int      `json:"year,omitempty"`
	Alternates       []Reading `json:"alternates,omitempty"`
	Pipeline         string    `json:"pipeline"`
	Confidence       string    `json:"confidence"`
	Note             string    `json:"note,omitempty"`
}

// Rejected is a pipeline detection the first request decided is not a
// citation of a case.
type Rejected struct {
	Raw    string `json:"raw"`
	Reason string `json:"reason"`
}

// DetectResponse is what the first request returns.
type DetectResponse struct {
	CustomID         string     `json:"custom_id"`
	Note             string     `json:"note,omitempty"`
	Citations        []Detected `json:"citations"`
	PipelineRejected []Rejected `json:"pipeline_rejected,omitempty"`
}

// Candidate is a case code found that a citation might refer to.
type Candidate struct {
	ID           string   `json:"id"`
	Source       string   `json:"source"`
	Cite         string   `json:"cite"`
	Name         string   `json:"name,omitempty"`
	Year         int      `json:"year,omitempty"`
	FirstPage    int      `json:"first_page,omitempty"`
	LastPage     int      `json:"last_page,omitempty"`
	Words        int      `json:"words,omitempty"`
	How          []string `json:"how"`
	PipelineLink bool     `json:"pipeline_link,omitempty"`

	// The target the candidate stands for; kept in the candidates file so
	// that finish can turn the id chosen back into a typed link.
	CaseID string `json:"case_id"`
}

// LinkCitation is a detected citation with its candidates.
type LinkCitation struct {
	Detected
	Candidates []Candidate `json:"candidates"`
}

// LinkRequest is the user message of the second request for a page.
type LinkRequest struct {
	CustomID  string         `json:"custom_id"`
	Edition   Edition        `json:"edition"`
	Page      PageRef        `json:"page"`
	Text      string         `json:"text"`
	Citations []LinkCitation `json:"citations"`
}

// Link is the choice the second request makes for one citation.
type Link struct {
	Seq        int    `json:"seq"`
	Candidate  string `json:"candidate,omitempty"`
	Pincite    bool   `json:"pincite,omitempty"`
	Confidence string `json:"confidence"`
	Note       string `json:"note,omitempty"`
}

// LinkResponse is what the second request returns.
type LinkResponse struct {
	CustomID string `json:"custom_id"`
	Links    []Link `json:"links"`
}

// Citation is one citation of the final record: the reading of the first
// request and the link of the second.
type Citation struct {
	Seq              int    `json:"seq"`
	Raw              string `json:"raw"`
	CaseName         string `json:"case_name,omitempty"`
	Volume           *int   `json:"volume,omitempty"`
	ReporterAbbr     string `json:"reporter_abbr,omitempty"`
	ReporterStandard string `json:"reporter_standard,omitempty"`
	Page             *int   `json:"page,omitempty"`
	Year             *int   `json:"year,omitempty"`
	Pipeline         string `json:"pipeline"`
	ReadConfidence   string `json:"read_confidence"`
	ReadNote         string `json:"read_note,omitempty"`
	Status           string `json:"status"`
	CapCaseID        *int64 `json:"cap_case_id,omitempty"`
	ErCaseID         string `json:"er_case_id,omitempty"`
	CodeReporterID   *int64 `json:"code_reporter_id,omitempty"`
	StubCite         string `json:"stub_cite,omitempty"`
	LinkedCite       string `json:"linked_cite,omitempty"`
	LinkedName       string `json:"linked_name,omitempty"`
	LinkHow          string `json:"link_how,omitempty"`
	PipelineLink     bool   `json:"pipeline_link,omitempty"`
	Pincite          bool   `json:"pincite,omitempty"`
	Confidence       string `json:"confidence"`
	Candidates       int    `json:"candidates"`
	Note             string `json:"note,omitempty"`
}

// PageRecord is the final record of one page, results/<psmid>/<pageid>.json.
type PageRecord struct {
	Psmid            string     `json:"psmid"`
	Pageid           string     `json:"pageid"`
	Bibliographicid  string     `json:"bibliographicid"`
	Title            string     `json:"title"`
	Year             int        `json:"year,omitempty"`
	Jurisdiction     string     `json:"jurisdiction"`
	Batch            int        `json:"batch"`
	Draw             int        `json:"draw"`
	Model            string     `json:"model"`
	ReadAt           time.Time  `json:"read_at"`
	Note             string     `json:"note,omitempty"`
	PipelineCount    int        `json:"pipeline_detections"`
	PipelineRejected []Rejected `json:"pipeline_rejected,omitempty"`
	Citations        []Citation `json:"citations"`
}

var (
	confidences = map[string]bool{"high": true, "medium": true, "low": true}
	relations   = map[string]bool{"same": true, "corrected": true, "new": true}
	statusOf    = map[string]string{
		"cap": "linked_cap", "english_reports": "linked_english_reports",
		"code_reporter": "linked_code_reporter", "stub": "linked_stub",
	}
)

func batchDir(dir string, n int) string {
	return filepath.Join(dir, "batches", fmt.Sprintf("%04d", n))
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// readBatches returns every batch in the directory, in order.
func readBatches(dir string) ([]*Batch, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "batches", "*", "batch.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []*Batch
	for _, p := range paths {
		var b Batch
		if err := readJSON(p, &b); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, nil
}

// check reports what is wrong with a first response.
func (r *DetectResponse) check(id string) []string {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, id+": "+fmt.Sprintf(format, a...)) }
	if r.CustomID != id {
		add("custom_id is %q", r.CustomID)
	}
	if r.Citations == nil {
		add("citations must be present (use [] for a page with none)")
	}
	for i, c := range r.Citations {
		if c.Seq != i+1 {
			add("citation %d: seq is %d, want %d", i+1, c.Seq, i+1)
		}
		if strings.TrimSpace(c.Raw) == "" {
			add("citation %d: raw is required", i+1)
		}
		if !confidences[c.Confidence] {
			add("citation %d: confidence must be high, medium or low, not %q", i+1, c.Confidence)
		}
		if !relations[c.Pipeline] {
			add("citation %d: pipeline must be same, corrected or new, not %q", i+1, c.Pipeline)
		}
	}
	return errs
}

// check reports what is wrong with a second response, given its request.
func (r *LinkResponse) check(req *LinkRequest) []string {
	var errs []string
	id := req.CustomID
	add := func(format string, a ...any) { errs = append(errs, id+": "+fmt.Sprintf(format, a...)) }
	if r.CustomID != id {
		add("custom_id is %q", r.CustomID)
	}
	if len(r.Links) != len(req.Citations) {
		add("%d links for %d citations: give one link for each seq", len(r.Links), len(req.Citations))
		return errs
	}
	for i, l := range r.Links {
		if l.Seq != i+1 {
			add("link %d: seq is %d, want %d", i+1, l.Seq, i+1)
		}
		if !confidences[l.Confidence] {
			add("link %d: confidence must be high, medium or low, not %q", i+1, l.Confidence)
		}
		if l.Candidate == "" {
			if l.Pincite {
				add("link %d: a no_match cannot be a pin cite", i+1)
			}
			continue
		}
		found := false
		for _, c := range req.Citations[i].Candidates {
			found = found || c.ID == l.Candidate
		}
		if !found {
			add("link %d: candidate %q is not one of the candidates offered for seq %d", i+1, l.Candidate, l.Seq)
		}
	}
	return errs
}
