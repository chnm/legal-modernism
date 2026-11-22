package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// How far around a cited page code looks for the case it falls in.
const (
	pagesBefore   = 40 // a pin cite can fall deep inside a long case
	pagesNear     = 3  // a start this close is offered whatever its length
	maxNameHits   = 8
	maxCandidates = 24
)

// reporter is a row of legalhist.reporters, as far as cite strings need it.
type reporter struct {
	Standard        string
	CAP             string
	SingleVol       bool
	PagedVolumes    int
	CitedByYearFrom int
}

type capVolume struct {
	Vol      int
	Reporter string
}

// lookups holds the small tables that turn a citation into cite strings.
type lookups struct {
	reporters map[string]*reporter
	whitelist map[string][]string          // spelling -> standard, fallback
	diffvols  map[string]map[int]capVolume // standard -> volume -> CAP's volume
}

func loadLookups(ctx context.Context, pool *pgxpool.Pool) (*lookups, error) {
	lk := &lookups{reporters: map[string]*reporter{}, whitelist: map[string][]string{}, diffvols: map[string]map[int]capVolume{}}
	rows, err := pool.Query(ctx, `
		SELECT reporter_standard, coalesce(reporter_cap, ''), coalesce(single_vol, false),
		       coalesce(paged_volumes, 0), coalesce(cited_by_year_from, 0) FROM legalhist.reporters`)
	if err != nil {
		return nil, fmt.Errorf("loading reporters: %w", err)
	}
	for rows.Next() {
		var r reporter
		if err := rows.Scan(&r.Standard, &r.CAP, &r.SingleVol, &r.PagedVolumes, &r.CitedByYearFrom); err != nil {
			return nil, err
		}
		lk.reporters[r.Standard] = &r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = pool.Query(ctx, `
		SELECT reporter_found, reporter_standard, coalesce(fallback_standard, '')
		  FROM legalhist.whitelist WHERE NOT junk`)
	if err != nil {
		return nil, fmt.Errorf("loading whitelist: %w", err)
	}
	for rows.Next() {
		var found, std, fb string
		if err := rows.Scan(&found, &std, &fb); err != nil {
			return nil, err
		}
		lk.whitelist[found] = []string{std}
		if fb != "" {
			lk.whitelist[found] = append(lk.whitelist[found], fb)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = pool.Query(ctx, `
		SELECT reporter_standard, vol, cap_vol, cap_reporter FROM legalhist.reporters_diffvols
		 WHERE vol IS NOT NULL AND cap_vol IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("loading diffvols: %w", err)
	}
	for rows.Next() {
		var std string
		var vol int
		var cv capVolume
		if err := rows.Scan(&std, &vol, &cv.Vol, &cv.Reporter); err != nil {
			return nil, err
		}
		if lk.diffvols[std] == nil {
			lk.diffvols[std] = map[int]capVolume{}
		}
		lk.diffvols[std][vol] = cv
	}
	return lk, rows.Err()
}

// readings returns the reporters a citation may name: the one the first
// request chose, then what the whitelist makes of the printed spelling.
func (lk *lookups) readings(c *Detected) []*reporter {
	var out []*reporter
	seen := map[string]bool{}
	add := func(std string) {
		if r := lk.reporters[std]; r != nil && !seen[std] {
			seen[std] = true
			out = append(out, r)
		}
	}
	add(c.ReporterStandard)
	for _, std := range lk.whitelist[c.ReporterAbbr] {
		add(std)
	}
	return out
}

// citeStrings returns the strings a citation of this reporter, volume and
// page is keyed by in the sources, as the linker builds them: under the
// standard spelling, CAP's spelling and CAP's renumbered volume, with the
// volume added or dropped for a single-volume reporter, and led by the year
// for a reporter cited by year.
func (lk *lookups) citeStrings(r *reporter, vol *int, page int, year *int) []string {
	prefix := ""
	if r.CitedByYearFrom > 0 && year != nil && *year >= r.CitedByYearFrom {
		prefix = fmt.Sprintf("[%d] ", *year)
	}
	one := 1
	forms := []*int{vol}
	switch {
	case vol != nil && *vol > 1 && *vol <= r.PagedVolumes:
		forms = append(forms, nil)
	case r.SingleVol && vol == nil:
		forms = append(forms, &one)
	case r.SingleVol && *vol == 1:
		forms = append(forms, nil)
	}
	var out []string
	build := func(v *int, name string) {
		if v == nil {
			out = append(out, prefix+fmt.Sprintf("%s %d", name, page))
		} else {
			out = append(out, prefix+fmt.Sprintf("%d %s %d", *v, name, page))
		}
	}
	for _, v := range forms {
		build(v, r.Standard)
		if r.CAP != "" {
			build(v, r.CAP)
		}
		if v != nil {
			if cv, ok := lk.diffvols[r.Standard][*v]; ok {
				build(&cv.Vol, cv.Reporter)
			}
		}
	}
	return out
}

// citePatterns returns LIKE patterns for the cites of the volume a citation
// names and for those of its reporter in any volume, under every reading.
func (lk *lookups) citePatterns(c *Detected) [2][]string {
	esc := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	out := [2][]string{{}, {}}
	for _, r := range lk.readings(c) {
		names := map[string]*int{r.Standard: c.Volume}
		if r.CAP != "" {
			names[r.CAP] = c.Volume
		}
		if c.Volume != nil {
			if cv, ok := lk.diffvols[r.Standard][*c.Volume]; ok {
				names[cv.Reporter] = &cv.Vol
			}
		}
		for name, vol := range names {
			if vol != nil {
				out[0] = append(out[0], fmt.Sprintf("%d %s %%", *vol, esc.Replace(name)))
			}
			out[1] = append(out[1], "%"+esc.Replace(name)+" %")
		}
	}
	return out
}

// probe is why a cite string was looked up: the page it stands for, and
// whether it is an alternate reading of the citation's numbers.
type probe struct {
	page      int
	alternate bool
}

// candidateSet gathers the candidates of one citation, merging the reasons
// a case was found for.
type candidateSet struct {
	byKey map[string]*Candidate
	order []string
}

func (s *candidateSet) add(c Candidate, how string) *Candidate {
	key := c.Source + "/" + c.CaseID
	have, ok := s.byKey[key]
	if !ok {
		cc := c
		have = &cc
		s.byKey[key] = have
		s.order = append(s.order, key)
	}
	for _, h := range have.How {
		if h == how {
			return have
		}
	}
	have.How = append(have.How, how)
	return have
}

// rank orders reasons, best first, so the likeliest candidates lead.
func rank(c *Candidate) int {
	best := 9
	for _, h := range c.How {
		r := 6
		switch {
		case h == "exact cite":
			r = 0
		case h == "pipeline link":
			r = 1
		case h == "exact cite of alternate reading":
			r = 2
		case h == "page inside case":
			r = 3
		case h == "name search":
			r = 4
		case strings.HasPrefix(h, "begins"):
			r = 5
		}
		best = min(best, r)
	}
	if len(c.How) > 1 {
		best-- // found two ways
	}
	return best
}

func (s *candidateSet) list() []Candidate {
	out := make([]Candidate, 0, len(s.order))
	for _, k := range s.order {
		out = append(out, *s.byKey[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(&out[i]) < rank(&out[j]) })
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	for i := range out {
		out[i].ID = fmt.Sprintf("c%d", i+1)
	}
	return out
}

func near(q, page int) string {
	switch d := page - q; {
	case d == 0:
		return "exact cite"
	case d == 1:
		return "begins 1 page before"
	case d > 1:
		return fmt.Sprintf("begins %d pages before", d)
	case d == -1:
		return "begins 1 page after"
	default:
		return fmt.Sprintf("begins %d pages after", -d)
	}
}

// candidatesFor looks a citation up in every source: by its cite and the
// cites around it, by its alternate readings, and by its printed name.
func candidatesFor(ctx context.Context, pool *pgxpool.Pool, lk *lookups, c *Detected, ceiling int, pipeline []PipelineCitation) ([]Candidate, error) {
	set := &candidateSet{byKey: map[string]*Candidate{}}

	probes := map[string]probe{}
	if c.Page != nil {
		for _, r := range lk.readings(c) {
			for q := max(1, *c.Page-pagesBefore); q <= *c.Page+pagesNear; q++ {
				for _, s := range lk.citeStrings(r, c.Volume, q, c.Year) {
					if _, ok := probes[s]; !ok {
						probes[s] = probe{page: q}
					}
				}
			}
			for _, a := range c.Alternates {
				if a.Page == nil {
					continue
				}
				for _, s := range lk.citeStrings(r, a.Volume, *a.Page, c.Year) {
					if _, ok := probes[s]; !ok {
						probes[s] = probe{page: *a.Page, alternate: true}
					}
				}
			}
		}
	}
	if len(probes) > 0 {
		cites := make([]string, 0, len(probes))
		for s := range probes {
			cites = append(cites, s)
		}
		sort.Strings(cites)
		if err := probeCites(ctx, pool, set, cites, probes, *c.Page); err != nil {
			return nil, err
		}
	}

	if tokens := nameTokens(c.CaseName); len(tokens) > 0 {
		if err := searchNames(ctx, pool, set, tokens, ceiling, lk.citePatterns(c)); err != nil {
			return nil, err
		}
	}

	// The case the pipeline linked this citation to, when it detected it at
	// the same numbers or under the same raw string.
	for _, p := range pipeline {
		if p.caseID == "" {
			continue
		}
		same := p.Raw == c.Raw || (c.Page != nil && p.Page == *c.Page && eq(p.Volume, c.Volume))
		if !same {
			continue
		}
		cand := Candidate{Source: p.source, CaseID: p.caseID, Cite: p.Linked}
		if p.source == "stub" {
			cand.Cite = p.caseID
		}
		set.add(cand, "pipeline link").PipelineLink = true
	}
	return set.list(), nil
}

func eq(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// probeCites looks the cite strings up in the four sources and keeps the
// cases at the cited page, around it, or containing it.
func probeCites(ctx context.Context, pool *pgxpool.Pool, set *candidateSet, cites []string, probes map[string]probe, page int) error {
	how := func(cite string) (string, bool) {
		p := probes[cite]
		if p.alternate {
			return "exact cite of alternate reading", true
		}
		return near(p.page, page), false
	}

	rows, err := pool.Query(ctx, `
		SELECT ci.cite, c.id::text, c.name_abbreviation, coalesce(c.decision_year, 0),
		       coalesce(c.first_page, 0), coalesce(c.last_page, 0)
		  FROM cap.citations ci JOIN cap.cases c ON c.id = ci."case"
		 WHERE ci.cite = ANY($1)`, cites)
	if err != nil {
		return fmt.Errorf("probing CAP: %w", err)
	}
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.Cite, &c.CaseID, &c.Name, &c.Year, &c.FirstPage, &c.LastPage); err != nil {
			return err
		}
		c.Source = "cap"
		h, alt := how(c.Cite)
		q := probes[c.Cite].page
		inside := q < page && c.LastPage >= page
		switch {
		case alt:
			set.add(c, h)
		case abs(q-page) <= pagesNear:
			set.add(c, h)
			if inside {
				set.add(c, "page inside case")
			}
		case inside && c.LastPage-c.FirstPage < 400:
			set.add(c, "page inside case")
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// The English Reports give no last page: offer the starts near the page
	// and the two nearest before it, with the length of each report.
	rows, err = pool.Query(ctx, `
		SELECT er_parallel_cite, id, coalesce(nullif(murrell_title, ''), er_name, ''),
		       coalesce(murrell_year, er_year), coalesce(word_count, 0)
		  FROM english_reports.cases WHERE er_parallel_cite = ANY($1)`, cites)
	if err != nil {
		return fmt.Errorf("probing the English Reports: %w", err)
	}
	type erHit struct {
		c Candidate
		q int
	}
	var before []erHit
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.Cite, &c.CaseID, &c.Name, &c.Year, &c.Words); err != nil {
			return err
		}
		c.Source = "english_reports"
		h, alt := how(c.Cite)
		q := probes[c.Cite].page
		switch {
		case alt || abs(q-page) <= pagesNear:
			set.add(c, h)
		case q < page:
			before = append(before, erHit{c, q})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	sort.Slice(before, func(i, j int) bool { return before[i].q > before[j].q })
	kept := map[int]bool{}
	for _, hit := range before {
		if !kept[hit.q] && len(kept) == 2 {
			break
		}
		kept[hit.q] = true
		set.add(hit.c, near(hit.q, page))
	}

	rows, err = pool.Query(ctx, `
		SELECT official_citation, id::text, name_abbreviation, decision_year, first_page, last_page
		  FROM legalhist.code_reporter WHERE official_citation = ANY($1)
		UNION ALL
		SELECT parallel_citation, id::text, name_abbreviation, decision_year, first_page, last_page
		  FROM legalhist.code_reporter WHERE parallel_citation = ANY($1)`, cites)
	if err != nil {
		return fmt.Errorf("probing the code reporter: %w", err)
	}
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.Cite, &c.CaseID, &c.Name, &c.Year, &c.FirstPage, &c.LastPage); err != nil {
			return err
		}
		c.Source = "code_reporter"
		h, alt := how(c.Cite)
		if q := probes[c.Cite].page; alt || abs(q-page) <= pagesNear {
			set.add(c, h)
		} else if q < page && c.LastPage >= page {
			set.add(c, "page inside case")
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// A stub is known only by its cite: offer the one at the cited page, and
	// the two nearest before it, since the page may be a pin cite into a case
	// whose first page is a stub. A stub has no last page to check that by.
	rows, err = pool.Query(ctx, `SELECT cite FROM legalhist.stub_cases WHERE cite = ANY($1)`, cites)
	if err != nil {
		return fmt.Errorf("probing the stubs: %w", err)
	}
	var stubsBefore []erHit
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.Cite); err != nil {
			return err
		}
		c.Source, c.CaseID = "stub", c.Cite
		h, alt := how(c.Cite)
		q := probes[c.Cite].page
		switch {
		case alt || q == page:
			set.add(c, h)
		case q < page:
			stubsBefore = append(stubsBefore, erHit{c, q})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	sort.Slice(stubsBefore, func(i, j int) bool { return stubsBefore[i].q > stubsBefore[j].q })
	for i, hit := range stubsBefore {
		if i == 2 {
			break
		}
		set.add(hit.c, near(hit.q, page))
	}
	return nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var (
	versus = regexp.MustCompile(`(?i)\s+(?:v|vs|versus|against|ads|agt)\.?\s+`)
	word   = regexp.MustCompile(`[\p{L}]+`)
	// Words too common in case names to find a case by.
	common = map[string]bool{
		"anonymous": true, "anon": true, "case": true, "cases": true, "state": true, "states": true,
		"united": true, "people": true, "commonwealth": true, "king": true, "queen": true, "regina": true,
		"bank": true, "company": true, "railroad": true, "railway": true, "insurance": true, "city": true,
		"county": true, "town": true, "parte": true, "matter": true, "estate": true, "lessee": true,
		"executors": true, "administrator": true, "administrators": true, "others": true, "inhabitants": true,
		"mayor": true, "bishop": true, "attorney": true, "general": true, "national": true, "same": true,
	}
)

// nameTokens picks the words to search a case name by: the most distinctive
// word of each party, or of the whole name when it has no "v.".
func nameTokens(name string) []string {
	var out []string
	for _, part := range versus.Split(name, 2) {
		best := ""
		for _, w := range word.FindAllString(part, -1) {
			w = strings.ToLower(w)
			if len(w) >= 4 && !common[w] && len(w) > len(best) {
				best = w
			}
		}
		if best != "" {
			out = append(out, best)
		}
	}
	// One short word finds too many cases to be worth offering.
	if len(out) == 1 && len(out[0]) < 6 {
		return nil
	}
	return out
}

// searchNames offers the cases whose names hold every token, decided no
// later than the citing book.
//
// volume and series are LIKE patterns for a cite in the cited volume and in
// the cited reporter; cases they match come first, so that a common name
// (Waddell v. Waddell) still finds the case in the reporter cited.
func searchNames(ctx context.Context, pool *pgxpool.Pool, set *candidateSet, tokens []string, ceiling int, pat [2][]string) error {
	if ceiling == 0 {
		ceiling = 1930
	}
	patterns := make([]string, len(tokens))
	for i, t := range tokens {
		patterns[i] = "%" + t + "%"
	}
	rows, err := pool.Query(ctx, `
		SELECT c.id::text, c.name_abbreviation, coalesce(c.decision_year, 0), coalesce(c.first_page, 0), coalesce(c.last_page, 0),
		       coalesce((SELECT string_agg(ci.cite, '; ' ORDER BY ci.type DESC, ci.cite) FROM cap.citations ci WHERE ci."case" = c.id), '')
		  FROM cap.cases c
		 WHERE c.decision_year <= $2 AND c.name_abbreviation ILIKE ALL($1)
		 ORDER BY EXISTS (SELECT 1 FROM cap.citations ci WHERE ci."case" = c.id AND ci.cite LIKE ANY($4)) DESC,
		          EXISTS (SELECT 1 FROM cap.citations ci WHERE ci."case" = c.id AND ci.cite LIKE ANY($5)) DESC,
		          c.decision_year LIMIT $3`, patterns, ceiling, maxNameHits, pat[0], pat[1])
	if err != nil {
		return fmt.Errorf("searching CAP names: %w", err)
	}
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.CaseID, &c.Name, &c.Year, &c.FirstPage, &c.LastPage, &c.Cite); err != nil {
			return err
		}
		c.Source = "cap"
		set.add(c, "name search")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = pool.Query(ctx, `
		SELECT id, coalesce(er_parallel_cite, er_cite), coalesce(nullif(murrell_title, ''), er_name, ''),
		       coalesce(murrell_year, er_year), coalesce(word_count, 0)
		  FROM english_reports.cases
		 WHERE er_year <= $2 + 1 AND (er_name ILIKE ALL($1) OR murrell_title ILIKE ALL($1))
		 ORDER BY coalesce(er_parallel_cite LIKE ANY($4), false) DESC, coalesce(er_parallel_cite LIKE ANY($5), false) DESC,
		          er_year LIMIT $3`, patterns, ceiling, maxNameHits, pat[0], pat[1])
	if err != nil {
		return fmt.Errorf("searching English Reports names: %w", err)
	}
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.CaseID, &c.Cite, &c.Name, &c.Year, &c.Words); err != nil {
			return err
		}
		c.Source = "english_reports"
		set.add(c, "name search")
	}
	return rows.Err()
}

// cmdCandidates checks the first responses of a batch and writes the second
// request for each page: its citations, each with the cases it could be.
func cmdCandidates(ctx context.Context, pool *pgxpool.Pool, dir string, n int) error {
	batch, err := loadBatch(dir, n)
	if err != nil {
		return err
	}
	bdir := batchDir(dir, n)
	var problems []string
	responses := map[string]*DetectResponse{}
	for _, p := range batch.Pages {
		var resp DetectResponse
		if err := readJSON(filepath.Join(bdir, "detect", p.id()+".response.json"), &resp); err != nil {
			problems = append(problems, err.Error())
			continue
		}
		problems = append(problems, resp.check(p.id())...)
		responses[p.id()] = &resp
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, p)
		}
		return fmt.Errorf("%d problems in the detect responses of batch %d", len(problems), n)
	}

	lk, err := loadLookups(ctx, pool)
	if err != nil {
		return err
	}
	cites, cands := 0, 0
	for _, p := range batch.Pages {
		var dreq DetectRequest
		if err := readJSON(filepath.Join(bdir, "detect", p.id()+".request.json"), &dreq); err != nil {
			return err
		}
		pipeline, err := pipelineFor(ctx, pool, p.Psmid, p.Pageid, dreq.Text)
		if err != nil {
			return err
		}
		req := &LinkRequest{CustomID: p.id(), Edition: dreq.Edition, Page: dreq.Page, Text: dreq.Text, Citations: []LinkCitation{}}
		for _, c := range responses[p.id()].Citations {
			list, err := candidatesFor(ctx, pool, lk, &c, dreq.Edition.Year, pipeline)
			if err != nil {
				return fmt.Errorf("%s seq %d: %w", p.id(), c.Seq, err)
			}
			req.Citations = append(req.Citations, LinkCitation{Detected: c, Candidates: list})
			cites++
			cands += len(list)
		}
		if err := writeJSON(filepath.Join(bdir, "link", p.id()+".request.json"), req); err != nil {
			return err
		}
	}
	slog.Info("wrote link requests", "batch", n, "pages", len(batch.Pages), "citations", cites, "candidates", cands)
	return nil
}
