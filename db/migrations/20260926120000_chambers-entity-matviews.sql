-- migrate:up
SET ROLE = law_admin;

-- Materialized views behind the chambers redesign (issue #305). Chambers
-- browses the corpus by its entities -- works, editions, pages, cases,
-- reporters, citations -- and every list or ranking it shows is an aggregate
-- over the 56M detected citations or the 20M edition-to-case rows of
-- edition_case_citations, far too much to compute on request. These views hold
-- those aggregates. All are built WITH NO DATA and refreshed, in dependency
-- order, by `make db-maintenance` (db/maintenance.sh). Timings are from
-- read-only runs of each SELECT on 2026-09-25.

-- pg_trgm is a trusted extension in PostgreSQL 17, so law_admin may install it
-- without superuser rights. It backs the case-name search on case_edition_counts.
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

-- One row per treatise edition: moml.treatises, materialized, with the
-- edition's work and its citation totals. moml.treatises is a view over
-- regexes and subject joins that costs about a second to scan (it is 0.6 ms to
-- look one edition up by id), so every list and every per-case join in chambers
-- reads this instead. cites and linked are summed from treatise_citation_counts
-- over the edition's volumes; cases is the number of distinct cases the edition
-- cites, and cases_not_pincite_only those it reaches other than only through
-- pin cites (edition_case_citations.pincite_only). Level 1: reads two
-- materialized views. Build: about a second; 16,139 rows.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.edition_citation_counts AS
WITH vols AS (
  SELECT v.bibliographicid,
         count(*)                     AS volumes,
         sum(v.total_pages)::bigint           AS pages,
         COALESCE(sum(tcc.n), 0)::bigint      AS cites,
         COALESCE(sum(tcc.linked), 0)::bigint AS linked
  FROM moml.volumes v
  LEFT JOIN moml_citations.treatise_citation_counts tcc ON tcc.moml_treatise = v.psmid
  GROUP BY v.bibliographicid
),
cases AS (
  SELECT bibliographicid,
         count(*)                                 AS cases,
         count(*) FILTER (WHERE NOT pincite_only) AS cases_not_pincite_only
  FROM moml_citations.edition_case_citations
  GROUP BY bibliographicid
)
SELECT t.bibliographicid,
       e.work_id,
       t.jurisdiction,
       t.year,
       t.title,
       e.author,
       t.vols,
       vols.volumes,
       vols.pages,
       e.derivative,
       vols.cites,
       vols.linked,
       COALESCE(cases.cases, 0)                  AS cases,
       COALESCE(cases.cases_not_pincite_only, 0) AS cases_not_pincite_only
FROM moml.treatises t
JOIN moml.editions e ON e.bibliographicid = t.bibliographicid
JOIN vols ON vols.bibliographicid = t.bibliographicid
LEFT JOIN cases ON cases.bibliographicid = t.bibliographicid
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW moml_citations.edition_citation_counts IS 'One row per treatise edition (moml.treatises, materialized) with its work, jurisdiction, year, title, volumes, pages, detected and linked citations, and the distinct cases it cites (issue #305). Refreshed by make db-maintenance.';
COMMENT ON COLUMN moml_citations.edition_citation_counts.cases_not_pincite_only IS 'Cases the edition reaches other than only through pin cites (edition_case_citations.pincite_only is false).';

CREATE UNIQUE INDEX IF NOT EXISTS edition_citation_counts_uq
  ON moml_citations.edition_citation_counts (bibliographicid);
CREATE INDEX IF NOT EXISTS edition_citation_counts_work_idx
  ON moml_citations.edition_citation_counts (work_id);
CREATE INDEX IF NOT EXISTS edition_citation_counts_cites_idx
  ON moml_citations.edition_citation_counts (cites DESC);

-- One row per work with at least one treatise edition: how many of its
-- editions are treatises (editions) out of all its editions (all_editions), the
-- span of their years, their jurisdictions, and their citation totals. cases is
-- the number of distinct cases cited across those editions. Level 2: reads
-- edition_citation_counts. Build: seconds; 10,700 rows.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.work_citation_counts AS
WITH cases AS (
  SELECT ecc.work_id,
         count(DISTINCT c.source || ':' || COALESCE(c.cap_case_id::text, c.er_case_id,
                                                    c.code_reporter_id::text, c.stub_cite)) AS cases
  FROM moml_citations.edition_case_citations c
  JOIN moml_citations.edition_citation_counts ecc ON ecc.bibliographicid = c.bibliographicid
  GROUP BY ecc.work_id
),
all_editions AS (
  SELECT work_id, count(*) AS n FROM moml.editions GROUP BY work_id
)
SELECT ecc.work_id,
       count(*)                                    AS editions,
       ae.n                                        AS all_editions,
       count(*) FILTER (WHERE ecc.derivative)      AS derivative_editions,
       min(ecc.year)                               AS first_year,
       max(ecc.year)                               AS last_year,
       bool_or(ecc.jurisdiction = 'US')            AS us,
       bool_or(ecc.jurisdiction = 'UK')            AS uk,
       sum(ecc.cites)::bigint                      AS cites,
       sum(ecc.linked)::bigint                     AS linked,
       COALESCE(max(cases.cases), 0)               AS cases
FROM moml_citations.edition_citation_counts ecc
JOIN all_editions ae ON ae.work_id = ecc.work_id
LEFT JOIN cases ON cases.work_id = ecc.work_id
GROUP BY ecc.work_id, ae.n
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW moml_citations.work_citation_counts IS 'One row per work with at least one treatise edition: its treatise editions, all its editions, the span of years, jurisdictions, citation totals and distinct cases cited (issue #305). Refreshed by make db-maintenance.';

CREATE UNIQUE INDEX IF NOT EXISTS work_citation_counts_uq
  ON moml_citations.work_citation_counts (work_id);
CREATE INDEX IF NOT EXISTS work_citation_counts_cites_idx
  ON moml_citations.work_citation_counts (cites DESC);

-- Cases ranked by the treatise editions that cite them: one row per case, from
-- edition_case_citations limited to treatise editions, with the case's display
-- name, year and citation copied in from its source so the ranking can be
-- listed and searched without joining four source tables. case_key is
-- source:id, the same key chambers puts in a URL. editions_not_pincite_only
-- leaves out the editions whose only citations were pin cites (issue #242);
-- works is how many different works the citing editions belong to, since a
-- work of many editions (Blackstone has 119) would otherwise weigh as many
-- times. A stub case has a name only once stub_case_metadata records one.
-- Level 2: reads edition_citation_counts. Build: about 40 seconds;
-- 1,303,455 rows.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.case_edition_counts AS
WITH agg AS (
  SELECT c.source, c.cap_case_id, c.er_case_id, c.code_reporter_id, c.stub_cite,
         count(*)                                   AS editions,
         count(*) FILTER (WHERE NOT c.pincite_only) AS editions_not_pincite_only,
         count(DISTINCT ecc.work_id)                AS works,
         sum(c.cite_count)::bigint                  AS cites,
         count(*) FILTER (WHERE ecc.jurisdiction = 'US') AS us_editions,
         min(ecc.year)                              AS first_cited,
         max(ecc.year)                              AS last_cited
  FROM moml_citations.edition_case_citations c
  JOIN moml_citations.edition_citation_counts ecc ON ecc.bibliographicid = c.bibliographicid
  GROUP BY c.source, c.cap_case_id, c.er_case_id, c.code_reporter_id, c.stub_cite
)
SELECT a.source,
       a.source || ':' || COALESCE(a.cap_case_id::text, a.er_case_id,
                                   a.code_reporter_id::text, a.stub_cite) AS case_key,
       a.cap_case_id, a.er_case_id, a.code_reporter_id, a.stub_cite,
       COALESCE(cc.name_abbreviation, er.murrell_title, er.er_name, code.name, sm.party_names) AS name,
       COALESCE(cc.decision_year, er.murrell_year, er.er_year, code.decision_year, sm.year_decided) AS year,
       COALESCE(capcite.cite, er.er_cite, code.official_citation, a.stub_cite) AS cite,
       a.editions, a.editions_not_pincite_only, a.works, a.cites,
       a.us_editions, a.first_cited, a.last_cited
FROM agg a
LEFT JOIN cap.cases cc ON cc.id = a.cap_case_id
LEFT JOIN LATERAL (
  SELECT ci.cite FROM cap.citations ci WHERE ci."case" = a.cap_case_id
  ORDER BY (ci.type = 'official') DESC, ci.cite LIMIT 1
) capcite ON true
LEFT JOIN english_reports.cases er ON er.id = a.er_case_id
LEFT JOIN legalhist.code_reporter code ON code.id = a.code_reporter_id
LEFT JOIN legalhist.stub_case_metadata sm ON sm.cite = a.stub_cite
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW moml_citations.case_edition_counts IS 'One row per case cited by treatise editions, with the case''s name, year and citation copied from its source, and the treatise editions, works and citations behind it (issue #305). case_key is source:id. Refreshed by make db-maintenance.';
COMMENT ON COLUMN moml_citations.case_edition_counts.editions_not_pincite_only IS 'Citing editions that reach the case other than only through pin cites.';
COMMENT ON COLUMN moml_citations.case_edition_counts.works IS 'Distinct works the citing editions belong to.';

CREATE UNIQUE INDEX IF NOT EXISTS case_edition_counts_key_uq
  ON moml_citations.case_edition_counts (case_key);
CREATE INDEX IF NOT EXISTS case_edition_counts_editions_idx
  ON moml_citations.case_edition_counts (editions DESC, case_key);
CREATE INDEX IF NOT EXISTS case_edition_counts_source_editions_idx
  ON moml_citations.case_edition_counts (source, editions DESC, case_key);
CREATE INDEX IF NOT EXISTS case_edition_counts_works_idx
  ON moml_citations.case_edition_counts (works DESC, case_key);
CREATE INDEX IF NOT EXISTS case_edition_counts_cites_idx
  ON moml_citations.case_edition_counts (cites DESC, case_key);
CREATE INDEX IF NOT EXISTS case_edition_counts_not_pincite_idx
  ON moml_citations.case_edition_counts (editions_not_pincite_only DESC, case_key);
CREATE INDEX IF NOT EXISTS case_edition_counts_cite_idx
  ON moml_citations.case_edition_counts (cite);
CREATE INDEX IF NOT EXISTS case_edition_counts_name_trgm_idx
  ON moml_citations.case_edition_counts USING gin (name gin_trgm_ops);

-- The reporter side of the edition-to-case edges: for each reporter (the
-- standard behind the spelling the treatise used, through the whitelist) and
-- each case reached by a citation to it, the treatise editions and citations.
-- A case is "reached through" a reporter, not "printed in" it: a parallel cite
-- reaches the same case through another reporter, and a stub case exists only
-- as a cite string. Level 0: a scan of the linked citations, like
-- edition_case_citations. Build: about 3 minutes; 1,682,718 rows.
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

-- Citations from each edition to each reporter, linked or not: an edition's
-- reporter profile, and the editions that cite a reporter most. Every
-- whitelisted, non-junk citation counts, whatever its link status. Not limited
-- to treatises, since the edition page serves any edition; join
-- edition_citation_counts for the treatise scope. Level 0: a scan of every
-- detected citation, like linking_dashboard_tiers. Build: about a minute;
-- 1,403,968 rows.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.edition_reporter_citations AS
SELECT v.bibliographicid,
       wl.reporter_standard,
       count(*)                                          AS cites,
       count(*) FILTER (WHERE cl.status LIKE 'linked%')  AS linked
FROM moml_citations.citations_unlinked cu
JOIN legalhist.whitelist wl
  ON wl.reporter_found = cu.reporter_abbr AND wl.reporter_standard IS NOT NULL
JOIN moml.volumes v ON v.psmid = cu.moml_treatise
LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
GROUP BY v.bibliographicid, wl.reporter_standard
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW moml_citations.edition_reporter_citations IS 'Citations from each MOML edition to each reporter standard, whitelisted and non-junk, with how many linked (issue #305). Refreshed by make db-maintenance.';

CREATE UNIQUE INDEX IF NOT EXISTS edition_reporter_citations_uq
  ON moml_citations.edition_reporter_citations (bibliographicid, reporter_standard);
CREATE INDEX IF NOT EXISTS edition_reporter_citations_reporter_idx
  ON moml_citations.edition_reporter_citations (reporter_standard, cites DESC);

-- The reporter page reads the unmatched cites of one reporter.
CREATE INDEX IF NOT EXISTS citations_unmatched_top_reporter_idx
  ON moml_citations.citations_unmatched_top (reporter_standard, n DESC);

-- The whitelist extender walks legalhist.top_reporters from the most frequent
-- spelling down until it has 250 the whitelist lacks; without this index that
-- is a sort of 3.6M rows on every request.
CREATE INDEX IF NOT EXISTS top_reporters_n_idx
  ON legalhist.top_reporters (n DESC);

-- The two views only the removed chambers pages read. case_citation_counts
-- (one row per case, all editions, no stubs) is superseded by
-- case_edition_counts; normalized_citation_counts backed the normalized
-- citations pages, whose lookup now runs on the citation_links.cite_normalized
-- index.
DROP MATERIALIZED VIEW IF EXISTS moml_citations.normalized_citation_counts;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.case_citation_counts;

-- After this migration the new views are empty. Populate them with
-- `make db-maintenance`.

-- migrate:down
SET ROLE = law_admin;

DROP INDEX IF EXISTS moml_citations.citations_unmatched_top_reporter_idx;
DROP INDEX IF EXISTS legalhist.top_reporters_n_idx;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.edition_reporter_citations;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.reporter_case_citations;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.case_edition_counts;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.work_citation_counts;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.edition_citation_counts;
DROP EXTENSION IF EXISTS pg_trgm;

CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.case_citation_counts AS
 SELECT 'cap'::text AS source,
    (cl.cap_case_id)::text AS case_id,
    count(DISTINCT ROW(cu.moml_treatise, cu.moml_page)) AS page_count,
    count(*) AS cite_count
   FROM (moml_citations.citation_links cl
     JOIN moml_citations.citations_unlinked cu ON ((cu.id = cl.citation_id)))
  WHERE (cl.status = 'linked_cap'::text)
  GROUP BY cl.cap_case_id
UNION ALL
 SELECT 'er'::text AS source,
    cl.er_case_id AS case_id,
    count(DISTINCT ROW(cu.moml_treatise, cu.moml_page)) AS page_count,
    count(*) AS cite_count
   FROM (moml_citations.citation_links cl
     JOIN moml_citations.citations_unlinked cu ON ((cu.id = cl.citation_id)))
  WHERE (cl.status = 'linked_english_reports'::text)
  GROUP BY cl.er_case_id
UNION ALL
 SELECT 'code'::text AS source,
    (cl.code_reporter_id)::text AS case_id,
    count(DISTINCT ROW(cu.moml_treatise, cu.moml_page)) AS page_count,
    count(*) AS cite_count
   FROM (moml_citations.citation_links cl
     JOIN moml_citations.citations_unlinked cu ON ((cu.id = cl.citation_id)))
  WHERE (cl.status = 'linked_code_reporter'::text)
  GROUP BY cl.code_reporter_id
  WITH NO DATA;

CREATE INDEX IF NOT EXISTS case_citation_counts_page_count_idx
  ON moml_citations.case_citation_counts USING btree (page_count DESC);
CREATE UNIQUE INDEX IF NOT EXISTS case_citation_counts_uq
  ON moml_citations.case_citation_counts USING btree (source, case_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.normalized_citation_counts AS
 SELECT cl.cite_normalized,
    count(*) AS cite_count,
    count(*) FILTER (WHERE (cl.status ~~ 'linked_%'::text)) AS linked_count,
    count(DISTINCT ROW(cu.moml_treatise, cu.moml_page)) AS page_count
   FROM (moml_citations.citation_links cl
     JOIN moml_citations.citations_unlinked cu ON ((cu.id = cl.citation_id)))
  WHERE (cl.cite_normalized IS NOT NULL)
  GROUP BY cl.cite_normalized
  WITH NO DATA;

CREATE INDEX IF NOT EXISTS normalized_citation_counts_count_idx
  ON moml_citations.normalized_citation_counts USING btree (cite_count DESC);
CREATE INDEX IF NOT EXISTS normalized_citation_counts_prefix_idx
  ON moml_citations.normalized_citation_counts USING btree (cite_normalized text_pattern_ops);
CREATE UNIQUE INDEX IF NOT EXISTS normalized_citation_counts_uq
  ON moml_citations.normalized_citation_counts USING btree (cite_normalized);
