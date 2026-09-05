-- Do interior pin-cite links draw treatise->case edges the exact links do not
-- already draw? (Issues #242 and #243; the gate kfunk074 asked for.)
--
-- The linking method aims to draw at most one edge between a citor (a treatise)
-- and a cited case. Legal writing convention is to pin-cite an interior page
-- only after giving the initial-page citation, so in a world of faithful
-- citation, perfect OCR, and a perfect detector every interior pin cite would
-- redraw an edge the exact cascade already drew, and the analysis would discard
-- it as a duplicate. The links would still be honest, but their yield would have
-- to be restated in edges rather than in citations, and the dashboard should not
-- count them as recoveries.
--
-- The world is not that one, in two directions that pull opposite ways:
--
--   * A treatise author and CAP can disagree about where a case begins. Volume
--     17 of Mass. is the worked example: CAP has Rising v. Stannard at
--     `17 Mass. 282`, but the printing notes the opinion originally began at
--     284, and 13 citations in the corpus cite it there. Those are new edges,
--     and correct ones -- the citation never had a first-page string to match.
--   * An OCR error in a page number produces a plausible interior page just as
--     readily as a real pin cite does, and that would be a new edge and a wrong
--     one.
--
-- Both show up here as "pairs_new". What distinguishes them is where they
-- concentrate: the first clusters in reporters with a reprint/pagination
-- history, the second scatters. Query 2 is that check.
--
-- Read-only; run with LAW_CLAUDE. Query 1 takes several minutes over the full
-- citation_links table.
SET statement_timeout = '20min';

-- 1. Interior links, grouped into (treatise, case) edges, split by whether an
--    exact-tier link from the same treatise already reaches that case.
WITH interior AS (
  SELECT cu.moml_treatise AS doc,
         CASE WHEN l.match_tier = 'cap_page_interior' THEN 'cap' ELSE 'er' END AS route,
         coalesce(l.cap_case_id::text, l.er_case_id) AS case_key
  FROM moml_citations.citation_links l
  JOIN moml_citations.citations_unlinked cu ON cu.id = l.citation_id
  WHERE l.match_tier IN ('cap_page_interior', 'er_page_interior')
),
-- Every edge the method already draws without page ranges. Restricted to links,
-- so a no_match tier never counts as an existing edge.
exact AS (
  SELECT DISTINCT cu.moml_treatise AS doc,
         coalesce(l.cap_case_id::text, l.er_case_id) AS case_key
  FROM moml_citations.citation_links l
  JOIN moml_citations.citations_unlinked cu ON cu.id = l.citation_id
  WHERE l.status LIKE 'linked%'
    AND l.match_tier NOT IN ('cap_page_interior', 'er_page_interior')
),
pairs AS (
  SELECT i.route, i.doc, i.case_key, count(*) AS n_cites,
         (e.doc IS NOT NULL) AS already_present
  FROM interior i
  LEFT JOIN exact e ON e.doc = i.doc AND e.case_key = i.case_key
  GROUP BY i.route, i.doc, i.case_key, e.doc
)
SELECT route,
       sum(n_cites)                                    AS interior_links,
       count(*)                                        AS doc_case_pairs,
       count(*) FILTER (WHERE already_present)         AS pairs_already_present,
       count(*) FILTER (WHERE NOT already_present)     AS pairs_new,
       sum(n_cites) FILTER (WHERE NOT already_present) AS links_in_new_pairs,
       round(100.0 * count(*) FILTER (WHERE NOT already_present) / count(*), 1) AS pct_pairs_new
FROM pairs
GROUP BY route
ORDER BY route;

-- 2. Where the new edges concentrate. A reporter whose printings renumber --
--    the Mass. nominates above all -- is where a "different first page" is
--    expected, and a high share of new edges there is the method working. A flat
--    scatter across reporters with no such history looks more like OCR noise in
--    the page number, and would argue for the frequency threshold discussed on
--    #242 (an interior page must recur N times before it is linked).
--
--    n_cites is carried through so the threshold can be sized from the same
--    pass: a real pin cite recurs, a mis-OCR'd page number usually does not.
WITH interior AS (
  SELECT cu.moml_treatise AS doc,
         cu.reporter_abbr,
         coalesce(l.cap_case_id::text, l.er_case_id) AS case_key
  FROM moml_citations.citation_links l
  JOIN moml_citations.citations_unlinked cu ON cu.id = l.citation_id
  WHERE l.match_tier IN ('cap_page_interior', 'er_page_interior')
),
exact AS (
  SELECT DISTINCT cu.moml_treatise AS doc,
         coalesce(l.cap_case_id::text, l.er_case_id) AS case_key
  FROM moml_citations.citation_links l
  JOIN moml_citations.citations_unlinked cu ON cu.id = l.citation_id
  WHERE l.status LIKE 'linked%'
    AND l.match_tier NOT IN ('cap_page_interior', 'er_page_interior')
),
pairs AS (
  SELECT i.reporter_abbr, i.doc, i.case_key, count(*) AS n_cites,
         (e.doc IS NOT NULL) AS already_present
  FROM interior i
  LEFT JOIN exact e ON e.doc = i.doc AND e.case_key = i.case_key
  GROUP BY i.reporter_abbr, i.doc, i.case_key, e.doc
)
SELECT reporter_abbr,
       count(*)                                    AS doc_case_pairs,
       count(*) FILTER (WHERE NOT already_present) AS pairs_new,
       round(100.0 * count(*) FILTER (WHERE NOT already_present) / count(*), 1) AS pct_pairs_new,
       round(avg(n_cites) FILTER (WHERE NOT already_present), 2) AS avg_cites_per_new_pair,
       count(*) FILTER (WHERE NOT already_present AND n_cites = 1) AS new_pairs_cited_once
FROM pairs
GROUP BY reporter_abbr
HAVING count(*) >= 500
ORDER BY pairs_new DESC
LIMIT 40;
