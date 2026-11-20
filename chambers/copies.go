package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
)

// An edition can be in the database in several copies (issue #402): the same
// printing scanned by Gale and by HathiTrust, or another printing of it. Each
// copy is a row of moml.editions, and editions.copy_of names the official
// copy, the one that stands for the edition in every list and count. Chambers
// lists and counts official copies, and names the others beside them: on the
// work page under their edition, and on each copy's own edition page.

// EditionCopy is one copy of an edition as it is named beside another.
type EditionCopy struct {
	BiblioID         string
	Edition          string // the official copy's bibliographic id
	Official         bool
	Source           string // MOML, HathiTrust, or side corpus
	Year             *int
	EditionStatement *string
	Imprint          *string
	Pages            *int
	Links            SourceLinks
	Cites            int
	Linked           int
	HasCounts        bool
}

func (c EditionCopy) URL() string       { return editionURL(c.BiblioID) }
func (c EditionCopy) LinkedPct() string { return pct(c.Linked, c.Cites) }

// Label names the copy by where it was scanned and its year: "MOML, 1859".
func (c EditionCopy) Label() string {
	if c.Year == nil {
		return c.Source
	}
	return fmt.Sprintf("%s, %d", c.Source, *c.Year)
}

// copiesLabel names several copies in a line, for a tooltip.
func copiesLabel(copies []EditionCopy) string {
	labels := make([]string, len(copies))
	for i, c := range copies {
		labels[i] = c.Label()
	}
	return strings.Join(labels, "; ")
}

// officialCopiesSQL is a condition on citations_unlinked cu that leaves out
// the citations in copies that are not official, for a tally over citations
// that would otherwise count an edition once for each copy. The volumes of
// such copies are few, so the subquery is a small hashed list.
const officialCopiesSQL = `cu.moml_treatise NOT IN (
	    SELECT v.psmid FROM moml.volumes v
	    JOIN moml.editions e ON e.bibliographicid = v.bibliographicid
	    WHERE e.copy_of IS NOT NULL)`

// The copies a query can ask for, each a condition on moml.editions e with
// one parameter. Fixed literals, never user input.
const (
	// copiesOfWork is every copy in a work that is not official.
	copiesOfWork = `e.work_id = $1 AND e.copy_of IS NOT NULL`
	// copiesBeside is the other copies of the edition a row belongs to,
	// the official one among them when the row is not it.
	copiesBeside = `e.bibliographicid <> $1 AND EXISTS (
	    SELECT 1 FROM moml.editions x
	    WHERE x.bibliographicid = $1
	      AND COALESCE(x.copy_of, x.bibliographicid) IN (e.bibliographicid, e.copy_of))`
)

// getCopies lists copies of editions, official first and then earliest
// first. A copy that is not official is not in the treatise view, so its
// citation counts come from its volumes (treatise_citation_counts); when that
// view is not populated the copies are listed without counts.
func (s *server) getCopies(ctx context.Context, where string, arg any) ([]EditionCopy, error) {
	copies, err := s.queryCopies(ctx, where, arg, true)
	if isUnavailable(err) {
		copies, err = s.queryCopies(ctx, where, arg, false)
	}
	if err != nil {
		return nil, fmt.Errorf("edition copies: %w", err)
	}
	return copies, nil
}

func (s *server) queryCopies(ctx context.Context, where string, arg any, withCounts bool) ([]EditionCopy, error) {
	slog.Debug("querying edition copies", "arg", arg, "with_counts", withCounts)
	counts := "NULL::bigint, NULL::bigint"
	join := ""
	if withCounts {
		counts = "t.cites, t.linked"
		join = `LEFT JOIN LATERAL (
		  SELECT COALESCE(sum(tcc.n), 0)::bigint AS cites, COALESCE(sum(tcc.linked), 0)::bigint AS linked
		  FROM moml.volumes vv
		  LEFT JOIN moml_citations.treatise_citation_counts tcc ON tcc.moml_treatise = vv.psmid
		  WHERE vv.bibliographicid = e.bibliographicid
		) t ON true`
	}
	query := `
	SELECT e.bibliographicid, c.edition, c.official, c.source,
	       v.year, v.edition_statement, v.imprint, v.pages, v.product_link, ` + counts + `
	FROM moml.editions e
	JOIN moml.edition_copies c ON c.bibliographicid = e.bibliographicid
	JOIN LATERAL (
	  SELECT min(vv.year) AS year, sum(vv.total_pages)::bigint AS pages,
	         (array_agg(vv.edition_statement ORDER BY vv.current_volume, vv.psmid))[1] AS edition_statement,
	         (array_agg(vv.imprint ORDER BY vv.current_volume, vv.psmid))[1] AS imprint,
	         (array_agg(vv.product_link ORDER BY vv.current_volume, vv.psmid))[1] AS product_link
	  FROM moml.volumes vv WHERE vv.bibliographicid = e.bibliographicid
	) v ON true
	` + join + `
	WHERE ` + where + `
	ORDER BY c.official DESC, v.year NULLS LAST, e.bibliographicid`
	return collect(ctx, s.db, query, []any{arg}, func(rows pgx.Rows) (EditionCopy, error) {
		var c EditionCopy
		var productLink *string
		var cites, linked *int
		err := rows.Scan(&c.BiblioID, &c.Edition, &c.Official, &c.Source,
			&c.Year, &c.EditionStatement, &c.Imprint, &c.Pages, &productLink, &cites, &linked)
		c.Links = sourceLinks(productLink, "")
		if cites != nil && linked != nil {
			c.Cites, c.Linked, c.HasCounts = *cites, *linked, true
		}
		return c, err
	})
}
