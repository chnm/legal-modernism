-- migrate:up
SET ROLE = law_admin;

-- Add an "unprocessed" row to citation_links_status: citations that exist in
-- citations_unlinked but have no citation_links row yet, i.e. the backlog the
-- cite-linker still has to work through. Without it the view only describes
-- citations already processed, so a run in progress looks complete.
--
-- The count is computed by subtraction rather than by an anti-join against
-- citations_unlinked. The two are exactly equivalent here: citation_links has a
-- primary key on citation_id (so it cannot count a citation twice) and a foreign
-- key citation_links_citation_id_fkey to citations_unlinked(id) (so every row it
-- counts is a citation that exists). Subtraction is two counts instead of an
-- anti-join that, once citation_links is full, builds a multi-gigabyte hash and
-- spills to disk.
--
-- The unprocessed row is always present, showing 0 when linking is complete.
CREATE OR REPLACE VIEW moml_citations.citation_links_status AS
SELECT status, count(*) AS n
FROM moml_citations.citation_links
GROUP BY status
UNION ALL
SELECT 'unprocessed'::text AS status,
       (SELECT count(*) FROM moml_citations.citations_unlinked)
     - (SELECT count(*) FROM moml_citations.citation_links) AS n
ORDER BY n DESC;

-- migrate:down
SET ROLE = law_admin;

CREATE OR REPLACE VIEW moml_citations.citation_links_status AS
SELECT status, count(*) AS n
FROM moml_citations.citation_links
GROUP BY status
ORDER BY n DESC;
