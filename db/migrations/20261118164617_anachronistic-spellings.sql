-- migrate:up
SET ROLE = law_admin;

-- Spellings that the anachronism check shows point at a reporter from the
-- wrong era (issue #331). Only the fixes Kellen Funk approved in
-- https://github.com/chnm/legal-modernism/issues/331 are made here; the other
-- spellings in that issue (Harr. (Del.), Smith, Il., Tucker, S. D., Bell,
-- Corn., C. D., and the regional reporters through FreeLaw) are unchanged.
--
-- Counts are of citations in both ledgers, measured 2026-11-18.
--
-- 1. N. J. The whitelist read it as the New Jersey Reports, which begin in
--    1948, so every link it made was refused as anachronistic. A treatise or
--    opinion of our period means the Law Reports or the Equity Reports. It is
--    now read as N.J.L., falling back to N.J. Eq., and is left unlinked if
--    neither has the case. The two series number their volumes side by side
--    (1-137 and 1-142), so an Equity cite finds the Law Reports' volume and
--    misses only at the page, a miss that by #358's rule never falls back.
--    whitelist.fallback_any_miss lets a spelling fall back after any miss; it
--    is set on these spellings only. Of the 13,454 "N. J." citations, about
--    7,100 reach an N.J.L. case and 1,640 more reach only an N.J. Eq. case.
--    Every spelling that was read as N.J. is moved: none can mean the 1948
--    series in documents decided or published by 1926.
--
-- 2. Dick. The whitelist read it as Dickinson's Chancery Reports (vols. 46-66
--    N.J. Eq., 1889-1904). In the treatises it is mostly Dickens's Chancery
--    Reports, which the English Reports hold as "Dick". It is now read as
--    Dickens, falling back to Dickinson after the misses #358 falls back on:
--    "3 Dick." is past Dickens's two volumes (uk_volume_absent), and a New
--    Jersey opinion's "Dick." is read as Dickinson first (#401). Dickens's two
--    volumes are paged straight through, and the English Reports key them by
--    page alone, "Dick 500"; reporters.paged_volumes says so, and the linker
--    then probes "2 Dick. 500" as "Dick 500" (8,600 citations read "2 Dick.").
--    The spelling "Dick" moves with it.
--
-- 3. N. C. C. A. and Tax Cas. were read as the North Carolina Court of Appeals
--    (1968-) and the U.S. Tax Court (1942-), every link anachronistic. They are
--    Negligence and Compensation Cases Annotated and the English Reports of
--    Tax Cases, which no source holds. New reporter rows take the spellings, so
--    their misses are reporter_absent and make db-stubs mints stubs for them
--    (at the default threshold, about 440 and 270 cite strings). The plain
--    variants of Tax Cas. move with it; "Tax C.", "Tax.", "T.C." and
--    "N. C. C." are ambiguous and stay where they are.
--
-- The data steps: a relink of both corpora, make db-stubs, and a relink of
-- both again, so the citations link to the new stubs.

ALTER TABLE legalhist.whitelist
  ADD COLUMN IF NOT EXISTS fallback_any_miss boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN legalhist.whitelist.fallback_any_miss IS 'Try fallback_standard after any miss of the spelling''s own reading, a page miss included, not only after a volume or anachronism miss (issue #331).';

ALTER TABLE legalhist.whitelist
  DROP CONSTRAINT IF EXISTS chk_whitelist_fallback_any_miss;
ALTER TABLE legalhist.whitelist
  ADD CONSTRAINT chk_whitelist_fallback_any_miss CHECK (
    NOT fallback_any_miss OR fallback_standard IS NOT NULL
  );

ALTER TABLE legalhist.reporters
  ADD COLUMN IF NOT EXISTS paged_volumes integer;

COMMENT ON COLUMN legalhist.reporters.paged_volumes IS 'Volumes 1 through paged_volumes are paginated as one run, so a cite to any of them is keyed by page alone ("2 Dick. 500" is "Dick 500"); NULL when the volumes are paged separately (issue #331).';

