-- How many citations a cite string must recur before it is treated as a case
-- (issue #248), and how often such a string is a case at all.
--
-- legalhist.stub_cases gives a record to cite strings in reporters no source
-- covers. Nothing checks those strings against a case list, because there is
-- none, so the threshold has to be sized from the reporters where a case list
-- does exist: on those, the linker knows whether a heavily cited string is the
-- first page of a case or a pin cite into one. Query 2 is that calibration and
-- query 3 tests whether pin cites could be folded into their case by page
-- distance. Query 1 is the yield side of the trade.
--
-- Read-only; run with LAW_CLAUDE. Each query takes a few minutes over the full
-- citation_links table. Numbers quoted in the migration and on #248 are from
-- 2026-09-06.
SET statement_timeout = '30min';

-- 1. The pool and the yield at each threshold. Eligibility is the test
--    db/stub_cases.sql applies: no real link and no miss deeper than
--    reporter_absent anywhere in the reporter.
WITH eligible AS (
  SELECT wl.reporter_standard
  FROM moml_citations.citation_links cl
  JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
  JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr
  WHERE wl.junk = false
    AND cl.status IN ('linked_cap', 'linked_code_reporter', 'linked_english_reports',
                      'linked_stub', 'no_match')
  GROUP BY wl.reporter_standard
  HAVING bool_and(cl.status = 'linked_stub'
                  OR (cl.status = 'no_match'
                      AND coalesce(cl.match_tier IN ('us_reporter_absent', 'uk_reporter_absent'), false)))
),
strings AS (
  SELECT wl.reporter_standard,
         CASE WHEN coalesce(r.single_vol, false) THEN coalesce(cu.volume, 1) ELSE cu.volume END AS volume,
         cu.page,
         count(*) AS n_citations,
         count(DISTINCT cu.moml_treatise) AS n_treatises
  FROM moml_citations.citation_links cl
  JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
  JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr AND wl.junk = false
  JOIN legalhist.reporters r ON r.reporter_standard = wl.reporter_standard
  JOIN eligible e ON e.reporter_standard = wl.reporter_standard
  WHERE (cl.status = 'linked_stub'
         OR (cl.status = 'no_match' AND cl.match_tier IN ('us_reporter_absent', 'uk_reporter_absent')))
    AND cu.page > 0
  GROUP BY 1, 2, 3
  HAVING (CASE WHEN coalesce(r.single_vol, false) THEN coalesce(cu.volume, 1) ELSE cu.volume END) > 0
),
thresholds AS (SELECT unnest(ARRAY[1, 2, 3, 5, 10, 15, 20, 30, 50]) AS threshold)
SELECT t.threshold,
       count(*) FILTER (WHERE s.n_citations >= t.threshold)                       AS stubs,
       sum(s.n_citations) FILTER (WHERE s.n_citations >= t.threshold)             AS citations,
       round(100.0 * sum(s.n_citations) FILTER (WHERE s.n_citations >= t.threshold)
             / sum(s.n_citations), 1)                                            AS pct_of_pool,
       count(*) FILTER (WHERE s.n_treatises >= t.threshold)                       AS stubs_by_treatises
FROM thresholds t CROSS JOIN strings s
GROUP BY t.threshold
ORDER BY t.threshold;

-- 2. Calibration on the covered reporters. For every distinct (reporter,
--    volume, page) that linked, the tier says whether the string was a case's
--    first page (cap_direct, er_direct, and the alternate/crosswalk hits) or an
--    interior page (cap_page_interior, er_page_interior). The share of first
--    pages among strings at or above each threshold is the best available
--    estimate of how many stubs at that threshold are distinct cases rather
--    than pin cites into a neighbour.
WITH linked AS (
  SELECT CASE WHEN r.jurisdiction LIKE 'uk%' THEN 'uk' ELSE 'us' END AS route,
         wl.reporter_standard, cu.volume, cu.page,
         count(*) AS n_citations,
         bool_or(cl.match_tier IN ('cap_page_interior', 'er_page_interior')) AS interior
  FROM moml_citations.citation_links cl
  JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
  JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr
  JOIN legalhist.reporters r ON r.reporter_standard = wl.reporter_standard
  WHERE cl.status IN ('linked_cap', 'linked_english_reports')
    AND cu.volume IS NOT NULL
  GROUP BY 1, 2, 3, 4
),
thresholds AS (SELECT unnest(ARRAY[3, 5, 10, 20, 50]) AS threshold)
SELECT l.route, t.threshold,
       count(*) FILTER (WHERE l.n_citations >= t.threshold) AS strings,
       round(100.0 * count(*) FILTER (WHERE l.n_citations >= t.threshold AND NOT l.interior)
             / count(*) FILTER (WHERE l.n_citations >= t.threshold), 1) AS pct_first_page
FROM thresholds t CROSS JOIN linked l
GROUP BY l.route, t.threshold
ORDER BY l.route, t.threshold;

-- 3. Could pin cites be folded into their case by distance? Among heavily cited
--    strings (>= 10) in the same covered volume, take each one with its nearest
--    heavily cited predecessor and ask whether they linked to the same case.
--    If proximity identified a pin cite, the share of same-case pairs at a gap
--    of one or two pages would be near 100%. It is not: cases in these
--    reporters are often shorter than the gap between two cited pages.
WITH hot AS (
  SELECT wl.reporter_standard, cu.volume, cu.page,
         min(coalesce(cl.cap_case_id::text, cl.er_case_id)) AS case_id,
         count(DISTINCT coalesce(cl.cap_case_id::text, cl.er_case_id)) AS cases
  FROM moml_citations.citation_links cl
  JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
  JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr
  WHERE cl.status IN ('linked_cap', 'linked_english_reports')
    AND cu.volume IS NOT NULL
  GROUP BY 1, 2, 3
  HAVING count(*) >= 10
),
pairs AS (
  SELECT page - lag(page) OVER w AS gap,
         case_id = lag(case_id) OVER w AS same_case
  FROM hot
  WHERE cases = 1
  WINDOW w AS (PARTITION BY reporter_standard, volume ORDER BY page)
)
SELECT CASE WHEN gap = 1 THEN '1' WHEN gap = 2 THEN '2' WHEN gap <= 5 THEN '3-5'
            WHEN gap <= 10 THEN '6-10' WHEN gap <= 20 THEN '11-20' ELSE '>20' END AS gap_pages,
       count(*)                                          AS pairs,
       round(100.0 * count(*) FILTER (WHERE same_case) / count(*), 1) AS pct_same_case
FROM pairs
WHERE gap IS NOT NULL
GROUP BY 1
ORDER BY min(gap);
