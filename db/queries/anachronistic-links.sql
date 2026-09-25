-- Links to a case decided after the treatise that cites it was published
-- (issue #319).
--
-- The treatise's year is its volume's moml.volumes.year. The case's year is
-- cap.cases.decision_year, legalhist.code_reporter.decision_year, or for the
-- English Reports murrell_year, falling back to er_year. A treatise and a case
-- of the same year are not anachronistic, and a link with either year unknown
-- is not counted. This is the same test the linker applies, so once
-- citation_links has been rebuilt by a linker that refuses these links, query 1
-- returns 0 in the anachronistic column and query 2 counts the refusals.
--
-- Read-only; run with LAW_CLAUDE. Query 1 takes a few minutes over the full
-- citation_links table. On the links built 2026-09-07, before the rule and
-- before PR #329, it found 414,889 of 27,439,100 CAP and English Reports links
-- anachronistic (1.51%): CAP 396,132 (1.93%), English Reports 18,757 (0.27%)
-- measured by murrell_year alone. The er_year fallback adds 1,123.
SET statement_timeout = '30min';

-- 1. Anachronistic links by source and tier, with how far the case postdates
--    the treatise.
WITH linked AS (
  SELECT 'cap' AS source, cl.match_tier, v.year AS treatise_year, c.decision_year AS case_year
  FROM moml_citations.citation_links cl
  JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
  LEFT JOIN moml.volumes v ON v.psmid = cu.moml_treatise
  JOIN cap.cases c ON c.id = cl.cap_case_id
  WHERE cl.status = 'linked_cap'
  UNION ALL
  SELECT 'code', cl.match_tier, v.year, cr.decision_year
  FROM moml_citations.citation_links cl
  JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
  LEFT JOIN moml.volumes v ON v.psmid = cu.moml_treatise
  JOIN legalhist.code_reporter cr ON cr.id = cl.code_reporter_id
  WHERE cl.status = 'linked_code_reporter'
  UNION ALL
  SELECT 'er', cl.match_tier, v.year, coalesce(e.murrell_year, e.er_year)
  FROM moml_citations.citation_links cl
  JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
  LEFT JOIN moml.volumes v ON v.psmid = cu.moml_treatise
  JOIN english_reports.cases e ON e.id = cl.er_case_id
  WHERE cl.status = 'linked_english_reports'
)
SELECT source, match_tier,
       count(*) AS links,
       count(*) FILTER (WHERE treatise_year IS NULL) AS no_treatise_year,
       count(*) FILTER (WHERE treatise_year < case_year) AS anachronistic,
       round(100.0 * count(*) FILTER (WHERE treatise_year < case_year) / count(*), 2) AS pct,
       count(*) FILTER (WHERE case_year - treatise_year = 1) AS gap_1,
       count(*) FILTER (WHERE case_year - treatise_year BETWEEN 2 AND 5) AS gap_2_5,
       count(*) FILTER (WHERE case_year - treatise_year > 5) AS gap_6plus
FROM linked
GROUP BY GROUPING SETS ((source, match_tier), (source), ())
ORDER BY source NULLS LAST, match_tier NULLS FIRST;

-- 2. The refusals the linker recorded, by reporter: the triage view for the
--    whitelist conflations the rule exposes, where a spelling lands in a
--    reporter of the wrong era.
SELECT wl.reporter_standard, cl.match_tier, count(*) AS refused
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr
WHERE cl.match_tier IN ('us_anachronistic', 'uk_anachronistic')
GROUP BY 1, 2
ORDER BY 3 DESC
LIMIT 50;
