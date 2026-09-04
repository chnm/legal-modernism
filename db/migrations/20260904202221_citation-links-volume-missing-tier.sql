-- migrate:up
SET ROLE = law_admin;

-- Split the volume-less citations out of us_volume_absent and uk_volume_absent
-- (issue #261).
--
-- The volume tiers were meant to say "right reporter, but no source holds this
-- volume". Measured on 2026-09-04, 76% of us_volume_absent (2.9M of 3.8M) and
-- 16% of uk_volume_absent (249K of 1.5M) were citations that carry no volume at
-- all: citations_unlinked.volume is NULL, the probe was the bare
-- "{reporter} {page}" form, and no CAP or English Reports key ever has that
-- shape for a multi-volume reporter. Nothing is known about coverage for those;
-- the volume was never detected, which points at detection (#267, #247), not at
-- dataset imports (#252). Filing them under volume_absent made CAP coverage
-- look like the bottleneck.
--
-- us_volume_missing / uk_volume_missing name that case. The linker emits them
-- when no probe it built carried a volume. Single-volume reporters never land
-- here: for them the linker also probes the volume-1 form, and the bare form is
-- a real cite, so their misses stay volume_absent or page_absent as before.
--
-- No backfill: the linker populates the column, and the rows are about to be
-- rewritten by a full re-detection and re-link for #267.
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
      'code_direct',
      'code_alt_spelling',
      'er_direct',
      'er_page_interior'          -- #243
    )
  );

-- migrate:down
SET ROLE = law_admin;

-- Rows carrying the new values would violate the narrower constraint, so fold
-- them back into the tier they were filed under before this migration.
UPDATE moml_citations.citation_links
   SET match_tier = 'us_volume_absent'
 WHERE match_tier = 'us_volume_missing';
UPDATE moml_citations.citation_links
   SET match_tier = 'uk_volume_absent'
 WHERE match_tier = 'uk_volume_missing';

ALTER TABLE moml_citations.citation_links
  DROP CONSTRAINT IF EXISTS chk_citation_links_match_tier;
ALTER TABLE moml_citations.citation_links
  ADD CONSTRAINT chk_citation_links_match_tier CHECK (
    match_tier IS NULL OR match_tier IN (
      'us_reporter_absent',
      'us_diffvols_missing',
      'us_volume_absent',
      'us_page_absent',
      'us_page_ambiguous',
      'us_page_gap',
      'uk_reporter_absent',
      'uk_volume_absent',
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
