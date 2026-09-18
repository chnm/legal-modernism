-- Rebuild legalhist.stub_cases from the linker's misses (issue #248).
--
-- Run with `make db-stubs` after cite-linker has finished. It needs write
-- access (LAW_DBSTR), takes a few minutes -- two passes over the join of
-- citation_links and citations_unlinked -- and is safe to repeat: a cite
-- string that already has a row is left alone, created_at survives, and only
-- rows that no longer qualify are deleted.
--
-- A stub is a cite string, "{volume} {reporter_standard} {page}", that meets
-- three tests:
--
--   1. Its reporter has no target dataset. Every citation the linker
--      processed for the reporter is either a stub link or a no_match at the
--      reporter_absent tier -- no probed spelling of the reporter appears in
--      CAP, the FreeLaw crosswalk, the code reporter, or the English Reports.
--      A reporter with a single real link, or a single miss at a deeper tier
--      (volume_absent, page_absent), is a coverage problem for that source,
--      not a candidate for stubs; #248 restricts stubs to reporters we will
--      never have a dataset for, so that pin cites into cases we do hold are
--      never minted as new cases.
--
--   2. It carries a volume and a page. A volume-less citation to a
--      multi-volume reporter identifies nothing; for a single-volume reporter
--      the detected volume-less form and the volume-1 form are the same cite,
--      and are written as volume 1, the equivalence the linker's volumeForms
--      applies when it probes. For a reporter cited by year
--      (legalhist.reporters.cited_by_year_from, issues #312 and #314) the
--      year is part of the key when the citation carries one from that year
--      on, "[1905] 2 K.B. 1", because the volume restarts every year, and a
--      year-cited citation may have no volume at all, "[1893] L.R.A.C. 22".
--      A year before cited_by_year_from is decoration on a volume-cited
--      citation and stays out of the key. A citation of such a reporter
--      detected without its year keeps the plain key, and collapses across
--      the years as before, which is visible as a stub without a year.
--
--   3. It recurs at least :threshold times across the corpus (default 5, set
--      with psql -v threshold=N). Measured on the covered reporters, where
--      the truth is known, 67% of distinct strings cited five or more times
--      are first pages of cases (US 82%, UK 45%) against 80% at ten; the
--      threshold trades that purity against coverage, which is 82% of the
--      eligible pool at five and 68% at ten. Five was chosen on 2026-09-08
--      for the coverage. db/queries/stub-case-threshold.sql is the sizing.
--
-- The registry records identity only (issue #320): the cite string, the
-- reporter, year, volume and page it was built from, and when the row was
-- minted. How often a stub is cited is not stored; it is derived from
-- citation_links whenever it is wanted. What is known about the case beyond
-- its cite -- party names, year decided, jurisdiction -- lives in
-- legalhist.stub_case_metadata, which this script never writes. Its foreign
-- key onto stub_cases means an annotated stub cannot be deleted, so the prune
-- below leaves annotated stubs in place even when they fall below the
-- threshold or their reporter gains a source: metadata settles directly the
-- question the threshold only estimates, and the linker never consults the
-- registry for a reporter that has a source, so such a row is inert.
--
-- The pipeline is linker -> db-stubs -> truncate-and-relink: the registry is
-- built from misses, and the linker then links those citations to it under
-- status linked_stub on its next full rebuild. The eligibility test counts
-- linked_stub rows on the reporter's side of the ledger, so a reporter does
-- not lose its stubs for having been linked to them. A routine incremental
-- linker run links new citations to the existing registry without a rebuild.

\set ON_ERROR_STOP on
\if :{?threshold}
\else
    \set threshold 5
\endif
\echo Rebuilding legalhist.stub_cases with threshold :threshold

SET statement_timeout = '1h';

BEGIN;

-- 1. Reporters with no target dataset: every processed citation is a stub
--    link or a reporter_absent miss. Skipped statuses carry no tier and say
--    nothing about coverage, so they are left out of the test; a NULL tier on
--    a no_match row (none exist after the 2026-07 tier migration, but the
--    column is nullable) counts against the reporter rather than being
--    ignored, since bool_and would otherwise skip it.
CREATE TEMP TABLE stub_eligible_reporters ON COMMIT DROP AS
SELECT wl.reporter_standard
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr
WHERE wl.junk = false
  AND cl.status IN ('linked_cap', 'linked_code_reporter', 'linked_english_reports',
                    'linked_stub', 'no_match')
GROUP BY wl.reporter_standard
HAVING bool_and(
    cl.status = 'linked_stub'
    OR (cl.status = 'no_match'
        AND coalesce(cl.match_tier IN ('us_reporter_absent', 'uk_reporter_absent'), false))
);

-- 2. Every qualifying cite string in those reporters. n_citations is what the
--    threshold applies to and what the summary reports; it is not stored.
CREATE TEMP TABLE stub_candidates ON COMMIT DROP AS
SELECT concat_ws(' ',
                 CASE WHEN v.year IS NOT NULL THEN format('[%s]', v.year) END,
                 v.volume, wl.reporter_standard, cu.page) AS cite,
       wl.reporter_standard,
       v.year,
       v.volume,
       cu.page,
       count(*)::integer AS n_citations
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr AND wl.junk = false
JOIN legalhist.reporters r ON r.reporter_standard = wl.reporter_standard
JOIN stub_eligible_reporters e ON e.reporter_standard = wl.reporter_standard
CROSS JOIN LATERAL (
    SELECT CASE WHEN coalesce(r.single_vol, false)
                THEN coalesce(cu.volume, 1)
                ELSE cu.volume END AS volume,
           CASE WHEN cu.year >= r.cited_by_year_from
                THEN cu.year END AS year
) v
WHERE (cl.status = 'linked_stub'
       OR (cl.status = 'no_match'
           AND cl.match_tier IN ('us_reporter_absent', 'uk_reporter_absent')))
  AND coalesce(r.type, '') <> 'statute'
  AND cu.page > 0
  AND (v.volume > 0 OR (v.volume IS NULL AND v.year IS NOT NULL))
GROUP BY wl.reporter_standard, v.year, v.volume, cu.page
HAVING count(*) >= :threshold;

SELECT count(*)                          AS candidates,
       sum(n_citations)                  AS citations,
       count(DISTINCT reporter_standard) AS reporters
FROM stub_candidates;

-- 3. Insert the new strings. An existing row is left untouched: there is
--    nothing on it to update, and created_at should keep the date the stub
--    was first minted.
WITH inserted AS (
    INSERT INTO legalhist.stub_cases (cite, reporter_standard, year, volume, page)
    SELECT cite, reporter_standard, year, volume, page
    FROM stub_candidates
    ON CONFLICT (cite) DO NOTHING
    RETURNING 1
)
SELECT count(*)                                            AS inserted,
       (SELECT count(*) FROM stub_candidates) - count(*)   AS existing
FROM inserted;

-- 4. Prune the rows that no longer qualify: below the threshold now, or in a
--    reporter that has since gained a source. Any linked_stub rows pointing
--    at them are stale until the next full linker rebuild. A stub with
--    metadata is kept regardless -- its foreign key would reject the delete,
--    and it is kept on purpose (see the header) -- and counted separately.
WITH pruned AS (
    DELETE FROM legalhist.stub_cases s
    WHERE NOT EXISTS (SELECT 1 FROM stub_candidates c WHERE c.cite = s.cite)
      AND NOT EXISTS (SELECT 1 FROM legalhist.stub_case_metadata m WHERE m.cite = s.cite)
    RETURNING 1
)
SELECT count(*) AS pruned,
       (SELECT count(*)
          FROM legalhist.stub_cases s
          JOIN legalhist.stub_case_metadata m ON m.cite = s.cite
         WHERE NOT EXISTS (SELECT 1 FROM stub_candidates c WHERE c.cite = s.cite)) AS kept_annotated
FROM pruned;

COMMIT;

ANALYZE legalhist.stub_cases;

SELECT count(*)                            AS stubs,
       count(DISTINCT s.reporter_standard) AS reporters,
       count(m.cite)                       AS annotated
FROM legalhist.stub_cases s
LEFT JOIN legalhist.stub_case_metadata m ON m.cite = s.cite;
