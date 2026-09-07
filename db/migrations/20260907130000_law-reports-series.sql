-- migrate:up
SET ROLE = law_admin;

-- The Law Reports: the "L. R." series prefix and the year-cited continuations
-- (issue #314, following #312 and #248).
--
-- Two habits of citing the Law Reports defeat the detector and the whitelist.
-- The 1865-1875 series are cited with the series prefix ahead of the volume,
-- "L. R. 5 Ch. 100": the detector records "5 Ch. 100" and the whitelist sends
-- it to whatever older reporter the bare abbreviation names -- Cases in
-- Chancery of 1660-1698 for "Ch.", Clark's House of Lords Cases for "H. L.",
-- Hilton's New York Common Pleas for "C. P." -- where, those reporters being in
-- the English Reports or CAP, it links to a case decades off. Measured on a
-- random sample of the OCR text, an "L. R." stands before 60% of "C. P." and
-- "H. L.", 44% of "Eq.", 15% of "Ch." and 13-18% of "Q. B.". And the series of
-- 1891 on are cited by year on spellings shared with volume-cited series:
-- "[1895] 2 Q. B. 1" is 32-50% of "Q. B." and reached the 1842 volume of
-- Adolphus & Ellis, "[1901] 1 Ch. 1" is 53% of "Ch.".
--
-- The detector now moves the prefix behind the volume, so "L. R. 5 Ch. 100"
-- is detected as "5 L. R. Ch. 100" with the spelling "L. R. Ch."
-- (citations.NormalizeSeriesPrefix), and this migration gives the whitelist
-- the "L. R. {series}" spellings to route. For the year-cited continuations,
-- cited_by_year_from replaces the boolean flag of #312: the first year from
-- which a reporter is cited by year. A year from then on goes into the cite
-- string and the stub key; a year before it is decoration on a volume-cited
-- citation. One row can then hold a series and its continuation, and the
-- linker keeps the year on the reporter ("[1895] Q.B.") so a year-bearing
-- string never reaches a volume-cited index. Each data block below stands on
-- its own so that a judgment call can be dropped.

-- 1. cited_by_year_from replaces cited_by_year. K.B. and L.R.Ir. carry the
--    boolean from 20260907120000; the Irish Reports are cited by year from
--    1894, which also keeps a parenthetical year on the 4th series (1878-1893)
--    out of the key, the caveat #312 recorded.
ALTER TABLE legalhist.reporters
  ADD COLUMN IF NOT EXISTS cited_by_year_from integer;
UPDATE legalhist.reporters r
   SET cited_by_year_from = v.from_year
  FROM (VALUES ('K.B.', 1901), ('L.R.Ir.', 1894)) AS v(std, from_year)
 WHERE r.reporter_standard = v.std
   AND r.cited_by_year_from IS NULL;
ALTER TABLE legalhist.reporters
  DROP COLUMN IF EXISTS cited_by_year;

-- 2. The continuations that share a row with a volume-cited series. QB is
--    Adolphus & Ellis' New Series (1841-1852) and, from 1891, the Law
--    Reports' Queen's Bench; L.R.A.C. is the Appeal Cases (1875-1890) and,
--    from 1891, A.C., which has no volume; the Scots Law Times numbers its
--    volumes 1-16 to 1908 and is cited by year after.
UPDATE legalhist.reporters r
   SET cited_by_year_from = v.from_year
  FROM (VALUES ('QB', 1891), ('L.R.A.C.', 1891), ('S.L.T.', 1909)) AS v(std, from_year)
 WHERE r.reporter_standard = v.std
   AND r.cited_by_year_from IS DISTINCT FROM v.from_year;
UPDATE legalhist.reporters
   SET reporter_title = 'Queen''s Bench: Adolphus & Ellis, New Series (1841-1852), and the Law Reports Queen''s Bench, cited by year from 1891'
 WHERE reporter_standard = 'QB' AND reporter_title = 'Queen''s Bench';
UPDATE legalhist.reporters
   SET reporter_title = 'Law Reports, Appeal Cases (App. Cas. 1875-1890; A.C., cited by year, from 1891)'
 WHERE reporter_standard = 'L.R.A.C.' AND reporter_title = 'Law Reports, Appeal Cases';

-- 3. New rows: the 1865-1875 series that had none, the year-cited Chancery
--    Division, and the English Law & Equity Reports split off C.L.R. below.
INSERT INTO legalhist.reporters
    (reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, single_vol, type, cited_by_year_from)
SELECT v.*
FROM (VALUES
    ('Ch.',           'Law Reports, Chancery Division, cited by year', 'national', 'uk:ch', 1891, NULL, false, 'official', 1891),
    ('L.R.Q.B.',      'Law Reports, Queen''s Bench',                   'national', 'uk:kb', 1865, 1875, false, 'official', NULL),
    ('L.R. Eq.',      'Law Reports, Equity Cases',                     'national', 'uk:ch', 1865, 1875, false, 'official', NULL),
    ('L.R. Exch.',    'Law Reports, Exchequer',                        'national', 'uk:ex', 1865, 1875, false, 'official', NULL),
    ('Eng. L. & Eq.', 'English Law and Equity Reports',                'national', 'uk:mc', 1851, 1868, false, 'specialized', NULL)
) AS v(reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, single_vol, type, cited_by_year_from)
WHERE NOT EXISTS (SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = v.reporter_standard);

-- 4. Whitelist repairs. Each is guarded by the mapping it replaces, so a
--    hand edit that got there first is left alone.
--    "A. & E." is Adolphus & Ellis: 85% of its citations have neither an "L.
--    R." prefix nor a year and its volumes run 1-12, the run of that
--    reporter; the Admiralty & Ecclesiastical series it was mapped to has
--    four volumes and is cited "L. R. 3 A. & E.", which the prefix now
--    keeps apart. 91,229 citations, and Ad & E is in the English Reports.
UPDATE legalhist.whitelist SET reporter_standard = 'Ad & E'
 WHERE reporter_found IN ('A. & E.', 'A. and E.', 'A. &E.') AND reporter_standard = 'L.R.A.E.';
--    "Eq." is the Law Reports' Equity Cases: 44% carry the prefix and the
--    volumes run 1-20, that series' run. C.L.R. never was an equity reporter.
UPDATE legalhist.whitelist SET reporter_standard = 'L.R. Eq.'
 WHERE reporter_found IN ('Eq.', 'Eq. R.') AND reporter_standard = 'C.L.R.';
UPDATE legalhist.whitelist SET reporter_standard = 'L.R. Eq.'
 WHERE reporter_found = 'L. R. Eq.' AND reporter_standard = 'L.R.Ch.';
UPDATE legalhist.reporters_abbreviations SET reporter_standard = 'L.R. Eq.'
 WHERE alt_abbr IN ('Eq.R.', 'Eq.Rep.', 'Equity Rep.') AND reporter_standard = 'C.L.R.';
--    "C. P." is the Law Reports' Common Pleas: 60% carry the prefix and the
--    volumes run 1-10; Hilton has two. (Judgment call: the other 40% are not
--    Hilton either -- a New York treatise cites "Hilt." -- but are not
--    identified.)
UPDATE legalhist.whitelist SET reporter_standard = 'L.R.C.P.'
 WHERE reporter_found = 'C. P.' AND reporter_standard = 'Hilt.';
--    Bare "Ch." and its OCR variants are the Chancery Division cited by year
--    (53% carry a year) or the Chancery Appeals with the prefix (15%), and at
--    most a third of them could be the two volumes of Cases in Chancery,
--    which keeps its own spellings ("Ch. Ca.", "Ch. Cas."). Judgment call.
UPDATE legalhist.whitelist SET reporter_standard = 'Ch.'
 WHERE reporter_found IN ('Ch.', 'ch.', 'Cb.', 'Chi.', 'Chl.', 'Clh.', 'Cli.', 'Cll.') AND reporter_standard = 'Chan Cas';
--    The series-prefixed spellings that were pointed at the wrong series.
UPDATE legalhist.whitelist SET reporter_standard = 'L.R. Ch. App.'
 WHERE reporter_found = 'L. R. Ch.' AND reporter_standard = 'L.R.Ch.';
UPDATE legalhist.whitelist SET reporter_standard = 'L.R.Q.B.'
 WHERE reporter_found = 'L. R. Q. B.' AND reporter_standard = 'L.R.K.B.';
UPDATE legalhist.whitelist SET reporter_standard = 'L.R. Exch.'
 WHERE reporter_found = 'L. R. Exch.' AND reporter_standard = 'L.R.Ex.';
--    "L. J. C." is the Law Journal's Chancery series: half of its volumes
--    exceed the 44 that the Common Pleas series ran to, and every one of a
--    random sample of its high-page citations sits among Chancery
--    companions ("Rolls v. Miller, 53 L. J. C. 682; 27 Ch. D. 71"). The
--    small-page rows were the Common Pleas series with "P." misread, and
--    20260906140100 corrects those in the text. Judgment call.
UPDATE legalhist.whitelist SET reporter_standard = 'L.J.Ch.'
 WHERE reporter_found = 'L. J. C.' AND reporter_standard = 'L.J.C.P.';
--    English Law & Equity Reports, an American reprint series, off C.L.R.
UPDATE legalhist.whitelist SET reporter_standard = 'Eng. L. & Eq.'
 WHERE reporter_found IN ('Eng. Law & Eq.', 'Eng. L. & Eq.') AND reporter_standard = 'C.L.R.';

-- 5. The "L. R. {series}" spellings the normalized form produces, from a
--    sample of 6,000 pages that mention "L. R.": Eq. 143, Q. B. 105, Ch. 78,
--    C. P. 71, Ex. 51, H. L. 27, App. 21, P. C. 20, Exch. 19, Ch. D. 14,
--    Q. B. D. 13, App. Cas. 12, Ch. App. 11, P. & D. 8, and a tail. The
--    whitelist-extender will show what this misses after the next detection.
INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk) VALUES
    ('L. R. Eq. Cas.',      'L.R. Eq.',      false),
    ('L. R. Q.B.',          'L.R.Q.B.',      false),
    ('L. R. Ch. Div.',      'L.R.Ch.',       false),
    ('L. R. Ch. D.',        'L.R.Ch.',       false),
    ('L. R. C. D.',         'L.R.Ch.',       false),
    ('L. R. C.P.',          'L.R.C.P.',      false),
    ('L. R. C. P. D.',      'C. P. D.',      false),
    ('L. R. C. P. Div.',    'C. P. D.',      false),
    ('L. R. Ex.',           'L.R. Exch.',    false),
    ('L. R. Ex. D.',        'L.R.Ex.',       false),
    ('L. R. Exch. Div.',    'L.R.Ex.',       false),
    ('L. R. H. L.',         'L. R. H. L.',   false),
    ('L. R. H.L.',          'L. R. H. L.',   false),
    ('L. R. H. L. Sc.',     'L. R. H. L.',   false),
    ('L. R. Sc. & Div.',    'L. R. H. L.',   false),
    ('L. R. E. & I. App.',  'L. R. H. L.',   false),
    ('L. R. P.C.',          'L.R.P.C.',      false),
    ('L. R. App. Cas.',     'L.R.A.C.',      false),
    ('L. R. App. Ca.',      'L.R.A.C.',      false),
    ('L. R. App. C.',       'L.R.A.C.',      false),
    ('L. R. App.',          'L.R.A.C.',      false),
    ('L. R. Q. B. D.',      'Q.B.D.',        false),
    ('L. R. Q.B. D.',       'Q.B.D.',        false),
    ('L. R. Q. B. Div.',    'Q.B.D.',        false),
    ('L. R. P. D.',         'P.D.',          false),
    ('L. R. A. & E.',       'L.R.A.E.',      false),
    ('L. R. Ad. & E.',      'L.R.A.E.',      false),
    ('L. R. Adm. & Ecc.',   'L.R.A.E.',      false)
