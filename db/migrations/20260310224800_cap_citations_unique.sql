-- migrate:up
-- Remove duplicate rows in cap.citations before adding a unique constraint.
-- See issue #109.
DELETE FROM cap.citations
WHERE ctid NOT IN (
    SELECT MIN(ctid)
    FROM cap.citations
    GROUP BY cite, type, "case"
);

ALTER TABLE cap.citations
    ADD CONSTRAINT cap_citations_unique UNIQUE (cite, type, "case");

-- migrate:down
ALTER TABLE cap.citations
    DROP CONSTRAINT IF EXISTS cap_citations_unique;
