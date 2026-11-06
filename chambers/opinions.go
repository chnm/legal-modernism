package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// The case page's view of the CAP opinions (issue #347): the CAP cases whose
// opinions cite the case, from any source, and for a CAP case the cases its
// own opinions cite. Both read opinion_citations.case_case_citations, one row
// per citing case and cited case, and leave out an opinion's citations of its
// own case (self_cite), as every reader of that view does.

// OpinionCase is a case on either side of an opinion citation, with the
// citations between the two and the opinions that make them.
type OpinionCase struct {
	CaseRef
	Cites    int
	Opinions int
}

// OpinionCases is one section of the page: the cases shown, how many there
// are in all, and how many citations they make between them.
type OpinionCases struct {
	Cases      []OpinionCase
	Total      int
	TotalCites int
}

// More reports whether the section shows fewer cases than there are.
func (o OpinionCases) More() bool { return len(o.Cases) < o.Total }

const opinionCasesLimit = 100

// getCitingOpinionCases lists the CAP cases whose opinions cite a case, most
// citations first. The predicate is on the typed case column, which each
// source has a partial index on.
func (s *server) getCitingOpinionCases(ctx context.Context, source, id string, limit int) (OpinionCases, error) {
	col, placeholder := caseIDColumn(source, 1)
	var o OpinionCases
	cases, err := collect(ctx, s.db, `
		SELECT k.id::text, k.name_abbreviation, k.decision_year, capcite.cite,
		       x.cite_count, x.opinion_count,
		       count(*) OVER(), sum(x.cite_count) OVER()::bigint
		FROM opinion_citations.case_case_citations x
		JOIN cap.cases k ON k.id = x.citing_case_id
		LEFT JOIN LATERAL (
		  SELECT ci.cite FROM cap.citations ci WHERE ci."case" = k.id
		  ORDER BY (ci.type = 'official') DESC, ci.cite LIMIT 1
		) capcite ON true
		WHERE x.`+col+` = `+placeholder+` AND NOT x.self_cite
		ORDER BY x.cite_count DESC, k.decision_year NULLS LAST, k.id
		LIMIT $2`, []any{id, limit},
		func(rows pgx.Rows) (OpinionCase, error) {
			c := OpinionCase{CaseRef: CaseRef{Source: "cap"}}
			err := rows.Scan(&c.ID, &c.Name, &c.Year, &c.Cite, &c.Cites, &c.Opinions, &o.Total, &o.TotalCites)
			return c, err
		})
	if err != nil {
		return o, fmt.Errorf("citing opinion cases: %w", err)
	}
	o.Cases = cases
	return o, nil
}

// getCitedOpinionCases lists the cases a CAP case's opinions cite, in any
// source, most citations first. citing_case_id leads the view's unique index.
func (s *server) getCitedOpinionCases(ctx context.Context, id string, limit int) (OpinionCases, error) {
	var o OpinionCases
	cases, err := collect(ctx, s.db, `
		SELECT `+caseMetaColumns("x")+`, x.cite_count, x.opinion_count,
		       count(*) OVER(), sum(x.cite_count) OVER()::bigint
		FROM opinion_citations.case_case_citations x
		`+caseMetaJoins("x")+`
		WHERE x.citing_case_id = $1::bigint AND NOT x.self_cite
		ORDER BY x.cite_count DESC, 4 NULLS LAST, 2
		LIMIT $2`, []any{id, limit},
		func(rows pgx.Rows) (OpinionCase, error) {
			var source, key, name, cite *string
			var year *int
			var c OpinionCase
			if err := rows.Scan(&source, &key, &name, &year, &cite, &c.Cites, &c.Opinions, &o.Total, &o.TotalCites); err != nil {
				return c, err
			}
			if ref := caseRefFrom(source, key, name, year, cite); ref != nil {
				c.CaseRef = *ref
			}
			return c, nil
		})
	if err != nil {
		return o, fmt.Errorf("cited opinion cases: %w", err)
	}
	o.Cases = cases
	return o, nil
}