ON CONFLICT (reporter_found) DO NOTHING;

-- 6. Row fixes. L.R.A. (n.s.) is the American Lawyers' Reports Annotated,
--    New Series; the UK jurisdiction routed it to the English Reports, where
--    it can never link, instead of to CAP and the FreeLaw crosswalk, which
--    knows it as L.R.A.N.S. (already an alternate). Nevile & Manning is a
--    King's Bench reporter. C.L.R. is cited at 342 volumes, which is the
--    Commonwealth Law Reports of Australia, not the three volumes of the
--    Common Law Reports of 1853-1855 (judgment call: retitle only, no source
--    holds either).
UPDATE legalhist.reporters SET jurisdiction = 'us:mc'
 WHERE reporter_standard = 'L.R.A. (n.s.)' AND jurisdiction = 'uk:mc';
UPDATE legalhist.reporters SET jurisdiction = 'uk:kb'
 WHERE reporter_standard = 'N. & M.' AND jurisdiction = 'us:us';
UPDATE legalhist.reporters
   SET reporter_title = 'Commonwealth Law Reports (Australia)', year_start = 1903, year_end = NULL
 WHERE reporter_standard = 'C.L.R.' AND reporter_title = 'Common Law & Equity Reports';

-- 7. L.R.K.B. duplicated K.B.; its one spelling moved above, and its one
--    alternate was "K.B." itself.
DELETE FROM legalhist.reporters_abbreviations WHERE reporter_standard = 'L.R.K.B.';
DELETE FROM legalhist.stub_cases WHERE reporter_standard = 'L.R.K.B.';
DELETE FROM legalhist.reporters r
 WHERE r.reporter_standard = 'L.R.K.B.'
   AND NOT EXISTS (SELECT 1 FROM legalhist.whitelist w WHERE w.reporter_standard = r.reporter_standard)
   AND NOT EXISTS (SELECT 1 FROM legalhist.reporters_diffvols d WHERE d.reporter_standard = r.reporter_standard);

