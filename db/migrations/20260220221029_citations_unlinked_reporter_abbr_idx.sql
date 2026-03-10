-- migrate:up
CREATE INDEX IF NOT EXISTS "citations_unlinked_reporter_abbr_idx" ON "moml_citations"."citations_unlinked" ("reporter_abbr");

-- migrate:down
DROP INDEX IF EXISTS moml_citations."citations_unlinked_reporter_abbr_idx";
