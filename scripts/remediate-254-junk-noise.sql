-- One-off remediation for issue #254: delete the stale skipped_not_whitelisted
-- rows for citations whose reporter abbreviation the whitelist now carries as
-- junk, so the next linker run records them as skipped_junk.
--
-- Background: the #254 migration seeds legalhist.whitelist with the letterless
-- reporter abbreviations the generic detector produces from tables of contents
-- and page-number columns (leader dots, stray punctuation, the empty string).
-- The linker fetches work with an anti-join against citation_links
-- (StreamUnprocessedCitations), so a citation that already has a row is never
-- looked at again: these citations would sit at skipped_not_whitelisted forever
-- even though the whitelist now classifies them. Deleting the row removes them
-- from citation_links entirely, so the anti-join picks them up on the next run
-- without --reset.
--
-- Expected: 59,626 rows as of 2026-07-29, all of them skipped_not_whitelisted
-- citations whose abbreviation contains no letter. They come back as
-- skipped_junk.
--
-- Skip this script if you are about to run the linker with --reset for another
-- reason (e.g. the #250 rerun): ResetUnlinked already deletes every
-- skipped_not_whitelisted row, which subsumes this.
--
-- The delete is deliberately not restricted to the abbreviations that migration
-- added. Any skipped_not_whitelisted row whose abbreviation is junk in the
-- current whitelist is stale by definition, and reprocessing one can only
-- produce skipped_junk — the whitelist lookup is the linker's first step, so no
-- link can be gained or lost. This also sweeps up anything left behind by
-- earlier junk-whitelist migrations, of which there are none today: measured
-- before the #254 migration, exactly 0 skipped_not_whitelisted rows had an
-- abbreviation already junked, so the wider predicate costs nothing now and
-- keeps the script correct if that stops being true.
--
-- Run order (needs write access, i.e. LAW_DBSTR):
--   1. make db-up                        # apply the #254 whitelist migration first
--   2. psql "$LAW_DBSTR" -f scripts/remediate-254-junk-noise.sql
--   3. run the cite-linker
--   4. make db-maintenance               # refresh the dashboard matviews

BEGIN;

DELETE FROM moml_citations.citation_links cl
USING moml_citations.citations_unlinked cu,
      legalhist.whitelist w
WHERE cl.citation_id = cu.id
  AND w.reporter_found = cu.reporter_abbr
  AND w.junk
  AND cl.status = 'skipped_not_whitelisted';

COMMIT;
