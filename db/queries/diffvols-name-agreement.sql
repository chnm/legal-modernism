-- Which volume offset does a nominative reporter have in CAP's numbering?
-- (issues #399, #400)
--
-- For every citation of a reporter in the CAP ledger with a directly detected
-- case name, look up the CAP case whose first page is the cited page at two
-- candidate offsets, and count how often the first word of the detected name
-- appears in that case's name. The right offset agrees for most citations in
-- every stretch of volumes; the wrong one almost never does. It reads the
-- detections, not the links, so it can be run before a migration and needs
-- no relink.
--
-- psql variables:
--   std     the whitelist's reporter_standard            ('Hun')
--   cap     CAP's spelling of the reporter in cap.citations ('N.Y. Sup. Ct.')
--   rid     CAP's reporter id (cap.reporters.id)            (110)
--   o1, o2  the offsets to compare: CAP volume = cited volume + offset
--   bucket  volumes per row
--
-- psql "$LAW_CLAUDE" -v std=Hun -v 'cap=N.Y. Sup. Ct.' -v rid=110 \
--   -v o1=7 -v o2=0 -v bucket=10 -f db/queries/diffvols-name-agreement.sql
-- psql "$LAW_CLAUDE" -v std=Vroom -v cap=N.J.L. -v rid=551 \
--   -v o1=29 -v o2=35 -v bucket=6 -f db/queries/diffvols-name-agreement.sql
--
-- Hun takes about ten minutes; a smaller reporter, one or two.

WITH cites AS (
    SELECT cu.volume AS v,
           cu.page AS p,
           lower(split_part(cu.case_name, ' ', 1)) AS word
      FROM opinion_citations.citations_unlinked cu
      JOIN legalhist.whitelist w ON w.reporter_found = cu.reporter_abbr
     WHERE w.reporter_standard = :'std'
       AND cu.case_name_source = 'direct'
       AND cu.volume IS NOT NULL
       AND length(split_part(cu.case_name, ' ', 1)) > 3
), cap_cases AS (
    SELECT split_part(ci.cite, ' ', 1)::int AS vol,
           k.first_page AS fp,
           lower(k.name_abbreviation) AS name
      FROM cap.citations ci
      JOIN cap.cases k ON k.id = ci."case"
     WHERE k.reporter = :rid
       AND ci.cite = split_part(ci.cite, ' ', 1) || ' ' || :'cap' || ' ' || k.first_page
)
SELECT (v - 1) / :bucket * :bucket + 1 AS volumes_from,
       count(*) AS citations,
       count(*) FILTER (WHERE EXISTS (
           SELECT 1 FROM cap_cases k
            WHERE k.vol = c.v + :o1 AND k.fp = c.p AND position(c.word IN k.name) > 0
       )) AS agree_o1,
       count(*) FILTER (WHERE EXISTS (
           SELECT 1 FROM cap_cases k
            WHERE k.vol = c.v + :o2 AND k.fp = c.p AND position(c.word IN k.name) > 0
       )) AS agree_o2
  FROM cites c
 GROUP BY 1
 ORDER BY 1;
