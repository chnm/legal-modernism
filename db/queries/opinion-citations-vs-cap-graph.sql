-- Our links from the CAP opinions to CAP cases, set against CAP's own
-- citation graph (issue #74).
--
-- cite-detector-cap finds citations in the text of cap.opinions and
-- cite-linker-cap links them; opinion_citations.case_case_citations
-- aggregates the links to one row per citing case and cited case. CAP
-- publishes its own case-to-case graph, cap_citations.citations (cites_from,
-- cites_to), extracted from the same text by its own means. Where the two
-- agree, each corroborates the other; where they differ, one has found what
-- the other missed, or made something up, and the difference is what to look
-- at. Neither is the truth, so a low number on either side is a lead, not a
-- verdict.
--
-- Scope. A citing case counts when it was decided by 1920 (the detector's
-- --max-year), has at least one opinion, and is in both cap.cases and
-- cap_citations.metadata. metadata.id is the cap.cases.id space, but the two
-- do not coincide: measured 2026-09-26, 4,367 of metadata's 5,084,607 ids are
-- not in cap.cases, and of the 1,408,784 cases with cap.cases.decision_year
-- <= 1920 metadata holds 1,059,076 (it has 1,059,402 rows with its own
-- decision_year <= 1920). So about a quarter of the cases we detect over have
-- no CAP edges to compare against, and they are left out rather than counted
-- as ours-only. On the cited side both endpoints must be in cap.cases,
-- because our links can only point at cap.cases rows: a cites_to outside it
-- is unreachable for us and is left out of theirs.
--
-- ours is the CAP-to-CAP rows of case_case_citations, without self-cites (an
-- opinion citing its own case, which CAP's graph does not record either).
-- theirs is the distinct (cites_from, cites_to) pairs. Both are sets of edges:
-- a case that cites another twice is one edge on either side.
--
-- Read-only; run with LAW_CLAUDE. That role cannot create temporary tables,
-- so the scope and the two edge sets are computed again in each of queries 1
-- to 4; each pass over the 6.26M CAP edges from the pre-1921 cases takes a
-- few minutes. Needs case_case_citations refreshed (make db-maintenance)
-- after a cite-linker-cap run. Not yet measured: the CAP corpus has not been
-- detected or linked.
SET statement_timeout = '30min';

-- 1 and 2. Agreement between the two graphs, overall (the first row, with a
--    NULL year) and by the citing case's decision year. in_both: edges in
--    ours and in theirs; ours_only and theirs_only: in one and not the other.
--    recall = in_both / theirs, how much of CAP's graph we find; precision =
--    in_both / ours, how much of what we find CAP also has.
WITH scope AS (
  SELECT c.id, c.decision_year
  FROM cap.cases c
  JOIN cap_citations.metadata m ON m.id = c.id
  WHERE c.decision_year <= 1920
    AND EXISTS (SELECT 1 FROM cap.opinions o WHERE o."case" = c.id)
),
ours AS (
  SELECT ccc.citing_case_id AS citing, ccc.cap_case_id AS cited
  FROM opinion_citations.case_case_citations ccc
  JOIN scope s ON s.id = ccc.citing_case_id
  WHERE ccc.source = 'cap' AND NOT ccc.self_cite
),
theirs AS (
  SELECT DISTINCT cc.cites_from::bigint AS citing, cc.cites_to::bigint AS cited
  FROM cap_citations.citations cc
  JOIN scope s ON s.id = cc.cites_from
  WHERE EXISTS (SELECT 1 FROM cap.cases k WHERE k.id = cc.cites_to)
),
edges AS (
  SELECT coalesce(o.citing, t.citing) AS citing,
         coalesce(o.cited, t.cited) AS cited,
         o.citing IS NOT NULL AS in_ours,
         t.citing IS NOT NULL AS in_theirs
  FROM ours o
  FULL OUTER JOIN theirs t ON t.citing = o.citing AND t.cited = o.cited
)
SELECT s.decision_year,
       count(*) FILTER (WHERE in_ours AND in_theirs) AS in_both,
       count(*) FILTER (WHERE in_ours AND NOT in_theirs) AS ours_only,
       count(*) FILTER (WHERE in_theirs AND NOT in_ours) AS theirs_only,
       round(100.0 * count(*) FILTER (WHERE in_ours AND in_theirs)
             / nullif(count(*) FILTER (WHERE in_theirs), 0), 2) AS recall_pct,
       round(100.0 * count(*) FILTER (WHERE in_ours AND in_theirs)
             / nullif(count(*) FILTER (WHERE in_ours), 0), 2) AS precision_pct
FROM edges e
JOIN scope s ON s.id = e.citing
GROUP BY GROUPING SETS ((s.decision_year), ())
ORDER BY s.decision_year NULLS FIRST;

-- 3. Disagreement by the cited case's reporter. A reporter with many
--    theirs_only edges is one our detector or the whitelist misses (a
--    spelling the treatises never used, a renumbered series, a reporter cited
--    by year); one with many ours_only edges is where we find what CAP's
--    extractor did not, or link a cite to the wrong case.
WITH scope AS (
  SELECT c.id, c.decision_year
  FROM cap.cases c
  JOIN cap_citations.metadata m ON m.id = c.id
  WHERE c.decision_year <= 1920
    AND EXISTS (SELECT 1 FROM cap.opinions o WHERE o."case" = c.id)
),
ours AS (
  SELECT ccc.citing_case_id AS citing, ccc.cap_case_id AS cited
  FROM opinion_citations.case_case_citations ccc
  JOIN scope s ON s.id = ccc.citing_case_id
  WHERE ccc.source = 'cap' AND NOT ccc.self_cite
),
theirs AS (
  SELECT DISTINCT cc.cites_from::bigint AS citing, cc.cites_to::bigint AS cited
  FROM cap_citations.citations cc
  JOIN scope s ON s.id = cc.cites_from
  WHERE EXISTS (SELECT 1 FROM cap.cases k WHERE k.id = cc.cites_to)
),
edges AS (
  SELECT coalesce(o.citing, t.citing) AS citing,
         coalesce(o.cited, t.cited) AS cited,
         o.citing IS NOT NULL AS in_ours,
         t.citing IS NOT NULL AS in_theirs
  FROM ours o
  FULL OUTER JOIN theirs t ON t.citing = o.citing AND t.cited = o.cited
)
SELECT r.short_name AS reporter, r.id AS reporter_id,
       count(*) FILTER (WHERE in_ours AND in_theirs) AS in_both,
       count(*) FILTER (WHERE in_ours AND NOT in_theirs) AS ours_only,
       count(*) FILTER (WHERE in_theirs AND NOT in_ours) AS theirs_only
FROM edges e
JOIN cap.cases k ON k.id = e.cited
JOIN cap.reporters r ON r.id = k.reporter
GROUP BY r.id, r.short_name
ORDER BY count(*) FILTER (WHERE in_ours <> in_theirs) DESC, r.short_name
LIMIT 50;

-- 4. How the disagreement is spread over the citing cases: percentiles and
--    the maximum of each count per case, over every case in scope (a case
--    with no edge on either side counts as zeros). A few cases with many
--    ours_only edges point at a runaway detection, such as a list of
--    authorities in the text; the same total spread thinly is systematic.
WITH scope AS (
  SELECT c.id, c.decision_year
  FROM cap.cases c
  JOIN cap_citations.metadata m ON m.id = c.id
  WHERE c.decision_year <= 1920
    AND EXISTS (SELECT 1 FROM cap.opinions o WHERE o."case" = c.id)
),
ours AS (
  SELECT ccc.citing_case_id AS citing, ccc.cap_case_id AS cited
  FROM opinion_citations.case_case_citations ccc
  JOIN scope s ON s.id = ccc.citing_case_id
  WHERE ccc.source = 'cap' AND NOT ccc.self_cite
),
theirs AS (
  SELECT DISTINCT cc.cites_from::bigint AS citing, cc.cites_to::bigint AS cited
  FROM cap_citations.citations cc
  JOIN scope s ON s.id = cc.cites_from
  WHERE EXISTS (SELECT 1 FROM cap.cases k WHERE k.id = cc.cites_to)
),
edges AS (
  SELECT coalesce(o.citing, t.citing) AS citing,
         coalesce(o.cited, t.cited) AS cited,
         o.citing IS NOT NULL AS in_ours,
         t.citing IS NOT NULL AS in_theirs
  FROM ours o
  FULL OUTER JOIN theirs t ON t.citing = o.citing AND t.cited = o.cited
),
per_case AS (
  SELECT s.id,
         count(e.citing) FILTER (WHERE in_ours AND in_theirs) AS in_both,
         count(e.citing) FILTER (WHERE in_ours AND NOT in_theirs) AS ours_only,
         count(e.citing) FILTER (WHERE in_theirs AND NOT in_ours) AS theirs_only
  FROM scope s
  LEFT JOIN edges e ON e.citing = s.id
  GROUP BY s.id
)
SELECT count(*) AS citing_cases,
       count(*) FILTER (WHERE in_both + ours_only + theirs_only = 0) AS no_edges,
       count(*) FILTER (WHERE ours_only = 0 AND theirs_only = 0 AND in_both > 0) AS agreeing,
       percentile_cont(ARRAY[0.5, 0.9, 0.99]) WITHIN GROUP (ORDER BY in_both) AS in_both_p50_p90_p99,
       max(in_both) AS in_both_max,
       percentile_cont(ARRAY[0.5, 0.9, 0.99]) WITHIN GROUP (ORDER BY ours_only) AS ours_only_p50_p90_p99,
       max(ours_only) AS ours_only_max,
       percentile_cont(ARRAY[0.5, 0.9, 0.99]) WITHIN GROUP (ORDER BY theirs_only) AS theirs_only_p50_p90_p99,
       max(theirs_only) AS theirs_only_max
FROM per_case;

-- 5. The spellings the linker skipped as not whitelisted, by count: the
--    stopgap until an opinion_citations twin of legalhist.top_reporters
--    exists for the chambers whitelist page. A spelling high on this list is
--    a reporter the courts cite that the whitelist, built from the treatises,
--    does not know, or a court's spelling of one it does.
SELECT cu.reporter_abbr,
       count(*) AS citations,
       count(DISTINCT cu.cap_case) AS citing_cases
FROM opinion_citations.citation_links cl
JOIN opinion_citations.citations_unlinked cu ON cu.id = cl.citation_id
WHERE cl.status = 'skipped_not_whitelisted'
GROUP BY cu.reporter_abbr
ORDER BY citations DESC, cu.reporter_abbr
LIMIT 50;

-- 6. Links to a case decided later in the same year as the citing case: what
--    a date-granular anachronism gate would refuse. The linker's gate compares
--    years (issue #319), so an opinion of March cannot link to a case decided
--    the next year, but can to one decided that December. Self-cites are left
--    out; a link to a later year would be a failure of the gate and is counted
--    apart as a check. The overall row comes first, then by tier.
SELECT cl.match_tier,
       count(*) AS links,
       count(*) FILTER (WHERE k.decision_year = c.decision_year
                          AND k.decision_date > c.decision_date) AS same_year_later_date,
       round(100.0 * count(*) FILTER (WHERE k.decision_year = c.decision_year
                                        AND k.decision_date > c.decision_date)
             / count(*), 2) AS pct,
       count(DISTINCT cu.cap_case) FILTER (WHERE k.decision_year = c.decision_year
                                             AND k.decision_date > c.decision_date) AS citing_cases,
       percentile_cont(0.5) WITHIN GROUP (ORDER BY k.decision_date - c.decision_date)
         FILTER (WHERE k.decision_year = c.decision_year
                   AND k.decision_date > c.decision_date) AS median_days_later,
       count(*) FILTER (WHERE k.decision_year > c.decision_year) AS later_year
FROM opinion_citations.citation_links cl
JOIN opinion_citations.citations_unlinked cu ON cu.id = cl.citation_id
JOIN cap.cases c ON c.id = cu.cap_case
JOIN cap.cases k ON k.id = cl.cap_case_id
WHERE cl.status = 'linked_cap'
  AND cl.cap_case_id <> cu.cap_case
GROUP BY GROUPING SETS ((cl.match_tier), ())
ORDER BY cl.match_tier NULLS FIRST;
