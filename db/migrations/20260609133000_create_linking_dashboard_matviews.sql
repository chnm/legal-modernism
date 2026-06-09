-- migrate:up
SET ROLE = law_admin;

-- Precomputed aggregates for the chambers linking dashboard
-- (/linking-dashboard). The dashboard previously aggregated the ~62M-row
-- moml_citations.citations_unlinked and ~61M-row moml_citations.citation_links
-- tables on every page load, which took over a minute and timed out in the
-- browser. These materialized views move that work off the request path; the
-- cite-linker refreshes them at the start and end of each run, alongside
-- moml_citations.citations_unmatched_top. Both are built WITH NO DATA; run
-- REFRESH MATERIALIZED VIEW to populate (see note before migrate:down).

-- Per-reporter linking breakdown. For each whitelisted (non-junk) standard
-- reporter, counts how many of its raw citations are linked, ended in
-- 'no_match', or have no link attempt yet (unprocessed), plus a flag for UK
-- jurisdiction reporters. Mirrors the query the dashboard formerly ran inline.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.linking_dashboard_reporters AS
SELECT
  wl.reporter_standard,
  count(*) FILTER (WHERE cl.status LIKE 'linked%') AS linked,
  count(*) FILTER (WHERE cl.status = 'no_match')   AS no_match,
  count(*) FILTER (WHERE cl.status IS NULL)         AS unprocessed,
  COALESCE(bool_or(r.jurisdiction LIKE 'uk:%'), false) AS uk
FROM moml_citations.citations_unlinked cu
JOIN legalhist.whitelist wl ON cu.reporter_abbr = wl.reporter_found
JOIN legalhist.reporters r ON r.reporter_standard = wl.reporter_standard
LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
WHERE wl.reporter_standard IS NOT NULL
  AND wl.junk = false
GROUP BY wl.reporter_standard
WITH NO DATA;

-- Unique index on the grouping key (reporter_standard is the GROUP BY key, so
-- it is unique by construction). Documents the invariant and keeps the option
-- of a manual REFRESH MATERIALIZED VIEW CONCURRENTLY open.
CREATE UNIQUE INDEX IF NOT EXISTS linking_dashboard_reporters_uq
  ON moml_citations.linking_dashboard_reporters (reporter_standard);

-- Dashboard summary metrics as key/value rows. Captures the total count of raw
-- detected citations plus the count of citation_links in each status
-- (no_match, linked_cap, linked_english_reports, linked_code_reporter,
-- skipped_not_whitelisted, skipped_junk). The dashboard reads this single small
-- view instead of running a count over citations_unlinked and a GROUP BY over
-- citation_links on every load.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.linking_dashboard_summary AS
SELECT 'total_raw_cites'::text AS metric, count(*) AS n
  FROM moml_citations.citations_unlinked
UNION ALL
SELECT status AS metric, count(*) AS n
  FROM moml_citations.citation_links
 GROUP BY status
WITH NO DATA;

-- Unique index on the metric key. Each metric appears once by construction.
CREATE UNIQUE INDEX IF NOT EXISTS linking_dashboard_summary_uq
  ON moml_citations.linking_dashboard_summary (metric);

-- After this migration both views are empty. The cite-linker refreshes them at
-- the start and end of each run, so they will be populated the next time
-- cite-linker runs. To populate them manually in the meantime:
--   REFRESH MATERIALIZED VIEW moml_citations.linking_dashboard_reporters;
--   REFRESH MATERIALIZED VIEW moml_citations.linking_dashboard_summary;

-- migrate:down
DROP MATERIALIZED VIEW IF EXISTS moml_citations.linking_dashboard_summary;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.linking_dashboard_reporters;
