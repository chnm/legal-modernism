-- migrate:up
SET ROLE = law_admin;

-- Record how far the cite-linker got with each citation, not just whether it
-- succeeded (issue #255).
--
-- citation_links.status says "no_match" for 20.1M citations without saying which
-- tier of the cascade was exhausted, so the diagnosis behind #242-#248 had to be
-- re-derived by query -- roughly 50 seconds per reporter against
-- citations_unmatched_top, which itself only covers combos occurring 5 or more
-- times. The linker already knows the answer: by the time it writes no_match it
-- has probed the CAP, FreeLaw, code-reporter, and English Reports maps, every
-- alternate spelling, and both volume forms, all in memory.
--
-- This is a separate column rather than sub-statuses of no_match because
-- citations.ResetUnlinked deletes exactly three literal statuses (no_match,
-- skipped_not_whitelisted, skipped_junk), so a new status would silently escape
-- --reset, and because every SQL consumer keys on status: the linked% patterns in
-- linking_dashboard_reporters and normalized_citation_counts, the status =
-- 'linked_cap' filters in case_citation_counts, and the subtraction in
-- citation_links_status. A new column is invisible to all of them.
--
-- A failure tier is the *closest approach across every target probed*, ordered
-- reporter_absent < volume_absent < page_absent, so a citation whose reporter is
-- missing from CAP but present in the code reporter reports the code-reporter
-- outcome. A single value is honest in a way a separate target column would not
-- be: a US no_match exhausted CAP and FreeLaw and the code reporter, so naming
-- one target would be a half-truth. That is why failure tiers carry a route
-- prefix (us_/uk_) while success tiers, where exactly one target matched, name
-- the target (cap_/code_/er_). Either way the prefix carries the route, which is
-- why this needs no uk column and no join to legalhist.reporters.
--
-- The column is left NULL for skipped_not_whitelisted and skipped_junk, where
-- status is already the whole explanation and no cascade ran.
ALTER TABLE moml_citations.citation_links
  ADD COLUMN IF NOT EXISTS match_tier text;

-- The allowed set is constrained here because status's is not: it lives only in
-- the Go constants in go/citations/linker.go, which is what left its vocabulary
-- ambiguous. This list must stay in step with the Tier constants in that file.
-- Values are not required to be present (no NOT NULL, no status-conditional
-- check) so that this applies to the rows already in the table; they populate as
-- the linker rewrites them.
--
-- us_page_ambiguous, us_page_gap, uk_page_ambiguous, uk_page_gap,
-- cap_page_interior, and er_page_interior are the outcomes of the page-range
-- matching in #242 and #243, listed now so those issues need no constraint
-- change. Until they land, a page-level miss is us_page_absent / uk_page_absent.
ALTER TABLE moml_citations.citation_links
  DROP CONSTRAINT IF EXISTS chk_citation_links_match_tier;
ALTER TABLE moml_citations.citation_links
  ADD CONSTRAINT chk_citation_links_match_tier CHECK (
    match_tier IS NULL OR match_tier IN (
      -- no_match, US route (CAP -> FreeLaw -> alternate spellings -> code reporter)
      'us_reporter_absent',       -- no probed reporter spelling appears in any US source
      'us_diffvols_missing',      -- renumbers in CAP, but no reporters_diffvols row for this volume
      'us_volume_absent',         -- reporter present, this volume never appears
      'us_page_absent',           -- reporter and volume present, page is not a first-page cite
      'us_page_ambiguous',        -- #242: page falls inside two or more case spans
      'us_page_gap',              -- #242: page falls inside no case span in a covered volume
      -- no_match, UK route (English Reports)
      'uk_reporter_absent',
      'uk_volume_absent',
      'uk_page_absent',
      'uk_page_ambiguous',        -- #243
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

-- Aggregated tiers per reporter. A materialized view because the reporter
-- breakdown needs the citations_unlinked/whitelist join that makes the
-- re-derivation expensive in the first place; db/maintenance.sh discovers
-- materialized views from the system catalogs, so it picks this up without
-- editing. Junk and non-whitelisted citations are excluded, matching
-- linking_dashboard_reporters -- the tier only describes citations the linker
-- actually tried to match -- so the counts here total less than citation_links.
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.linking_dashboard_tiers AS
SELECT wl.reporter_standard,
       cl.status,
       cl.match_tier,
       count(*) AS n
FROM moml_citations.citations_unlinked cu
  JOIN legalhist.whitelist wl ON cu.reporter_abbr = wl.reporter_found
  LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
WHERE wl.reporter_standard IS NOT NULL
  AND wl.junk = false
GROUP BY wl.reporter_standard, cl.status, cl.match_tier
WITH NO DATA;

-- NULLS NOT DISTINCT because both status and match_tier are nullable here:
-- status is NULL for unprocessed citations (the LEFT JOIN) and match_tier is NULL
-- for every row until the linker populates it. Without it those rows would not
-- collide and the index would not enforce uniqueness -- which REFRESH
-- MATERIALIZED VIEW CONCURRENTLY requires.
CREATE UNIQUE INDEX IF NOT EXISTS linking_dashboard_tiers_uq
  ON moml_citations.linking_dashboard_tiers (reporter_standard, status, match_tier)
  NULLS NOT DISTINCT;

-- Corpus-wide rollup. Reads the materialized view rather than citation_links, so
-- it answers instantly and needs no refresh of its own; it is as stale as the
-- last db-maintenance run. Query linking_dashboard_tiers directly for the
-- per-reporter breakdown (which reporters dominate a tier).
--
-- A NULL status is a citation with no citation_links row at all, labelled
-- 'unprocessed' as in citation_links_status. Its tier is NULL for the same
-- reason: no cascade has run for it yet.
CREATE OR REPLACE VIEW moml_citations.linking_tier_summary AS
SELECT COALESCE(status, 'unprocessed') AS status,
       match_tier,
       sum(n) AS n,
       round(100.0 * sum(n) / sum(sum(n)) OVER (PARTITION BY status), 2) AS pct_of_status
FROM moml_citations.linking_dashboard_tiers
GROUP BY status, match_tier
ORDER BY sum(n) DESC;

-- migrate:down
SET ROLE = law_admin;

DROP VIEW IF EXISTS moml_citations.linking_tier_summary;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.linking_dashboard_tiers;
ALTER TABLE moml_citations.citation_links
  DROP CONSTRAINT IF EXISTS chk_citation_links_match_tier;
ALTER TABLE moml_citations.citation_links
  DROP COLUMN IF EXISTS match_tier;
