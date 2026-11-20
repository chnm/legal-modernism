-- Editions the database holds in more than one copy (issue #402).
--
-- One row per copy, grouped under its edition, with where it was scanned,
-- its year and imprint, and the citations detected and linked in it, so two
-- scans of one book can be compared. The official copy, the one moml.treatises
-- and everything that reads it counts, comes first. Read-only; run with
-- LAW_CLAUDE.

WITH copies AS (
  SELECT c.edition, c.bibliographicid, c.official, c.source, c.work_id
  FROM moml.edition_copies c
  WHERE c.edition IN (SELECT copy_of FROM moml.editions WHERE copy_of IS NOT NULL)
),
vols AS (
  SELECT v.bibliographicid, min(v.year) AS year, min(v.imprint) AS imprint,
         sum(v.total_pages) AS pages,
         coalesce(sum(t.n), 0) AS citations, coalesce(sum(t.linked), 0) AS linked
  FROM moml.volumes v
  LEFT JOIN moml_citations.treatise_citation_counts t ON t.moml_treatise = v.psmid
  WHERE v.bibliographicid IN (SELECT bibliographicid FROM copies)
  GROUP BY v.bibliographicid
)
SELECT c.work_id, c.edition, c.bibliographicid, c.official, c.source,
       v.year, v.pages, v.citations, v.linked,
       round(100.0 * v.linked / nullif(v.citations, 0), 1) AS pct_linked,
       v.imprint
FROM copies c
JOIN vols v USING (bibliographicid)
ORDER BY c.work_id, c.edition, c.official DESC, v.year, c.bibliographicid;
