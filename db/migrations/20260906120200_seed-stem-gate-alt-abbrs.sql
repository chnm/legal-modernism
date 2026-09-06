-- migrate:up
SET ROLE = law_admin;

-- Restore detection for the four whitelisted spellings that the narrowed
-- single-volume stem no longer reaches (issue #283).
--
-- cite-detector-moml builds one bare-citation detector per (single_vol
-- reporter, spelling) pair from legalhist.reporters and
-- reporters_abbreviations. Until now a short abbreviation could stem into a
-- longer word -- "Bur" reached "Burrcll" through \w* -- which is how these
-- spellings were found even though none is registered. The stem is now applied
-- only to abbreviations of five characters or more, counted without whitespace
-- or periods, because on a short one it reached far more ordinary words than
-- reporters (1,466,414 rows for 137 links). Registering the spellings that were
-- worth having is the remedy the same review used in 20260905120300.
--
-- Re-running db/queries/single-vol-uncovered-spellings.sql with the gate leaves
-- exactly four spellings uncovered that were covered before. Every one is an
-- OCR corruption of a single-volume reporter, already whitelisted to that
-- reporter. Counts are from the corpus as it stood on 2026-09-06, before the
-- rebuild truncated it:
--
--   Burrcll -> Burrell (uk:ad)  150 bare rows, 93 linked to the English Reports
--                               (an e read as c; "Bur" is 3 characters)
--   Taytor  -> Taylor  (us:nc)   37 bare rows, 37 linked to CAP
--                               (an l read as t; "Tay" is 3 characters)
--   Hetl    -> Het     (uk:cp)   18 bare rows,  7 linked to the English Reports
--                               (the existing alternate "Hetl." requires a
--                               literal period, so bare "Hetl" matched nothing)
--   Burnet  -> Bur.    (us:wi) 2,233 rows, none linked -- see below
--
-- The first three are the whole cost of the stem gate: 137 links, recovered
-- here in full.
--
-- Guards, as in 20260905120300: the CHECK forbids alt_abbr = reporter_standard;
-- the FK needs the reporter row; the table has no unique constraint, so the
-- exact pair must be absent; and no alternate may itself be a
-- reporter_standard, because such an alternate cross-links two reporters
-- (issues #272 and #289).
INSERT INTO legalhist.reporters_abbreviations (reporter_standard, alt_abbr)
SELECT v.reporter_standard, v.alt_abbr
FROM (VALUES
    ('Burrell', 'Burrcll'),  -- 150 bare rows, 93 linked
    ('Taylor', 'Taytor'),    -- 37 bare rows, 37 linked
    ('Het', 'Hetl'),         -- 18 bare rows, 7 linked
    ('Bur.', 'Burnet')       -- 2,233 rows, none linked
) AS v(reporter_standard, alt_abbr)
WHERE v.reporter_standard <> v.alt_abbr
  AND EXISTS (SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = v.reporter_standard)
  AND NOT EXISTS (SELECT 1 FROM legalhist.reporters r2 WHERE r2.reporter_standard = v.alt_abbr)
  AND NOT EXISTS (
    SELECT 1 FROM legalhist.reporters_abbreviations ra
    WHERE ra.reporter_standard = v.reporter_standard AND ra.alt_abbr = v.alt_abbr
  );

-- The fourth spelling needs its whitelist row corrected as well, because the
-- row as it stands cannot ever be reached.
--
-- legalhist.whitelist holds 'Burnet,' -- with a trailing comma -- routed to
-- 'Bur.' and not junk. Detect ends with TrimRight(TrimSpace(abbr), " ,"), so no
-- detected spelling can end in a comma: the row had 0 rows in the old corpus
-- and could never acquire any. It is an artifact of an older detector that kept
-- trailing commas. The live spelling is 'Burnet', which had 2,233 rows (2,093
-- bare, 140 volumed), every one skipped_not_whitelisted because the whitelist
-- knows only the comma form.
--
-- 'Bur.' already carries the alternates 'Burnett' and 'Burnett (Wis.)', so the
-- reporter is Burnett's Wisconsin Reports and 'Burnet' is a one-t variant of
-- it. Correcting the row restores the routing decision a human already made,
-- rather than making a new one.
--
-- reporter_found is the primary key. Pre-checked read-only: no 'Burnet' row
-- exists, so the update cannot collide with one. The row keeps junk = false and
-- reporter_standard = 'Bur.', satisfying chk_whitelist_junk_no_standard,
-- chk_whitelist_nonjunk_has_standard and the FK to legalhist.reporters.
--
-- Not fixed here, and unchanged by this migration: 'Burnett' is an alternate
-- with no whitelist row of its own, so text reading "Burnett" is detected and
-- then skipped as not-whitelisted. 'Burnet' keeps the stem at six characters,
-- so its detector also matches "Burnett" and records it under the spelling that
-- matched, as every detector does -- which leaves that text exactly where it
-- was. Whether to whitelist 'Burnett' is a separate decision.
UPDATE legalhist.whitelist SET reporter_found = 'Burnet'
 WHERE reporter_found = 'Burnet,'
   AND reporter_standard = 'Bur.'
   AND NOT EXISTS (
     SELECT 1 FROM legalhist.whitelist w2 WHERE w2.reporter_found = 'Burnet'
   );

-- migrate:down
SET ROLE = law_admin;

UPDATE legalhist.whitelist SET reporter_found = 'Burnet,'
 WHERE reporter_found = 'Burnet'
   AND reporter_standard = 'Bur.'
   AND NOT EXISTS (
     SELECT 1 FROM legalhist.whitelist w2 WHERE w2.reporter_found = 'Burnet,'
   );

-- Remove exactly the pairs seeded above.
DELETE FROM legalhist.reporters_abbreviations ra
USING (VALUES
    ('Burrell', 'Burrcll'),
    ('Taylor', 'Taytor'),
    ('Het', 'Hetl'),
    ('Bur.', 'Burnet')
) AS v(reporter_standard, alt_abbr)
WHERE ra.reporter_standard = v.reporter_standard
  AND ra.alt_abbr = v.alt_abbr;