-- 8. A stub for a year-cited citation without a volume: "[1893] A.C. 22".
ALTER TABLE legalhist.stub_cases ALTER COLUMN volume DROP NOT NULL;
ALTER TABLE legalhist.stub_cases DROP CONSTRAINT IF EXISTS stub_cases_volume_check;
ALTER TABLE legalhist.stub_cases DROP CONSTRAINT IF EXISTS stub_cases_volume_or_year_check;
ALTER TABLE legalhist.stub_cases
  ADD CONSTRAINT stub_cases_volume_or_year_check CHECK ((volume IS NOT NULL AND volume > 0) OR year IS NOT NULL);

-- migrate:down
SET ROLE = law_admin;

DELETE FROM legalhist.stub_cases WHERE volume IS NULL;
ALTER TABLE legalhist.stub_cases DROP CONSTRAINT IF EXISTS stub_cases_volume_or_year_check;
ALTER TABLE legalhist.stub_cases ALTER COLUMN volume SET NOT NULL;
ALTER TABLE legalhist.stub_cases DROP CONSTRAINT IF EXISTS stub_cases_volume_check;
ALTER TABLE legalhist.stub_cases ADD CONSTRAINT stub_cases_volume_check CHECK (volume > 0);

INSERT INTO legalhist.reporters (reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, type)
SELECT 'L.R.K.B.', 'Law Reports, King''s Bench', 'national', 'uk:kb', 1901, 1952, 'specialized'
 WHERE NOT EXISTS (SELECT 1 FROM legalhist.reporters WHERE reporter_standard = 'L.R.K.B.');
