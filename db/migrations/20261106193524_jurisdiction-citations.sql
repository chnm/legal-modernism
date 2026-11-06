-- migrate:up
SET ROLE = law_admin;

-- The CAP citation graph rolled up to jurisdictions (issue #216): for each
-- citing jurisdiction, year and cited jurisdiction, the linked citations from
-- the opinions of cases decided in that jurisdiction and year to cases of the
-- cited jurisdiction, and the citing cases that make them. Chambers' draft
-- public visualization /viz/jurisdictions reads it.
--
-- The citations are our own links (opinion_citations.citation_links, through
-- case_case_citations), not CAP's citation graph. The citing jurisdiction is
-- the citing case's cap.jurisdictions slug; the year is the citing case's
-- decision year, so a bar in the "cited by" direction is still dated by the
-- citing case. A cited CAP case takes its own jurisdiction, and a cited English
-- Reports case counts as 'uk', the slug CAP itself uses for its two United
-- Kingdom cases. Stub and Code Reporter cases are left out: no jurisdiction is
-- recorded for them. An opinion's citation of its own case (self_cite) is left
-- out too; a citation to another case of the same jurisdiction stays, as a row
-- whose two jurisdictions are equal.
--
-- citing_cases is a count of distinct cases within the row, and since a case
-- has one year and one jurisdiction it sums exactly over years and over cited
-- jurisdictions. Built WITH NO DATA; `make db-maintenance` (db/maintenance.sh)
-- refreshes it after case_case_citations.

CREATE MATERIALIZED VIEW IF NOT EXISTS opinion_citations.jurisdiction_citations AS
 SELECT c.decision_year AS citing_year,
        cj.slug AS citing_jurisdiction,
        CASE WHEN ccc.source = 'cap' THEN kj.slug ELSE 'uk' END AS cited_jurisdiction,
        sum(ccc.cite_count)::bigint AS citations,
        count(DISTINCT ccc.citing_case_id) AS citing_cases
   FROM opinion_citations.case_case_citations ccc
   JOIN cap.cases c ON c.id = ccc.citing_case_id
   JOIN cap.jurisdictions cj ON cj.id = c.jurisdiction
   LEFT JOIN cap.cases k ON k.id = ccc.cap_case_id
   LEFT JOIN cap.jurisdictions kj ON kj.id = k.jurisdiction
  WHERE ccc.source IN ('cap', 'er')
    AND NOT ccc.self_cite
    AND c.decision_year IS NOT NULL
  GROUP BY 1, 2, 3
  WITH NO DATA;

CREATE UNIQUE INDEX IF NOT EXISTS jurisdiction_citations_uq
  ON opinion_citations.jurisdiction_citations USING btree (citing_jurisdiction, cited_jurisdiction, citing_year);

COMMENT ON MATERIALIZED VIEW opinion_citations.jurisdiction_citations IS 'Our CAP citation links rolled up by the citing case''s jurisdiction and decision year and the cited case''s jurisdiction (cap.jurisdictions slugs; a cited English Reports case is uk), with the citations and the distinct citing cases (issue #216). Self-cites, stubs and Code Reporter cases are left out. Read by chambers'' /viz/jurisdictions. Refreshed by make db-maintenance.';

-- After this migration the view is empty. Populate it with `make
-- db-maintenance`, or alone with
-- REFRESH MATERIALIZED VIEW opinion_citations.jurisdiction_citations;

-- migrate:down
SET ROLE = law_admin;
DROP MATERIALIZED VIEW IF EXISTS opinion_citations.jurisdiction_citations;
