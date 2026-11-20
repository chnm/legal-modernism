-- migrate:up
SET ROLE = law_admin;

-- Citations that Claude found and linked by reading MOML pages one at a time
-- (issue #439), starting with the editions of Stephen on Pleading. A reader
-- sees only a page's OCR text and looks the cases up itself, so these links
-- are independent of the regex detector and linker in citations_unlinked and
-- citation_links, and can be compared with them. They are written by
-- llm-reader as JSON files under llm-reader/results and loaded with
-- `llm-reader load`.

-- One row per page read, so that a page with no citations is recorded as read.
CREATE TABLE IF NOT EXISTS moml_citations.llm_pages (
    psmid          text        NOT NULL,
    pageid         text        NOT NULL,
    model          text        NOT NULL,
    read_at        timestamptz NOT NULL,
    citation_count integer     NOT NULL CHECK (citation_count >= 0),
    note           text,
    loaded_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (psmid, pageid),
    CONSTRAINT llm_pages_page_fkey FOREIGN KEY (pageid, psmid)
        REFERENCES moml.page (pageid, psmid)
);

-- One row per citation of a case on a page, in the order it is printed.
CREATE TABLE IF NOT EXISTS moml_citations.llm_links (
    psmid            text    NOT NULL,
    pageid           text    NOT NULL,
    seq              integer NOT NULL CHECK (seq > 0),
    raw              text    NOT NULL,
    case_name        text,
    volume           integer,
    reporter_abbr    text,
    page             integer,
    year             integer,
    status           text    NOT NULL,
    cap_case_id      bigint  REFERENCES cap.cases (id),
    er_case_id       text    REFERENCES english_reports.cases (id),
    code_reporter_id bigint  REFERENCES legalhist.code_reporter (id),
    stub_cite        text    REFERENCES legalhist.stub_cases (cite) ON UPDATE CASCADE,
    pincite          boolean NOT NULL DEFAULT false,
    confidence       text    NOT NULL,
    note             text,
    PRIMARY KEY (psmid, pageid, seq),
    CONSTRAINT llm_links_page_fkey FOREIGN KEY (psmid, pageid)
        REFERENCES moml_citations.llm_pages (psmid, pageid) ON DELETE CASCADE,
    CONSTRAINT chk_llm_links_status CHECK (status IN (
        'linked_cap', 'linked_english_reports', 'linked_code_reporter',
        'linked_stub', 'no_match')),
    CONSTRAINT chk_llm_links_confidence CHECK (confidence IN ('high', 'medium', 'low')),
    -- A linked row names exactly its own target; a no_match row names none.
    CONSTRAINT chk_llm_links_target CHECK (
        (cap_case_id IS NOT NULL)::int + (er_case_id IS NOT NULL)::int
      + (code_reporter_id IS NOT NULL)::int + (stub_cite IS NOT NULL)::int
      = CASE WHEN status = 'no_match' THEN 0 ELSE 1 END
      AND (status <> 'linked_cap'             OR cap_case_id IS NOT NULL)
      AND (status <> 'linked_english_reports' OR er_case_id IS NOT NULL)
      AND (status <> 'linked_code_reporter'   OR code_reporter_id IS NOT NULL)
      AND (status <> 'linked_stub'            OR stub_cite IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS llm_links_cap_idx ON moml_citations.llm_links (cap_case_id) WHERE cap_case_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS llm_links_er_idx ON moml_citations.llm_links (er_case_id) WHERE er_case_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS llm_links_code_idx ON moml_citations.llm_links (code_reporter_id) WHERE code_reporter_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS llm_links_stub_idx ON moml_citations.llm_links (stub_cite) WHERE stub_cite IS NOT NULL;

COMMENT ON TABLE moml_citations.llm_pages IS
  'MOML pages Claude has read for citations (issue #439), one row per page, including pages with none. Loaded from llm-reader/results by llm-reader load.';
COMMENT ON TABLE moml_citations.llm_links IS
  'Citations of cases Claude found and linked by reading a MOML page (issue #439), independent of citations_unlinked and citation_links.';
COMMENT ON COLUMN moml_citations.llm_links.raw IS
  'The citation as printed on the page (OCR errors and all).';
COMMENT ON COLUMN moml_citations.llm_links.case_name IS
  'The case name as printed, or as the reader inferred it from context; NULL when none is given.';
COMMENT ON COLUMN moml_citations.llm_links.pincite IS
  'True when the cited page is inside the case rather than its first page.';
COMMENT ON COLUMN moml_citations.llm_links.note IS
  'The reader''s reasoning when it is not obvious: an OCR correction, a disambiguation, why no case was found.';

GRANT SELECT ON moml_citations.llm_pages, moml_citations.llm_links TO law_service;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE
   ON moml_citations.llm_pages, moml_citations.llm_links TO law_dev;

-- migrate:down
SET ROLE = law_admin;

DROP TABLE IF EXISTS moml_citations.llm_links;
DROP TABLE IF EXISTS moml_citations.llm_pages;