INSERT INTO legalhist.reporters_abbreviations (reporter_standard, alt_abbr)
SELECT 'L.R.K.B.', 'K.B.'
 WHERE NOT EXISTS (SELECT 1 FROM legalhist.reporters_abbreviations WHERE reporter_standard = 'L.R.K.B.' AND alt_abbr = 'K.B.');

UPDATE legalhist.reporters
   SET reporter_title = 'Common Law & Equity Reports', year_start = 1853, year_end = 1855
 WHERE reporter_standard = 'C.L.R.' AND reporter_title = 'Commonwealth Law Reports (Australia)';
UPDATE legalhist.reporters SET jurisdiction = 'us:us'
 WHERE reporter_standard = 'N. & M.' AND jurisdiction = 'uk:kb';
UPDATE legalhist.reporters SET jurisdiction = 'uk:mc'
 WHERE reporter_standard = 'L.R.A. (n.s.)' AND jurisdiction = 'us:mc';

DELETE FROM legalhist.whitelist
 WHERE reporter_found IN (
    'L. R. Eq. Cas.', 'L. R. Q.B.', 'L. R. Ch. Div.', 'L. R. Ch. D.', 'L. R. C. D.', 'L. R. C.P.',
    'L. R. C. P. D.', 'L. R. C. P. Div.', 'L. R. Ex.', 'L. R. Ex. D.', 'L. R. Exch. Div.', 'L. R. H. L.',
    'L. R. H.L.', 'L. R. H. L. Sc.', 'L. R. Sc. & Div.', 'L. R. E. & I. App.', 'L. R. P.C.',
    'L. R. App. Cas.', 'L. R. App. Ca.', 'L. R. App. C.', 'L. R. App.', 'L. R. Q. B. D.', 'L. R. Q.B. D.',
    'L. R. Q. B. Div.', 'L. R. P. D.', 'L. R. A. & E.', 'L. R. Ad. & E.', 'L. R. Adm. & Ecc.');

