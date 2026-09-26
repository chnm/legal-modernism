-- migrate:up
SET ROLE = law_admin;

-- opinion_citations: the citations our detector finds in the text of the CAP
-- opinions, and what they link to (issue #74). The twin of moml_citations,
-- which holds the same for the pages of the MOML treatises.
--
-- The two schemas hold the same two tables with the same columns, save the
-- citing key: a MOML citation is found on a page of a volume (moml_treatise,
-- moml_page), a CAP citation in an opinion of a case (cap_opinion, cap_case).
-- The same detector code writes both (cite-detector-moml, cite-detector-cap)
-- and the same linking cascade (go/linker) reads both, so the columns the
-- cascade reads and writes must agree in name and type; and the questions the
-- project has for the two corpora together (which cases the treatises cite
-- that the courts do not, and the reverse) join them on the same case ids.
--
-- Not to be confused with cap_citations, which is CAP's own citation graph:
-- cap_citations.citations holds 45.5M cites_from/cites_to edges, 6,262,617
-- of them from the 848,695 citing cases decided by 1920. That graph is the
-- comparison target for what this schema holds
-- (db/queries/opinion-citations-vs-cap-graph.sql), not a source for it.
--
-- Sizing, measured 2026-09-26: 1,408,784 of the 6,920,596 CAP cases were
-- decided by 1920, the detector's default --max-year; their opinions are
-- about 1.59M rows and 9.6 GB of text. A regex over a sample found about
-- four citation-shaped strings per opinion (median 1; 48% of opinions have
-- none), so expect on the order of 6.5M rows in citations_unlinked, against
-- 55.9M in the MOML ledger; 96.8% of the sampled strings use a spelling
-- already in legalhist.whitelist. In a local rehearsal the up's statements
-- took 6 ms together and the down's 1.4 ms, and a rerun of either was a
-- no-op.
CREATE SCHEMA IF NOT EXISTS opinion_citations;
GRANT USAGE ON SCHEMA opinion_citations TO law_service, law_dev;
COMMENT ON SCHEMA opinion_citations IS 'Citations detected by cite-detector-cap in the text of cap.opinions and linked by cite-linker-cap (issue #74): the twin of moml_citations for the CAP corpus. Not CAP''s own citation graph, which is cap_citations.';

-- One row per opinion and distinct cite the detector found in it, the twin
-- of moml_citations.citations_unlinked. cap_case is determined by cap_opinion
-- and stored again on purpose: the linker's anachronism gate (the citing
-- case's decision_year), the case_case_citations view and every per-case
-- question key on it, and none of them should have to join a 38 GB table to
-- get it.
CREATE TABLE IF NOT EXISTS opinion_citations.citations_unlinked (
    id            uuid PRIMARY KEY,
    cap_case      bigint NOT NULL REFERENCES cap.cases(id),
    cap_opinion   bigint NOT NULL REFERENCES cap.opinions(id),
    raw           text NOT NULL,
    volume        integer,
    reporter_abbr text NOT NULL,
    page          integer NOT NULL,
    year          integer,
    created_at    timestamp without time zone NOT NULL
);

-- The per-opinion key, mirroring the MOML per-page key: a cite repeated in
-- one opinion is one row. volume is NULL for a single-volume reporter and
-- year for a volume-cited one, and NULL is distinct from NULL in a unique
-- index, so both are coalesced.
CREATE UNIQUE INDEX IF NOT EXISTS citations_unlinked_uq
  ON opinion_citations.citations_unlinked
  (cap_opinion, COALESCE(volume, -1), reporter_abbr, page, COALESCE(year, -1));
CREATE INDEX IF NOT EXISTS citations_unlinked_reporter_abbr_idx
  ON opinion_citations.citations_unlinked (reporter_abbr);
CREATE INDEX IF NOT EXISTS citations_unlinked_cap_case_idx
  ON opinion_citations.citations_unlinked (cap_case);
-- The MOML twin also carries a second unique index on id
-- (moml_citations_id_key), redundant with its primary key. Deliberately not
-- copied.

COMMENT ON TABLE opinion_citations.citations_unlinked IS 'Our detections from the text of cap.opinions, written by cite-detector-cap for the cases decided by the run''s --max-year: one row per opinion and distinct cite (volume, reporter spelling, page, year). The twin of moml_citations.citations_unlinked. Not CAP''s own citation graph, which is cap_citations.citations. The foreign keys onto cap.cases and cap.opinions mean a TRUNCATE of cap.opinions needs CASCADE and empties the detections.';

-- What each citation linked to, the twin of moml_citations.citation_links:
-- the same ten columns, types and foreign keys, so the linker's save is the
-- same statement with the table name switched. stub_cite has no foreign key,
-- for the reason 20260906130000_create-stub-cases.sql gives: the registry is
-- rebuilt from the MOML linker's misses and prunes rows that stale links
-- still point at.
CREATE TABLE IF NOT EXISTS opinion_citations.citation_links (
    citation_id      uuid PRIMARY KEY REFERENCES opinion_citations.citations_unlinked(id),
    status           text NOT NULL,
    match_tier       text,
    cap_case_id      bigint REFERENCES cap.cases(id),
    code_reporter_id bigint REFERENCES legalhist.code_reporter(id),
    er_case_id       text REFERENCES english_reports.cases(id),
    stub_cite        text,
    cite_cleaned     text,
    cite_normalized  text,
    cite_linked      text,
    created_at       timestamp with time zone DEFAULT now() NOT NULL,
    -- Must stay in step with the constraint of the same name on
    -- moml_citations.citation_links and with the Tier constants in
    -- go/citations/linker.go: one cascade links both corpora, so a tier it
    -- emits must be admitted by both CHECKs, and a migration that widens one
    -- widens both. go/citations/linker_tier_constraint_test.go holds the
    -- pair to it. The values and their comments are a verbatim copy of
    -- 20260925120000_anachronistic-tiers.sql.
    CONSTRAINT chk_citation_links_match_tier CHECK (
      match_tier IS NULL OR match_tier IN (
        -- no_match, US route (CAP -> FreeLaw -> alternate spellings -> code reporter)
        'us_reporter_absent',       -- no probed reporter spelling appears in any US source
        'us_diffvols_missing',      -- renumbers in CAP, but no reporters_diffvols row for this volume
        'us_volume_absent',         -- reporter present, this volume never appears
        'us_volume_missing',        -- #261: reporter present, but the citation carries no volume
        'us_page_absent',           -- reporter and volume present, page is not a first-page cite
        'us_page_ambiguous',        -- #242: page falls inside two or more case spans
        'us_page_gap',              -- #242: page falls inside no case span in a covered volume
        'us_anachronistic',         -- #319: every case found was decided after the treatise
        -- no_match, UK route (English Reports)
        'uk_reporter_absent',
        'uk_volume_absent',
        'uk_volume_missing',        -- #261
        'uk_page_absent',
        'uk_page_ambiguous',        -- #256
        'uk_page_gap',              -- #243
        'uk_anachronistic',         -- #319
        -- linked_*: which probe produced the link
        'cap_direct',               -- cap.citations, under the normalized cite
        'cap_freelaw',              -- freelaw.cite_to_cap, under the normalized cite
        'cap_alt_spelling',         -- cap.citations, under a reporters_abbreviations alternate
        'cap_freelaw_alt_spelling', -- freelaw.cite_to_cap, under an alternate
        'cap_page_interior',        -- #242: unique containment in a case's page span
        'code_direct',              -- legalhist.code_reporter, under the cleaned cite
        'er_direct',
        'er_page_interior',         -- #243
        'stub_direct'               -- #248: legalhist.stub_cases, under the cleaned cite
      )
    )
);

-- The same six indexes as the MOML twin.
CREATE INDEX IF NOT EXISTS idx_citation_links_status
  ON opinion_citations.citation_links (status);
CREATE INDEX IF NOT EXISTS idx_citation_links_cap
  ON opinion_citations.citation_links (cap_case_id) WHERE cap_case_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_citation_links_code
  ON opinion_citations.citation_links (code_reporter_id) WHERE code_reporter_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_citation_links_er
  ON opinion_citations.citation_links (er_case_id) WHERE er_case_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_citation_links_stub
  ON opinion_citations.citation_links (stub_cite) WHERE stub_cite IS NOT NULL;
CREATE INDEX IF NOT EXISTS citation_links_cite_normalized_idx
  ON opinion_citations.citation_links (cite_normalized);

COMMENT ON TABLE opinion_citations.citation_links IS 'What each citation in citations_unlinked linked to, written by cite-linker-cap: the twin of moml_citations.citation_links, with the same columns, statuses and tiers. A self-citation, cap_case_id = citations_unlinked.cap_case (an opinion citing its own case, as by a parallel cite), is kept and not refused: the anachronism gate admits the same year. Links to legalhist.stub_cases (linked_stub) point at a registry this corpus only reads and never feeds, so a MOML stubs rebuild can leave them stale until these links are rebuilt.';

GRANT SELECT ON opinion_citations.citations_unlinked, opinion_citations.citation_links
  TO law_service;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE
  ON opinion_citations.citations_unlinked, opinion_citations.citation_links
  TO law_dev;

-- migrate:down
SET ROLE = law_admin;

DROP TABLE IF EXISTS opinion_citations.citation_links;
DROP TABLE IF EXISTS opinion_citations.citations_unlinked;
DROP SCHEMA IF EXISTS opinion_citations;
