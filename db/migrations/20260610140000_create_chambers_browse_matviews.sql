-- migrate:up
SET ROLE = law_admin;

-- Precomputed aggregates for the chambers browse pages (/treatises, /cases,
-- /normalized). Each of these pages ranks or counts over the ~62M-row
-- moml_citations.citations_unlinked and moml_citations.citation_links tables;
-- aggregating them on every request takes 90+ seconds. These materialized
-- views move that work off the request path. They are built WITH NO DATA and
-- refreshed manually with `make db-maintenance` (db/maintenance.sh discovers
-- every materialized view automatically, so no script edit is needed). See the
-- note before migrate:down for manual REFRESH commands.

-- Per-treatise citation counts. For each MOML treatise volume (psmid, stored as
-- citations_unlinked.moml_treatise), counts total detected citations and splits
-- them into linked (any 'linked_%' status) and not-linked (no_match, skipped_%,
-- or no link row). The /treatises list sums these over each work's volumes; the
-- /treatise detail reads per-volume counts directly.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.treatise_citation_counts AS
SELECT
  cu.moml_treatise,
  count(*) AS n,
  count(*) FILTER (WHERE cl.status LIKE 'linked_%') AS linked,
  count(*) FILTER (WHERE cl.status IS NULL OR cl.status NOT LIKE 'linked_%') AS not_linked
FROM moml_citations.citations_unlinked cu
LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
GROUP BY cu.moml_treatise
WITH NO DATA;

-- Unique index on the grouping key (moml_treatise is the GROUP BY key, unique
-- by construction). Documents the invariant and keeps REFRESH ... CONCURRENTLY
-- available; also used to look up a single volume's counts.
CREATE UNIQUE INDEX IF NOT EXISTS treatise_citation_counts_uq
  ON moml_citations.treatise_citation_counts (moml_treatise);

-- Case citation counts across all three linked case sources, unified into one
-- ranking. source is 'cap', 'er' (English Reports), or 'code' (code reporter);
-- case_id is the source-table primary key cast to text so the three id types
-- (bigint, text, bigint) share one column. page_count counts each treatise page
-- once even if a case is cited several times on it (the user's "count one
-- citation per treatise page"); cite_count is the raw number of linked
-- citations. Powers /cases (ranked by page_count).
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.case_citation_counts AS
SELECT 'cap'::text AS source,
       cl.cap_case_id::text AS case_id,
       count(DISTINCT (cu.moml_treatise, cu.moml_page)) AS page_count,
       count(*) AS cite_count
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
WHERE cl.status = 'linked_cap'
GROUP BY cl.cap_case_id
UNION ALL
SELECT 'er'::text,
       cl.er_case_id,
       count(DISTINCT (cu.moml_treatise, cu.moml_page)),
       count(*)
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
WHERE cl.status = 'linked_english_reports'
GROUP BY cl.er_case_id
UNION ALL
SELECT 'code'::text,
       cl.code_reporter_id::text,
       count(DISTINCT (cu.moml_treatise, cu.moml_page)),
       count(*)
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
WHERE cl.status = 'linked_code_reporter'
GROUP BY cl.code_reporter_id
WITH NO DATA;

-- Unique index on (source, case_id): each case id is distinct within its source
-- by GROUP BY, and source distinguishes the three unions.
CREATE UNIQUE INDEX IF NOT EXISTS case_citation_counts_uq
  ON moml_citations.case_citation_counts (source, case_id);

-- Supports ranked reads / pagination (ORDER BY page_count DESC).
CREATE INDEX IF NOT EXISTS case_citation_counts_page_count_idx
  ON moml_citations.case_citation_counts (page_count DESC);

-- Normalized-citation counts. Groups every link attempt by its normalized
-- citation string (cite_normalized, e.g. "2 Haw. 63"); a normalized string can
-- mix linked and not-linked instances. cite_count is total occurrences,
-- linked_count how many were linked, and page_count the number of distinct
-- treatise pages the normalized citation appears on. Powers /normalized.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.normalized_citation_counts AS
SELECT
  cl.cite_normalized,
  count(*) AS cite_count,
  count(*) FILTER (WHERE cl.status LIKE 'linked_%') AS linked_count,
  count(DISTINCT (cu.moml_treatise, cu.moml_page)) AS page_count
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
WHERE cl.cite_normalized IS NOT NULL
GROUP BY cl.cite_normalized
WITH NO DATA;

-- Unique index on the grouping key (cite_normalized is the GROUP BY key).
CREATE UNIQUE INDEX IF NOT EXISTS normalized_citation_counts_uq
  ON moml_citations.normalized_citation_counts (cite_normalized);

-- Supports ranked reads / pagination (ORDER BY cite_count DESC).
CREATE INDEX IF NOT EXISTS normalized_citation_counts_count_idx
  ON moml_citations.normalized_citation_counts (cite_count DESC);

-- Supports anchored prefix search on the normalized string (LIKE 'foo%') from
-- the /normalized search box.
CREATE INDEX IF NOT EXISTS normalized_citation_counts_prefix_idx
  ON moml_citations.normalized_citation_counts (cite_normalized text_pattern_ops);

-- Index on citation_links.cite_normalized so the /normalized/cite detail page
-- can list every instance of one normalized string without scanning the whole
-- ~62M-row table. This index build is the heavy part of this migration (it
-- scans the full table once); it is a one-time cost at migration time.
CREATE INDEX IF NOT EXISTS citation_links_cite_normalized_idx
  ON moml_citations.citation_links (cite_normalized);

-- After this migration the three views are empty. Populate them (and refresh the
-- existing dashboard views) with:
--   make db-refresh
-- or individually:
--   REFRESH MATERIALIZED VIEW moml_citations.treatise_citation_counts;
--   REFRESH MATERIALIZED VIEW moml_citations.case_citation_counts;
--   REFRESH MATERIALIZED VIEW moml_citations.normalized_citation_counts;

-- migrate:down
SET ROLE = law_admin;
DROP INDEX IF EXISTS moml_citations.citation_links_cite_normalized_idx;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.normalized_citation_counts;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.case_citation_counts;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.treatise_citation_counts;
