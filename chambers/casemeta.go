package main

import (
	"strconv"
	"strings"
)

// A case is whatever a linked citation points to: a CAP case, an English
// Reports case, a code reporter case, or a stub case. The four sources are
// keyed differently, so every page passes cases around as a CaseRef and every
// query that shows one joins the four source tables the same way.

// caseSources are the source codes of edition_case_citations and the URL
// segment that names each.
var caseSourceLabels = map[string]string{
	"cap":  "CAP",
	"er":   "Eng. Rep.",
	"code": "Code rep.",
	"stub": "Stub",
}

func validCaseSource(source string) bool {
	_, ok := caseSourceLabels[source]
	return ok
}

// CaseRef identifies a case and carries what a list needs to show it.
type CaseRef struct {
	Source string
	ID     string
	Name   *string
	Year   *int
	Cite   *string
}

// Key is the source:id form the ranking view and the URLs use.
func (c CaseRef) Key() string { return c.Source + ":" + c.ID }

// URL is the case's page in chambers.
func (c CaseRef) URL() string { return caseURL(c.Source, c.ID) }

// SourceLabel is the short human name of the source.
func (c CaseRef) SourceLabel() string {
	if l, ok := caseSourceLabels[c.Source]; ok {
		return l
	}
	return c.Source
}

// BadgeClass is the colour class of the source badge.
func (c CaseRef) BadgeClass() string { return "src-" + c.Source }

// Display is the case's name, else its citation, else a placeholder.
func (c CaseRef) Display() string {
	switch {
	case c.Name != nil && *c.Name != "":
		return *c.Name
	case c.Cite != nil && *c.Cite != "":
		return *c.Cite
	default:
		return "(unknown case)"
	}
}

// splitCaseKey parses a source:id key into its parts.
func splitCaseKey(key string) (source, id string, ok bool) {
	source, id, ok = strings.Cut(key, ":")
	if !ok || !validCaseSource(source) || id == "" {
		return "", "", false
	}
	return source, id, true
}

// caseIDColumn returns the citation_links column that holds a source's case id
// and the typed placeholder to compare it with, so the partial index on the
// column is used. The id is bound as text and cast in SQL for the bigint
// columns; never cast the column itself.
func caseIDColumn(source string, param int) (column string, placeholder string) {
	p := "$" + strconv.Itoa(param)
	switch source {
	case "cap":
		return "cap_case_id", p + "::bigint"
	case "er":
		return "er_case_id", p
	case "code":
		return "code_reporter_id", p + "::bigint"
	default:
		return "stub_cite", p
	}
}

// caseMetaJoins joins the four source tables to the typed case columns of the
// row aliased alias (citation_links, edition_case_citations, or an aggregate
// over either). The CAP citation comes from cap.citations, official form first;
// the stub's name and year from stub_case_metadata, when curated.
func caseMetaJoins(alias string) string {
	return strings.ReplaceAll(`
LEFT JOIN cap.cases cc ON cc.id = A.cap_case_id
LEFT JOIN LATERAL (
  SELECT ci.cite FROM cap.citations ci WHERE ci."case" = A.cap_case_id
  ORDER BY (ci.type = 'official') DESC, ci.cite LIMIT 1
) capcite ON true
LEFT JOIN english_reports.cases er ON er.id = A.er_case_id
LEFT JOIN legalhist.code_reporter code ON code.id = A.code_reporter_id
LEFT JOIN legalhist.stub_case_metadata sm ON sm.cite = A.stub_cite
`, "A.", alias+".")
}

// caseMetaColumns selects, in this order, the source, id, name, year and
// citation of the case on a row joined by caseMetaJoins. Scan them with
// scanCaseRef.
func caseMetaColumns(alias string) string {
	return strings.ReplaceAll(`
CASE WHEN A.cap_case_id IS NOT NULL THEN 'cap'
     WHEN A.er_case_id IS NOT NULL THEN 'er'
     WHEN A.code_reporter_id IS NOT NULL THEN 'code'
     WHEN A.stub_cite IS NOT NULL THEN 'stub' END,
COALESCE(A.cap_case_id::text, A.er_case_id, A.code_reporter_id::text, A.stub_cite),
COALESCE(cc.name_abbreviation, er.murrell_title, er.er_name, code.name, sm.party_names),
COALESCE(cc.decision_year, er.murrell_year, er.er_year, code.decision_year, sm.year_decided),
COALESCE(capcite.cite, er.er_cite, code.official_citation, A.stub_cite)
`, "A.", alias+".")
}

// caseRefFrom builds a CaseRef from the five nullable columns of
// caseMetaColumns; nil when the row has no case.
func caseRefFrom(source, id *string, name *string, year *int, cite *string) *CaseRef {
	if source == nil || id == nil {
		return nil
	}
	return &CaseRef{Source: *source, ID: *id, Name: name, Year: year, Cite: cite}
}
