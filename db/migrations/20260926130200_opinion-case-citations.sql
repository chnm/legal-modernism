-- migrate:up
SET ROLE = law_admin;

-- Citations from a CAP case to a case (issue #74): one row for each citing
-- case and each case its opinions cite, aggregated from
-- opinion_citations.citation_links. The twin of
-- moml_citations.edition_case_citations (issue #213), with the citing case
-- (citations_unlinked.cap_case) in the place of the edition.
--
-- The case is the unit, not the opinion, because a case's opinions cite
-- overlapping sets (a dissent answers the majority's authorities) and because
-- CAP's own graph, cap_citations.citations, is case to case: this view is
-- what the comparison in db/queries/opinion-citations-vs-cap-graph.sql
-- sets against it. opinion_count keeps the number of the case's opinions the
-- citation appears in for whoever wants the finer grain.
--
-- A case is whatever a linked citation points to: a CAP case, an English
-- Reports case, a code reporter case, or a stub case (legalhist.stub_cases),
-- in the same typed columns as citation_links, exactly one of which is set on
-- each row; source says which. Two citations that reach the same case through
-- different reporters (a parallel cite) are one row.
--
-- cite_count is the number of linked citations behind the row. pincite_only
-- is true when every one of them was linked through an interior page
-- (cap_page_interior or er_page_interior). self_cite is true when the cited
-- CAP case is the citing case, which happens when an opinion cites its own
-- case (a parallel cite, or an earlier decision in the same matter under the
-- same cite): the linker keeps such links, because its anachronism gate
-- admits the same year and it has no other rule against them, and this
-- column is where the comparison and any later measurement leave them out.
--
-- Not yet measured: the CAP corpus has not been detected or linked. Built
-- WITH NO DATA, like the other materialized views; `make db-maintenance`
-- (db/maintenance.sh) finds and refreshes it after a linker run. In a local
-- rehearsal the up's statements took 4 ms together and the down's 0.8 ms, a
-- rerun of either was a no-op, and a refresh over 403 seeded links took
-- 2 ms.
CREATE MATERIALIZED VIEW IF NOT EXISTS opinion_citations.case_case_citations AS
SELECT
  cu.cap_case AS citing_case_id,
  CASE
    WHEN cl.cap_case_id IS NOT NULL THEN 'cap'
    WHEN cl.er_case_id IS NOT NULL THEN 'er'
    WHEN cl.code_reporter_id IS NOT NULL THEN 'code'
    ELSE 'stub'
  END AS source,
  cl.cap_case_id,
  cl.er_case_id,
  cl.code_reporter_id,
  cl.stub_cite,
  count(*) AS cite_count,
  count(DISTINCT cu.cap_opinion) AS opinion_count,
  count(*) FILTER (WHERE cl.match_tier IN ('cap_page_interior', 'er_page_interior'))
    = count(*) AS pincite_only,
  coalesce(cl.cap_case_id = cu.cap_case, false) AS self_cite
FROM opinion_citations.citation_links cl
JOIN opinion_citations.citations_unlinked cu ON cu.id = cl.citation_id
WHERE cl.status LIKE 'linked%'
GROUP BY cu.cap_case, cl.cap_case_id, cl.er_case_id, cl.code_reporter_id, cl.stub_cite
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW opinion_citations.case_case_citations IS 'One row for each CAP case and each case its opinions cite, from the linked citations in opinion_citations.citation_links (issue #74); the twin of moml_citations.edition_case_citations. Exactly one of cap_case_id, er_case_id, code_reporter_id and stub_cite is set. Refreshed by make db-maintenance.';
COMMENT ON COLUMN opinion_citations.case_case_citations.citing_case_id IS 'The citing case, cap.cases.id.';
COMMENT ON COLUMN opinion_citations.case_case_citations.source IS 'Which column holds the cited case: cap, er, code or stub.';
COMMENT ON COLUMN opinion_citations.case_case_citations.cite_count IS 'Number of linked citations from the citing case''s opinions to the case.';
COMMENT ON COLUMN opinion_citations.case_case_citations.opinion_count IS 'Number of the citing case''s opinions that cite the case.';
COMMENT ON COLUMN opinion_citations.case_case_citations.pincite_only IS 'True when every citation from the citing case to the case was linked through an interior page (cap_page_interior or er_page_interior), so the link exists only because of a pin cite.';
COMMENT ON COLUMN opinion_citations.case_case_citations.self_cite IS 'True when the cited CAP case is the citing case: an opinion citing its own case. Kept by the linker, left out of the comparison with CAP''s graph.';

-- The grain: a citing case and a cited case. The case columns other than the
-- one that is set are NULL, so NULLS NOT DISTINCT is what makes the index
-- enforce it. Also serves lookups by citing case.
CREATE UNIQUE INDEX IF NOT EXISTS case_case_citations_uq
  ON opinion_citations.case_case_citations
  (citing_case_id, cap_case_id, er_case_id, code_reporter_id, stub_cite) NULLS NOT DISTINCT;

-- Lookups by cited case: the cases that cite it.
CREATE INDEX IF NOT EXISTS case_case_citations_cap_idx
  ON opinion_citations.case_case_citations (cap_case_id) WHERE cap_case_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS case_case_citations_er_idx
  ON opinion_citations.case_case_citations (er_case_id) WHERE er_case_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS case_case_citations_code_idx
  ON opinion_citations.case_case_citations (code_reporter_id) WHERE code_reporter_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS case_case_citations_stub_idx
  ON opinion_citations.case_case_citations (stub_cite) WHERE stub_cite IS NOT NULL;

-- After this migration the view is empty. Populate it with `make
-- db-maintenance`, or alone with:
--   REFRESH MATERIALIZED VIEW opinion_citations.case_case_citations;

-- migrate:down
SET ROLE = law_admin;
DROP MATERIALIZED VIEW IF EXISTS opinion_citations.case_case_citations;