UPDATE legalhist.whitelist SET reporter_standard = 'C.L.R.'
 WHERE reporter_found IN ('Eng. Law & Eq.', 'Eng. L. & Eq.') AND reporter_standard = 'Eng. L. & Eq.';
UPDATE legalhist.whitelist SET reporter_standard = 'L.J.C.P.'
 WHERE reporter_found = 'L. J. C.' AND reporter_standard = 'L.J.Ch.';
UPDATE legalhist.whitelist SET reporter_standard = 'L.R.Ex.'
 WHERE reporter_found = 'L. R. Exch.' AND reporter_standard = 'L.R. Exch.';
UPDATE legalhist.whitelist SET reporter_standard = 'L.R.K.B.'
 WHERE reporter_found = 'L. R. Q. B.' AND reporter_standard = 'L.R.Q.B.';
UPDATE legalhist.whitelist SET reporter_standard = 'L.R.Ch.'
 WHERE reporter_found = 'L. R. Ch.' AND reporter_standard = 'L.R. Ch. App.';
UPDATE legalhist.whitelist SET reporter_standard = 'Chan Cas'
 WHERE reporter_found IN ('Ch.', 'ch.', 'Cb.', 'Chi.', 'Chl.', 'Clh.', 'Cli.', 'Cll.') AND reporter_standard = 'Ch.';
UPDATE legalhist.whitelist SET reporter_standard = 'Hilt.'
 WHERE reporter_found = 'C. P.' AND reporter_standard = 'L.R.C.P.';
UPDATE legalhist.reporters_abbreviations SET reporter_standard = 'C.L.R.'
 WHERE alt_abbr IN ('Eq.R.', 'Eq.Rep.', 'Equity Rep.') AND reporter_standard = 'L.R. Eq.';
UPDATE legalhist.whitelist SET reporter_standard = 'L.R.Ch.'
 WHERE reporter_found = 'L. R. Eq.' AND reporter_standard = 'L.R. Eq.';
UPDATE legalhist.whitelist SET reporter_standard = 'C.L.R.'
 WHERE reporter_found IN ('Eq.', 'Eq. R.') AND reporter_standard = 'L.R. Eq.';
UPDATE legalhist.whitelist SET reporter_standard = 'L.R.A.E.'
 WHERE reporter_found IN ('A. & E.', 'A. and E.', 'A. &E.') AND reporter_standard = 'Ad & E';

DELETE FROM legalhist.stub_cases WHERE reporter_standard IN ('Ch.', 'L.R.Q.B.', 'L.R. Eq.', 'L.R. Exch.', 'Eng. L. & Eq.');
DELETE FROM legalhist.reporters r
 WHERE r.reporter_standard IN ('Ch.', 'L.R.Q.B.', 'L.R. Eq.', 'L.R. Exch.', 'Eng. L. & Eq.')
   AND NOT EXISTS (SELECT 1 FROM legalhist.whitelist w WHERE w.reporter_standard = r.reporter_standard)
   AND NOT EXISTS (SELECT 1 FROM legalhist.reporters_abbreviations a WHERE a.reporter_standard = r.reporter_standard)
   AND NOT EXISTS (SELECT 1 FROM legalhist.reporters_diffvols d WHERE d.reporter_standard = r.reporter_standard);

UPDATE legalhist.reporters SET reporter_title = 'Law Reports, Appeal Cases'
 WHERE reporter_standard = 'L.R.A.C.' AND reporter_title LIKE 'Law Reports, Appeal Cases (%';
UPDATE legalhist.reporters SET reporter_title = 'Queen''s Bench'
 WHERE reporter_standard = 'QB' AND reporter_title LIKE 'Queen''s Bench: %';
UPDATE legalhist.reporters SET cited_by_year_from = NULL
 WHERE reporter_standard IN ('QB', 'L.R.A.C.', 'S.L.T.');

ALTER TABLE legalhist.reporters ADD COLUMN IF NOT EXISTS cited_by_year boolean;
UPDATE legalhist.reporters SET cited_by_year = true
 WHERE reporter_standard IN ('K.B.', 'L.R.Ir.') AND cited_by_year IS DISTINCT FROM true;
ALTER TABLE legalhist.reporters DROP COLUMN IF EXISTS cited_by_year_from;
