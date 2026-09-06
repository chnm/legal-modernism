-- migrate:up
SET ROLE = law_admin;

-- Add an 'unprocessed' metric to linking_dashboard_summary (issue #296).
--
-- The view reported total_raw_cites and a count per citation_links.status. A
-- linker run that stopped part-way through simply produced smaller status
-- counts, and every percentage the chambers dashboard derives from them --
-- "% of raw", "% of potential", "% of linked" -- silently rebased on the subset
-- that had been processed. There was nothing on the page to say so, which made
-- a partial run indistinguishable from a finished one.
--
-- The count is a subtraction rather than an anti-join because it can be: every
-- citation_links row has a citations_unlinked row behind it (the citation_id
-- primary key and its foreign key together guarantee it), so the difference of
-- the two counts is exactly the citations with no link row. This is the same
-- expression citation_links_status already uses, for the same reason.
--
-- Recreated rather than altered because a materialized view's query cannot be
-- changed in place. It comes back WITH NO DATA, like its siblings, so the next
-- db-maintenance populates it; until then the dashboard degrades to empty
-- rather than erroring.
DROP MATERIALIZED VIEW IF EXISTS moml_citations.linking_dashboard_summary;

CREATE MATERIALIZED VIEW moml_citations.linking_dashboard_summary AS
 SELECT 'total_raw_cites'::text AS metric,
        count(*) AS n
   FROM moml_citations.citations_unlinked
UNION ALL
 SELECT status AS metric,
        count(*) AS n
   FROM moml_citations.citation_links
  GROUP BY status
UNION ALL
 SELECT 'unprocessed'::text AS metric,
        (SELECT count(*) FROM moml_citations.citations_unlinked)
      - (SELECT count(*) FROM moml_citations.citation_links) AS n
  WITH NO DATA;

CREATE UNIQUE INDEX IF NOT EXISTS linking_dashboard_summary_uq
  ON moml_citations.linking_dashboard_summary USING btree (metric);

-- migrate:down
SET ROLE = law_admin;

DROP MATERIALIZED VIEW IF EXISTS moml_citations.linking_dashboard_summary;

CREATE MATERIALIZED VIEW moml_citations.linking_dashboard_summary AS
 SELECT 'total_raw_cites'::text AS metric,
        count(*) AS n
   FROM moml_citations.citations_unlinked
UNION ALL
 SELECT status AS metric,
        count(*) AS n
   FROM moml_citations.citation_links
  GROUP BY status
  WITH NO DATA;

CREATE UNIQUE INDEX IF NOT EXISTS linking_dashboard_summary_uq
  ON moml_citations.linking_dashboard_summary USING btree (metric);
