-- migrate:up
SET ROLE = law_admin;

CREATE INDEX "citations_unlinked_reporter_abbr_idx"
    ON "moml_citations"."citations_unlinked"("reporter_abbr");

-- migrate:down
SET ROLE = law_admin;

DROP INDEX IF EXISTS moml_citations."citations_unlinked_reporter_abbr_idx";