UPDATE legalhist.reporters
   SET paged_volumes = 2
 WHERE reporter_standard = 'Dick';

INSERT INTO legalhist.reporters
  (reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, single_vol, type)
VALUES
  ('N.C.C.A.', 'Negligence and Compensation Cases Annotated', 'mixed', 'us:mc', 1912, NULL, false, 'specialized'),
  ('Tax Cas.', 'Reports of Tax Cases', 'national', 'uk:mc', 1875, NULL, false, 'specialized')
ON CONFLICT (reporter_standard) DO NOTHING;

-- N. J.: the Law Reports, then the Equity Reports.
UPDATE legalhist.whitelist
   SET reporter_standard = 'N.J.L.', fallback_standard = 'N.J. Eq.', fallback_any_miss = true
 WHERE reporter_standard = 'N.J.'
   AND reporter_found IN ('N. J.', 'N.J.', '(N. J.', '(N. J.,', 'N. J. ,.', 'N. J. .',
                          'N. J. ..', 'N. J.)', 'N. J.),', 'N. J.,', 'X. J.', 'New Jersey,');

-- Dick.: Dickens, then Dickinson.
UPDATE legalhist.whitelist
   SET reporter_standard = 'Dick', fallback_standard = 'Dick. Eq.'
 WHERE reporter_standard = 'Dick. Eq.'
   AND reporter_found IN ('Dick.', 'Dick');

UPDATE legalhist.whitelist
   SET reporter_standard = 'N.C.C.A.'
 WHERE reporter_standard = 'N.C. App.'
   AND reporter_found = 'N. C. C. A.';

UPDATE legalhist.whitelist
   SET reporter_standard = 'Tax Cas.'
 WHERE reporter_standard = 'T.C.'
   AND reporter_found IN ('Tax Cas.', 'Tax. Cas.', 'Tax Ca.', 'Tax Cases,');

-- migrate:down
SET ROLE = law_admin;

UPDATE legalhist.whitelist
   SET reporter_standard = 'T.C.'
 WHERE reporter_standard = 'Tax Cas.'
   AND reporter_found IN ('Tax Cas.', 'Tax. Cas.', 'Tax Ca.', 'Tax Cases,');

UPDATE legalhist.whitelist
   SET reporter_standard = 'N.C. App.'
 WHERE reporter_standard = 'N.C.C.A.'
   AND reporter_found = 'N. C. C. A.';

UPDATE legalhist.whitelist
   SET reporter_standard = 'Dick. Eq.', fallback_standard = NULL
 WHERE reporter_standard = 'Dick'
   AND reporter_found IN ('Dick.', 'Dick');

UPDATE legalhist.whitelist
   SET reporter_standard = 'N.J.', fallback_standard = NULL, fallback_any_miss = false
 WHERE reporter_standard = 'N.J.L.'
   AND fallback_standard = 'N.J. Eq.'
   AND reporter_found IN ('N. J.', 'N.J.', '(N. J.', '(N. J.,', 'N. J. ,.', 'N. J. .',
                          'N. J. ..', 'N. J.)', 'N. J.),', 'N. J.,', 'X. J.', 'New Jersey,');

-- The stubs minted for the two reporters go with them. A stub with curated
-- metadata blocks this on stub_case_metadata's foreign key, deliberately: move
-- or delete that metadata first.
DELETE FROM legalhist.stub_cases WHERE reporter_standard IN ('N.C.C.A.', 'Tax Cas.');
DELETE FROM legalhist.reporters WHERE reporter_standard IN ('N.C.C.A.', 'Tax Cas.');

ALTER TABLE legalhist.reporters
  DROP COLUMN IF EXISTS paged_volumes;

ALTER TABLE legalhist.whitelist
  DROP CONSTRAINT IF EXISTS chk_whitelist_fallback_any_miss;
ALTER TABLE legalhist.whitelist
  DROP COLUMN IF EXISTS fallback_any_miss;
