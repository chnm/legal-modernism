-- migrate:up
-- Remove duplicate rows in cap.citations before adding a unique constraint.
-- See issue 109.
DELETE FROM cap.citations a
USING cap.citations b
WHERE a.ctid > b.ctid
  AND a.cite = b.cite
  AND a.type = b.type
  AND a."case" = b."case";

ALTER TABLE cap.citations
    ADD CONSTRAINT cap_citations_unique UNIQUE (cite, type, "case");

-- migrate:down
ALTER TABLE cap.citations
    DROP CONSTRAINT IF EXISTS cap_citations_unique;
