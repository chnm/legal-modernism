-- One-off remediation for issue #250: delete linked rows that were created
-- from ambiguous lookup keys the linker no longer uses, so the next run
-- re-processes them honestly.
--
-- Background: LoadCAPCitations used SELECT DISTINCT ON (cite) with no ORDER
-- BY, and LoadCodeReporterCitations last-wrote duplicate official citations
-- into its map, so a cite belonging to several cases linked to an arbitrary
-- one. The fixed loaders drop ambiguous keys, but --reset deliberately
-- preserves linked_* rows, so the wrong rows already written must be deleted
-- explicitly. Rows deleted here leave citation_links entirely, so the linker's
-- anti-join picks them up without --reset; still run the linker with --reset
-- so existing no_match rows see the new alternate-spelling probes.
--
-- Run order (needs write access, i.e. LAW_DBSTR):
--   1. psql "$LAW_DBSTR" -f scripts/remediate-250-ambiguous-links.sql
--   2. run the cite-linker with --reset
--   3. make db-maintenance
--
-- Some deleted linked_cap rows will relink identically or better through the
-- FreeLaw crosswalk (its cluster grouping disambiguates some cites that are
-- ambiguous in cap.citations); the rest become honest no_match.

BEGIN;

-- linked_cap rows on cites that map to more than one CAP case (the old
-- DISTINCT ON picked one arbitrarily). Expected: ~353,745 rows as of
-- 2026-07-29.
DELETE FROM moml_citations.citation_links cl
USING (
  SELECT cite FROM cap.citations
  GROUP BY cite HAVING count(DISTINCT "case") > 1
) amb
WHERE cl.status = 'linked_cap'
  AND cl.cite_linked = amb.cite;

-- linked_code_reporter rows whose linked cite is not in the new unambiguous
-- merged official+parallel key set (a mirror of the rewritten
-- LoadCodeReporterCitations query). Expected: exactly 3,770 rows as of
-- 2026-07-29; every retained row already carries the id the new map assigns.
-- NOT EXISTS rather than NOT IN: NOT IN would silently delete nothing if the
-- subquery ever yielded a NULL.
WITH keys AS MATERIALIZED (
  SELECT cite
  FROM (
    SELECT official_citation AS cite, id FROM legalhist.code_reporter
    UNION ALL
    SELECT regexp_replace(trim(seg), '\s*\(\d{4}\)$', ''), id
    FROM legalhist.code_reporter,
         unnest(string_to_array(parallel_citation, ';')) AS seg
    WHERE parallel_citation IS NOT NULL
  ) t
  WHERE cite <> ''
  GROUP BY cite
  HAVING count(DISTINCT id) = 1
)
DELETE FROM moml_citations.citation_links cl
WHERE cl.status = 'linked_code_reporter'
  AND NOT EXISTS (SELECT 1 FROM keys WHERE keys.cite = cl.cite_linked);

COMMIT;
