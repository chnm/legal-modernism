-- migrate:up
CREATE OR REPLACE VIEW moml_citations.citation_links_status AS
SELECT status, count(*) AS n
FROM moml_citations.citation_links
GROUP BY status
ORDER BY n DESC;

CREATE OR REPLACE VIEW moml_citations.citation_links_detail AS
SELECT
    cl.citation_id,
    cl.status,
    cu.raw AS cite_raw,
    cl.cite_cleaned,
    cl.cite_normalized,
    cl.cite_linked,
    COALESCE(cc.name, cr.name, er.er_name) AS case_name,
    cl.cap_case_id,
    cl.code_reporter_id,
    cl.er_case_id
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
LEFT JOIN cap.cases cc ON cc.id = cl.cap_case_id
LEFT JOIN legalhist.code_reporter cr ON cr.id = cl.code_reporter_id
LEFT JOIN english_reports.cases er ON er.id = cl.er_case_id;

-- migrate:down
DROP VIEW IF EXISTS moml_citations.citation_links_detail;
DROP VIEW IF EXISTS moml_citations.citation_links_status;
