-- migrate:up
CREATE OR REPLACE VIEW moml_citations.citation_links_status AS
SELECT status, count(*) AS n
FROM moml_citations.citation_links
GROUP BY status
ORDER BY n DESC;

-- migrate:down
DROP VIEW IF EXISTS moml_citations.citation_links_status;
