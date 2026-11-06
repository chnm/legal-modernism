-- migrate:up
SET ROLE = law_admin;

-- reporter_case_citations (issue #305) was the longest refresh in make
-- db-maintenance by far: about 510 seconds in every run, and 475 alone on the
-- server on 2026-11-06, against 160 for edition_case_citations, which scans
-- the same 26M linked citations. Two things kept it single-threaded. It
-- counted editions with count(DISTINCT bibliographicid), which Postgres cannot
-- aggregate in parallel, so the whole input was sorted on disk (1.1 GB of
-- temp files) in one process. And it joined moml.treatises before grouping:
-- treatises is a regex-heavy view whose row count the planner guesses at a
-- fifth of the truth, and the underestimate cascaded into a serial nested
-- loop over the citation tables.
--
-- The view is redefined as a two-stage aggregate that gives the same rows.
-- The first stage groups by edition as well, over the whitelisted linked
-- citations, and plans like edition_case_citations: parallel hash joins on
-- seven workers. The second counts the edition groups of each reporter and
-- case and sums their citations, and only here joins moml.treatises. The
-- treatise test is on bibliographicid, a key of the first grouping, so where
-- the join sits does not change the result (checked row for row against the
-- old view on 2026-11-06). Alone on the server the new query took 186 seconds.
--
-- case_edition_counts has the same count(DISTINCT) shape but already plans in
-- parallel (68 seconds alone), and a two-stage rewrite of it was slower.
--
-- Created WITH NO DATA, like its predecessor: make db-maintenance populates it.

DROP MATERIALIZED VIEW IF EXISTS moml_citations.reporter_case_citations;

CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.reporter_case_citations AS
WITH links AS (
  SELECT wl.reporter_standard, v.bibliographicid,
         CASE WHEN cl.cap_case_id IS NOT NULL THEN 'cap'
              WHEN cl.er_case_id IS NOT NULL THEN 'er'
              WHEN cl.code_reporter_id IS NOT NULL THEN 'code'
              ELSE 'stub' END AS source,
         cl.cap_case_id, cl.er_case_id, cl.code_reporter_id, cl.stub_cite,
         count(*) AS cites
  FROM moml_citations.citation_links cl
  JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
  JOIN legalhist.whitelist wl
    ON wl.reporter_found = cu.reporter_abbr AND wl.reporter_standard IS NOT NULL
  JOIN moml.volumes v ON v.psmid = cu.moml_treatise
  WHERE cl.status LIKE 'linked%'
  GROUP BY 1, 2, 3, 4, 5, 6, 7
)
SELECT l.reporter_standard, l.source,
       l.source || ':' || COALESCE(l.cap_case_id::text, l.er_case_id, l.code_reporter_id::text, l.stub_cite) AS case_key,
       l.cap_case_id, l.er_case_id, l.code_reporter_id, l.stub_cite,
       count(*)             AS editions,
       sum(l.cites)::bigint AS cites
FROM links l
JOIN moml.treatises t ON t.bibliographicid = l.bibliographicid
GROUP BY l.reporter_standard, l.source, l.cap_case_id, l.er_case_id, l.code_reporter_id, l.stub_cite
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW moml_citations.reporter_case_citations IS 'For each reporter standard and each case reached through a citation to it, the treatise editions and citations behind the link (issue #305). Refreshed by make db-maintenance.';

CREATE UNIQUE INDEX IF NOT EXISTS reporter_case_citations_uq
  ON moml_citations.reporter_case_citations (reporter_standard, case_key);
CREATE INDEX IF NOT EXISTS reporter_case_citations_editions_idx
  ON moml_citations.reporter_case_citations (reporter_standard, editions DESC);

-- migrate:down
SET ROLE = law_admin;

DROP MATERIALIZED VIEW IF EXISTS moml_citations.reporter_case_citations;

CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.reporter_case_citations AS
WITH links AS (
  SELECT wl.reporter_standard, v.bibliographicid,
         CASE WHEN cl.cap_case_id IS NOT NULL THEN 'cap'
              WHEN cl.er_case_id IS NOT NULL THEN 'er'
              WHEN cl.code_reporter_id IS NOT NULL THEN 'code'
              ELSE 'stub' END AS source,
         cl.cap_case_id, cl.er_case_id, cl.code_reporter_id, cl.stub_cite
  FROM moml_citations.citation_links cl
  JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
  JOIN legalhist.whitelist wl
    ON wl.reporter_found = cu.reporter_abbr AND wl.reporter_standard IS NOT NULL
  JOIN moml.volumes v ON v.psmid = cu.moml_treatise
  JOIN moml.treatises t ON t.bibliographicid = v.bibliographicid
  WHERE cl.status LIKE 'linked%'
)
SELECT reporter_standard, source,
       source || ':' || COALESCE(cap_case_id::text, er_case_id, code_reporter_id::text, stub_cite) AS case_key,
       cap_case_id, er_case_id, code_reporter_id, stub_cite,
       count(DISTINCT bibliographicid) AS editions,
       count(*)                        AS cites
FROM links
GROUP BY reporter_standard, source, cap_case_id, er_case_id, code_reporter_id, stub_cite
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW moml_citations.reporter_case_citations IS 'For each reporter standard and each case reached through a citation to it, the treatise editions and citations behind the link (issue #305). Refreshed by make db-maintenance.';

CREATE UNIQUE INDEX IF NOT EXISTS reporter_case_citations_uq
  ON moml_citations.reporter_case_citations (reporter_standard, case_key);
CREATE INDEX IF NOT EXISTS reporter_case_citations_editions_idx
  ON moml_citations.reporter_case_citations (reporter_standard, editions DESC);
