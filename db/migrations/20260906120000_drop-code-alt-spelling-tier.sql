-- migrate:up
SET ROLE = law_admin;

-- Drop 'code_alt_spelling' from chk_citation_links_match_tier (issue #292).
--
-- The linker probed legalhist.code_reporter under every alternate reporter
-- spelling after the cleaned cite missed. That probe has never produced a link:
-- of the 2,091 rows in the whole table with a code_* tier, all 2,091 are
-- code_direct and none is code_alt_spelling. It could hardly be otherwise --
-- code_reporter is 633 rows of one New York series, so an alternate spelling of
-- some other reporter has nothing there to reach -- and the probe ran once per
-- alternate per volume form for every US citation in the corpus.
--
-- The Go probe and the TierCodeAltSpelling constant go with it, so the value can
-- no longer be emitted and the constraint should no longer admit it.
--
-- No backfill is needed: there are no rows to move. The count is asserted below
-- rather than assumed, because narrowing a CHECK that current data violates
-- fails to apply, and failing with a clear message beats failing on the
-- constraint.
DO $$
DECLARE n bigint;
BEGIN
  SELECT count(*) INTO n
    FROM moml_citations.citation_links
   WHERE match_tier = 'code_alt_spelling';
  IF n > 0 THEN
    RAISE EXCEPTION 'cannot narrow chk_citation_links_match_tier: % rows still carry code_alt_spelling', n;
  END IF;
END $$;

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
      -- no_match, UK route (English Reports)
      'uk_reporter_absent',
      'uk_volume_absent',
      'uk_volume_missing',        -- #261
      'uk_page_absent',
      'uk_page_ambiguous',        -- #256
      'uk_page_gap',              -- #243
      -- linked_*: which probe produced the link
      'cap_direct',               -- cap.citations, under the normalized cite
      'cap_freelaw',              -- freelaw.cite_to_cap, under the normalized cite
      'cap_alt_spelling',         -- cap.citations, under a reporters_abbreviations alternate
      'cap_freelaw_alt_spelling', -- freelaw.cite_to_cap, under an alternate
      'cap_page_interior',        -- #242: unique containment in a case's page span
      'code_direct',              -- legalhist.code_reporter, under the cleaned cite
      'er_direct',
      'er_page_interior'          -- #243
    )
  );

-- migrate:down
SET ROLE = law_admin;

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
      'code_alt_spelling',
      'er_direct',
      'er_page_interior'
    )
  );
