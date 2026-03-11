-- migrate:up
ALTER TABLE cap.citations
    ADD CONSTRAINT cap_citations_unique UNIQUE (cite, type, "case");

-- migrate:down
ALTER TABLE cap.citations
    DROP CONSTRAINT IF EXISTS cap_citations_unique;
