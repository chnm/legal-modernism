-- Rebuild legalhist.stub_cases from the linker's misses (issue #248).
--
-- Run with `make db-stubs` after cite-linker has finished. It needs write
-- access (LAW_DBSTR), takes a few minutes -- two passes over the join of
-- citation_links and citations_unlinked -- and is safe to repeat: rows are
-- upserted on their cite string, counts are rewritten in place, created_at
-- survives, and only rows that no longer qualify are deleted.
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
--      applies when it probes.
--
--   3. It recurs at least :threshold times across the corpus (default 10, set
--      with psql -v threshold=N). Measured on the covered reporters, where
--      the truth is known, 80% of distinct strings cited ten or more times
--      are first pages of cases (US 90%, UK 55%) against 67% at five; the
--      threshold trades that purity against coverage, which is 68% of the
--      eligible pool at ten and 82% at five. n_treatises is stored so a
--      stricter test can be evaluated without a rebuild.
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
    \set threshold 10
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

-- 2. Every qualifying cite string in those reporters, with its counts. The
--    treatise years come from moml.book_info, keyed by psmid, which is what
--    citations_unlinked.moml_treatise holds.
CREATE TEMP TABLE stub_candidates ON COMMIT DROP AS
SELECT format('%s %s %s', v.volume, wl.reporter_standard, cu.page) AS cite,
       wl.reporter_standard,
       v.volume,
       cu.page,
       count(*)::integer                        AS n_citations,
       count(DISTINCT cu.moml_treatise)::integer AS n_treatises,
       min(bi.year)                             AS first_cited_year,
       max(bi.year)                             AS last_cited_year
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr AND wl.junk = false
JOIN legalhist.reporters r ON r.reporter_standard = wl.reporter_standard
JOIN stub_eligible_reporters e ON e.reporter_standard = wl.reporter_standard
CROSS JOIN LATERAL (
    SELECT CASE WHEN coalesce(r.single_vol, false)
                THEN coalesce(cu.volume, 1)
                ELSE cu.volume END AS volume
) v
LEFT JOIN moml.book_info bi ON bi.psmid = cu.moml_treatise
WHERE (cl.status = 'linked_stub'
       OR (cl.status = 'no_match'
           AND cl.match_tier IN ('us_reporter_absent', 'uk_reporter_absent')))
  AND coalesce(r.type, '') <> 'statute'
  AND cu.page > 0
  AND v.volume > 0
GROUP BY wl.reporter_standard, v.volume, cu.page
HAVING count(*) >= :threshold;

-- 3. Upsert. A row whose counts have not changed is left alone, so updated_at
--    means "the counts last changed", and xmax = 0 tells an insert from an
--    update in the RETURNING set.
WITH upserted AS (
    INSERT INTO legalhist.stub_cases AS s
        (cite, reporter_standard, volume, page,
         n_citations, n_treatises, first_cited_year, last_cited_year)
    SELECT cite, reporter_standard, volume, page,
           n_citations, n_treatises, first_cited_year, last_cited_year
    FROM stub_candidates
    ON CONFLICT (cite) DO UPDATE
       SET n_citations      = EXCLUDED.n_citations,
           n_treatises      = EXCLUDED.n_treatises,
           first_cited_year = EXCLUDED.first_cited_year,
           last_cited_year  = EXCLUDED.last_cited_year,
           updated_at       = now()
     WHERE (s.n_citations, s.n_treatises, s.first_cited_year, s.last_cited_year)
           IS DISTINCT FROM
           (EXCLUDED.n_citations, EXCLUDED.n_treatises, EXCLUDED.first_cited_year, EXCLUDED.last_cited_year)
    RETURNING (xmax = 0) AS inserted
)
SELECT count(*) FILTER (WHERE inserted)                    AS inserted,
       count(*) FILTER (WHERE NOT inserted)                AS updated,
       (SELECT count(*) FROM stub_candidates) - count(*)   AS unchanged
FROM upserted;

-- 4. Prune the rows that no longer qualify: below the threshold now, or in a
--    reporter that has since gained a source. Any linked_stub rows pointing
--    at them are stale until the next full linker rebuild.
WITH pruned AS (
    DELETE FROM legalhist.stub_cases s
    WHERE NOT EXISTS (SELECT 1 FROM stub_candidates c WHERE c.cite = s.cite)
    RETURNING 1
)
SELECT count(*) AS pruned FROM pruned;

COMMIT;

ANALYZE legalhist.stub_cases;

SELECT count(*)                          AS stubs,
       sum(n_citations)                  AS citations,
       count(DISTINCT reporter_standard) AS reporters,
       min(n_citations)                  AS min_citations,
       max(n_citations)                  AS max_citations
FROM legalhist.stub_cases;
