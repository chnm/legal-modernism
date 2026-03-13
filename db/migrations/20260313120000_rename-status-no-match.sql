-- migrate:up
UPDATE moml_citations.citation_links SET status = 'no_match' WHERE status = 'skipped_no_match';

-- migrate:down
UPDATE moml_citations.citation_links SET status = 'skipped_no_match' WHERE status = 'no_match';
