-- migrate:up
CREATE TABLE IF NOT EXISTS moml_citations.citation_links (
    citation_id       uuid NOT NULL REFERENCES moml_citations.citations_unlinked(id),
    status            text NOT NULL,
    cap_case_id       bigint REFERENCES cap.cases(id),
    code_reporter_id  bigint REFERENCES legalhist.code_reporter(id),
    er_case_id        text REFERENCES english_reports.cases(id),
    cite_cleaned      text,
    cite_normalized   text,
    cite_linked       text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (citation_id)
);

CREATE INDEX IF NOT EXISTS idx_citation_links_status
    ON moml_citations.citation_links (status);
CREATE INDEX IF NOT EXISTS idx_citation_links_cap
    ON moml_citations.citation_links (cap_case_id) WHERE cap_case_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_citation_links_code
    ON moml_citations.citation_links (code_reporter_id) WHERE code_reporter_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_citation_links_er
    ON moml_citations.citation_links (er_case_id) WHERE er_case_id IS NOT NULL;

-- migrate:down
DROP TABLE IF EXISTS moml_citations.citation_links;
