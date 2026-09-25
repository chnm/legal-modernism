-- migrate:up
SET ROLE = law_admin;

-- Admit 'us_anachronistic' and 'uk_anachronistic' in
-- chk_citation_links_match_tier (issue #319).
--
-- A treatise cannot cite a case decided after it was published, so the linker
-- now refuses such a hit: the year of the treatise volume
-- (moml.book_info.year) must not be earlier than the year of the case
-- (cap.cases.decision_year, legalhist.code_reporter.decision_year, or for the
-- English Reports murrell_year, falling back to er_year). A refused hit is
-- passed over and the cascade continues; if nothing else links, the row is
-- no_match with one of these tiers. They stand apart from the failure ladder:
-- the cascade did find a case, and refused it on its date.
--
-- Measured 2026-09-25 over the links built 2026-09-07, 414,889 of 27,439,100
-- CAP and English Reports links (1.51%) were anachronistic; PR #329 had
-- already removed 35,010 of them, the alternates that were another reporter's
-- standard.
--
-- Widening the constraint needs no backfill: no current row carries either
-- value. Rows get them when citation_links is truncated and relinked.
ALTER TABLE moml_citations.citation_links
  DROP CONSTRAINT IF EXISTS chk_citation_links_match_tier;
ALTER TABLE moml_citations.citation_links
  ADD CONSTRAINT chk_citation_links_match_tier CHECK (
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
  );

-- migrate:down
SET ROLE = law_admin;

-- A refused row records no case, so it cannot be folded back into the link it
-- would have been. Refuse to narrow the constraint while any exist: truncate
-- moml_citations.citation_links and relink with the previous linker instead.
DO $$
DECLARE n bigint;
BEGIN
  SELECT count(*) INTO n
    FROM moml_citations.citation_links
   WHERE match_tier IN ('us_anachronistic', 'uk_anachronistic');
  IF n > 0 THEN
    RAISE EXCEPTION 'cannot narrow chk_citation_links_match_tier: % rows carry an anachronistic tier; truncate citation_links and relink first', n;
  END IF;
END $$;

ALTER TABLE moml_citations.citation_links
  DROP CONSTRAINT IF EXISTS chk_citation_links_match_tier;
ALTER TABLE moml_citations.citation_links
  ADD CONSTRAINT chk_citation_links_match_tier CHECK (
    match_tier IS NULL OR match_tier IN (
      'us_reporter_absent',
      'us_diffvols_missing',
      'us_volume_absent',
      'us_volume_missing',
      'us_page_absent',
      'us_page_ambiguous',
      'us_page_gap',
      'uk_reporter_absent',
      'uk_volume_absent',
      'uk_volume_missing',
      'uk_page_absent',
      'uk_page_ambiguous',
      'uk_page_gap',
      'cap_direct',
      'cap_freelaw',
      'cap_alt_spelling',
      'cap_freelaw_alt_spelling',
      'cap_page_interior',
      'code_direct',
      'er_direct',
      'er_page_interior',
      'stub_direct'
    )
  );
