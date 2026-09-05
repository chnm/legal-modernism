-- migrate:up
SET ROLE = law_admin;

-- Alternate spellings for single-volume reporters that the whitelist already
-- recognizes but no single-volume detector looks for (issue #275, item 2,
-- part of #165). cite-detector-moml builds one bare-citation detector per
-- (single_vol reporter, spelling) pair from legalhist.reporters and
-- reporters_abbreviations, so a spelling that is only in legalhist.whitelist
-- is found solely when the generic detector sees a volume in front of it.
-- Measured 2026-09-05 with db/queries/single-vol-uncovered-spellings.sql:
-- 280 such spellings, 60,532 volumed rows. The full list with a proposed
-- action per row is db/single-vol-uncovered-spellings.tsv.
--
-- Seeded here: the 46 abbreviation-like spellings (initials, ampersands,
-- periods) whose whitelist routing is right, 33,485 volumed rows today.
-- Bare surnames and long forms ("Hopkins") are left out: the detector's \w*
-- stem would make a detector for one of those match every longer word. Each
-- row seeded here builds one detector on the next detection run and is probed
-- as an alternate spelling on the US linking route whatever the flag says.
--
-- Guards: the CHECK forbids alt_abbr = reporter_standard; the FK needs the
-- reporter row; the table has no unique constraint, so the exact pair must be
-- absent; and no alternate may itself be a reporter_standard, because such an
-- alternate cross-links two reporters (issue #272).
INSERT INTO legalhist.reporters_abbreviations (reporter_standard, alt_abbr)
SELECT v.reporter_standard, v.alt_abbr
FROM (VALUES
    ('Barn KB', 'Barn. K. B.'),  -- 148 volumed rows
    ('Bell', 'Bell C. C.'),  -- 71 volumed rows
    ('Busb.', 'Bus.'),  -- 137 volumed rows
    ('Car & M', 'C. & M.'),  -- 8622 volumed rows
    ('Car & M', 'C. & Mar.'),  -- 247 volumed rows
    ('Car & M', 'Carr. & M.'),  -- 153 volumed rows
    ('Car & M', 'C. & Marsh.'),  -- 77 volumed rows
    ('Colles', 'Coil.'),  -- 965 volumed rows
    ('Comst.', 'Corn.'),  -- 2531 volumed rows
    ('Comst.', 'Comst. R.'),  -- 524 volumed rows
    ('Comst.', 'Comst. (N. Y.)'),  -- 462 volumed rows
    ('Dears', 'Dears. C. C.'),  -- 538 volumed rows
    ('Dears & B', 'D. & B.'),  -- 2044 volumed rows
    ('Edw', 'Edw. Adm.'),  -- 69 volumed rows
    ('El Bl & El', 'E. B. & E.'),  -- 427 volumed rows
    ('Hoff. Ch.', 'Hoff. Ch. Pr.'),  -- 366 volumed rows
    ('Hoff. Ch.', 'Hoff. Ch. R.'),  -- 106 volumed rows
    ('Hoff. Ch.', 'Hoff. Pr.'),  -- 52 volumed rows
    ('Hoff. Ch.', 'Hoff. C. R.'),  -- 36 volumed rows
    ('Hoff. Ch.', 'Hoff. Ch. Rep.'),  -- 34 volumed rows
    ('Hoff. Ch.', 'Hoff. Ch. Prac.'),  -- 23 volumed rows
    ('Hopk. Ch.', 'Hopk.'),  -- 647 volumed rows
    ('Hopk. Ch.', 'Hopk. Ch. R.'),  -- 35 volumed rows
    ('Hopk. Ch.', 'Hopk. Ch. Rep.'),  -- 23 volumed rows
    ('Jac & W', 'Jac. and W.'),  -- 200 volumed rows
    ('Jac & W', 'Jacob & Walker'),  -- 115 volumed rows
    ('Johns', 'Johns. Ch'),  -- 175 volumed rows
    ('Le & Ca', 'L. & C.'),  -- 522 volumed rows
    ('Le & Ca', 'L. & C. C. C.'),  -- 39 volumed rows
    ('M & M', 'Mood. & M.'),  -- 498 volumed rows
    ('M & M', 'Moody & Malkin'),  -- 108 volumed rows
    ('Martin', 'Martin, R.'),  -- 857 volumed rows
    ('Martin', 'Martin R.'),  -- 130 volumed rows
    ('Peck', 'Pec.'),  -- 144 volumed rows
    ('Peck', 'Peek.'),  -- 102 volumed rows
    ('Russ & Ry', 'R. & R.'),  -- 417 volumed rows
    ('Russ & Ry', 'Russ. & R.'),  -- 69 volumed rows
    ('Ry & Mood', 'Ryan & Moody'),  -- 108 volumed rows
    ('S. & S.', 'S. & St.'),  -- 1720 volumed rows
    ('Saxton', 'Saxt. Ch.'),  -- 21 volumed rows
    ('Speers Eq.', 'Speer Eq.'),  -- 47 volumed rows
    ('Turn & R', 'Turn. and R.'),  -- 36 volumed rows
    ('Vern', 'Vcrn.'),  -- 4933 volumed rows
    ('Vern', 'Vern. R.'),  -- 1363 volumed rows
    ('Ves Sen Supp', 'B. Sup.'),  -- 3145 volumed rows
    ('Win.', 'WVm.')  -- 399 volumed rows
) AS v(reporter_standard, alt_abbr)
WHERE v.reporter_standard <> v.alt_abbr
  AND EXISTS (SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = v.reporter_standard)
  AND NOT EXISTS (SELECT 1 FROM legalhist.reporters r2 WHERE r2.reporter_standard = v.alt_abbr)
  AND NOT EXISTS (
    SELECT 1 FROM legalhist.reporters_abbreviations ra
    WHERE ra.reporter_standard = v.reporter_standard AND ra.alt_abbr = v.alt_abbr
  );

-- Routing corrections found by the same review. Each is a whitelist spelling
-- routed to a single-volume reporter that the cited volumes show it cannot be.
-- Reviewer: strike any of these you disagree with before applying.
--   Ch. Cas., Ch. Cas. Ch. -> Chan Cas (Cases in Chancery; cited at volumes 1-3,
--     5,075 rows, and Choyce Cases has one volume)
--   B. N. C. -> Bing NC (Bingham's New Cases; cited at volumes 1-8, 2,923 rows;
--     Brooke's New Cases has one volume)
--   Term -> junk (the word "Term" from Taylor's North Carolina Term Reports, 4,763 rows)
--   I). -> junk (an OCR fragment of "D.", 1,330 rows, routed to Daniell)
UPDATE legalhist.whitelist SET reporter_standard = 'Chan Cas'
 WHERE reporter_found IN ('Ch. Cas.', 'Ch. Cas. Ch.') AND reporter_standard = 'Choyce Cases'
   AND EXISTS (SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = 'Chan Cas');
UPDATE legalhist.whitelist SET reporter_standard = 'Bing NC'
 WHERE reporter_found = 'B. N. C.' AND reporter_standard = 'Brooke'
   AND EXISTS (SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = 'Bing NC');
UPDATE legalhist.whitelist SET junk = true, reporter_standard = NULL
 WHERE reporter_found = 'Term' AND reporter_standard = 'Taylor';
UPDATE legalhist.whitelist SET junk = true, reporter_standard = NULL
 WHERE reporter_found = 'I).' AND reporter_standard = 'Dan';

-- migrate:down
SET ROLE = law_admin;

UPDATE legalhist.whitelist SET junk = false, reporter_standard = 'Dan'
 WHERE reporter_found = 'I).' AND junk = true;
UPDATE legalhist.whitelist SET junk = false, reporter_standard = 'Taylor'
 WHERE reporter_found = 'Term' AND junk = true;
UPDATE legalhist.whitelist SET reporter_standard = 'Brooke'
 WHERE reporter_found = 'B. N. C.' AND reporter_standard = 'Bing NC';
UPDATE legalhist.whitelist SET reporter_standard = 'Choyce Cases'
 WHERE reporter_found IN ('Ch. Cas.', 'Ch. Cas. Ch.') AND reporter_standard = 'Chan Cas';

-- Remove exactly the pairs seeded above.
DELETE FROM legalhist.reporters_abbreviations ra
USING (VALUES
    ('Barn KB', 'Barn. K. B.'),
    ('Bell', 'Bell C. C.'),
    ('Busb.', 'Bus.'),
    ('Car & M', 'C. & M.'),
    ('Car & M', 'C. & Mar.'),
    ('Car & M', 'Carr. & M.'),
    ('Car & M', 'C. & Marsh.'),
    ('Colles', 'Coil.'),
    ('Comst.', 'Corn.'),
    ('Comst.', 'Comst. R.'),
    ('Comst.', 'Comst. (N. Y.)'),
    ('Dears', 'Dears. C. C.'),
    ('Dears & B', 'D. & B.'),
    ('Edw', 'Edw. Adm.'),
    ('El Bl & El', 'E. B. & E.'),
    ('Hoff. Ch.', 'Hoff. Ch. Pr.'),
    ('Hoff. Ch.', 'Hoff. Ch. R.'),
    ('Hoff. Ch.', 'Hoff. Pr.'),
    ('Hoff. Ch.', 'Hoff. C. R.'),
    ('Hoff. Ch.', 'Hoff. Ch. Rep.'),
    ('Hoff. Ch.', 'Hoff. Ch. Prac.'),
    ('Hopk. Ch.', 'Hopk.'),
    ('Hopk. Ch.', 'Hopk. Ch. R.'),
    ('Hopk. Ch.', 'Hopk. Ch. Rep.'),
    ('Jac & W', 'Jac. and W.'),
    ('Jac & W', 'Jacob & Walker'),
    ('Johns', 'Johns. Ch'),
    ('Le & Ca', 'L. & C.'),
    ('Le & Ca', 'L. & C. C. C.'),
    ('M & M', 'Mood. & M.'),
    ('M & M', 'Moody & Malkin'),
    ('Martin', 'Martin, R.'),
    ('Martin', 'Martin R.'),
    ('Peck', 'Pec.'),
    ('Peck', 'Peek.'),
    ('Russ & Ry', 'R. & R.'),
    ('Russ & Ry', 'Russ. & R.'),
    ('Ry & Mood', 'Ryan & Moody'),
    ('S. & S.', 'S. & St.'),
    ('Saxton', 'Saxt. Ch.'),
    ('Speers Eq.', 'Speer Eq.'),
    ('Turn & R', 'Turn. and R.'),
    ('Vern', 'Vcrn.'),
    ('Vern', 'Vern. R.'),
    ('Ves Sen Supp', 'B. Sup.'),
    ('Win.', 'WVm.')
) AS v(reporter_standard, alt_abbr)
WHERE ra.reporter_standard = v.reporter_standard
  AND ra.alt_abbr = v.alt_abbr;
