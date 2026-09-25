-- migrate:up
SET ROLE = law_admin;

-- Citations from a MOML edition to a case (issue #213): one row for each
-- edition and each case it cites, aggregated from moml_citations.citation_links.
--
-- The edition (moml.editions.bibliographicid) is the unit because most of the
-- project's questions are about which editions cite a case; counting pages or
-- volumes would count tables of authorities and parallel cites broken across a
-- page. A citation's volume (citations_unlinked.moml_treatise, a psmid) gives
-- its edition through moml.volumes. Join moml.editions for the work
-- (work_id) and moml.treatises for the jurisdiction.
--
-- A case is whatever a linked citation points to: a CAP case, an English
-- Reports case, a code reporter case, or a stub case (legalhist.stub_cases),
-- in the same typed columns as citation_links, exactly one of which is set on
-- each row; source says which. Two citations that reach the same case through
-- different reporters (a parallel cite) are one row.
--
-- cite_count is the number of linked citations behind the row. pincite_only is
-- true when every one of them was linked through an interior page
-- (cap_page_interior or er_page_interior), so the edition cites the case only
-- because of a pin cite; issue #242 found that many such links look like OCR
-- noise rather than real pin cites.
--
-- Measured on 2026-09-25, before this migration: 19,788,634 rows from
-- 31,806,455 linked citations in 15,469 editions, of which 3,525,924 are
-- pincite_only (2,304,474 CAP and 1,221,450 English Reports). The query took
-- about four and a half minutes.
--
-- Built WITH NO DATA, like the other materialized views; `make db-maintenance`
-- (db/maintenance.sh) finds and refreshes it after a linker run.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.edition_case_citations AS
SELECT
  v.bibliographicid,
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
  count(*) FILTER (WHERE cl.match_tier IN ('cap_page_interior', 'er_page_interior'))
    = count(*) AS pincite_only
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
JOIN moml.volumes v ON v.psmid = cu.moml_treatise
WHERE cl.status LIKE 'linked%'
GROUP BY v.bibliographicid, cl.cap_case_id, cl.er_case_id, cl.code_reporter_id, cl.stub_cite
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW moml_citations.edition_case_citations IS 'One row for each MOML edition and each case it cites, from the linked citations in citation_links (issue #213). Exactly one of cap_case_id, er_case_id, code_reporter_id and stub_cite is set. Refreshed by make db-maintenance.';
COMMENT ON COLUMN moml_citations.edition_case_citations.source IS 'Which column holds the case: cap, er, code or stub.';
COMMENT ON COLUMN moml_citations.edition_case_citations.cite_count IS 'Number of linked citations from the edition to the case.';
COMMENT ON COLUMN moml_citations.edition_case_citations.pincite_only IS 'True when every citation from the edition to the case was linked through an interior page (cap_page_interior or er_page_interior), so the link exists only because of a pin cite.';

-- The grain: an edition and a case. The case columns other than the one that
-- is set are NULL, so NULLS NOT DISTINCT is what makes the index enforce it.
-- Also serves lookups by edition.
CREATE UNIQUE INDEX IF NOT EXISTS edition_case_citations_uq
  ON moml_citations.edition_case_citations
  (bibliographicid, cap_case_id, er_case_id, code_reporter_id, stub_cite) NULLS NOT DISTINCT;

-- Lookups by case: the editions that cite it.
CREATE INDEX IF NOT EXISTS edition_case_citations_cap_idx
  ON moml_citations.edition_case_citations (cap_case_id) WHERE cap_case_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS edition_case_citations_er_idx
  ON moml_citations.edition_case_citations (er_case_id) WHERE er_case_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS edition_case_citations_code_idx
  ON moml_citations.edition_case_citations (code_reporter_id) WHERE code_reporter_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS edition_case_citations_stub_idx
  ON moml_citations.edition_case_citations (stub_cite) WHERE stub_cite IS NOT NULL;

-- After this migration the view is empty. Populate it with `make
-- db-maintenance`, or alone with:
--   REFRESH MATERIALIZED VIEW moml_citations.edition_case_citations;

-- migrate:down
SET ROLE = law_admin;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.edition_case_citations;
