-- migrate:up
ALTER TABLE moml_citations.citation_links
    RENAME COLUMN normalized_cite TO cite_cleaned;
ALTER TABLE moml_citations.citation_links
    ADD COLUMN IF NOT EXISTS cite_normalized text;
ALTER TABLE moml_citations.citation_links
    ADD COLUMN IF NOT EXISTS cite_linked text;

-- migrate:down
ALTER TABLE moml_citations.citation_links
    DROP COLUMN IF EXISTS cite_linked;
ALTER TABLE moml_citations.citation_links
    DROP COLUMN IF EXISTS cite_normalized;
ALTER TABLE moml_citations.citation_links
    RENAME COLUMN cite_cleaned TO normalized_cite;
