-- migrate:up
SET ROLE = law_admin;

-- Volume renumbering for two Pennsylvania nominate reporters (issue #246,
-- part of #165). The Pennsylvania State Reports were cited by their reporter's
-- name for the first 107 volumes, and CAP holds them as "N Pa. P", so a
-- citation like "5 Harris 120" has to be probed as "17 Pa. 120". That is
-- what legalhist.reporters_diffvols is for, and the table already carries
-- Barr (Pa. 1-10), P.F. Smith (51-81) and Norris (82-96), but not Harris
-- (13-24) or Casey (25-36). Measured 2026-09-05: Harris has 7,291 no_match
-- rows and Casey 6,469, cited at volumes 1-12 in both cases, and CAP holds
-- 2,741 distinct first-page cites in Pa. volumes 13-36.
--
-- Outerbridge (Pa. 97-107) is the third case, found while reviewing the
-- OCR-variant candidates for #247: the spelling "Out." is not whitelisted at
-- all, and the harness proposed it as Ontario. Its 1,075 rows are cited at
-- volumes 1-9, spread evenly, which is Outerbridge's run, not Ontario's. It
-- needs a reporter row and a whitelist row before its diffvols rows can be
-- reached, so all three are seeded here; the down removes them in reverse.
--
-- Jones (Pa. 11-12) and Wright (Pa. 37-50) are not seeded: the whitelist
-- routes "Jones" to Jones' North Carolina Law Reports and "Wright" to
-- Wright's Ohio Reports, so their Pennsylvania volumes need a routing decision
-- before a diffvols row would reach them.
--
-- Seeding any row for a reporter switches cite-linker to the diffvols path
-- for it (WhitelistEntry.CAPDifferent), and a cited volume outside 1-12 is
-- then tiered us_diffvols_missing rather than guessed at. The table has no
-- unique constraint, so each row is inserted only when its
-- (reporter_standard, vol) pair is absent.

INSERT INTO legalhist.reporters
    (reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, single_vol, type)
SELECT 'Out.', 'Outerbridge''s Pennsylvania State Reports', 'state', 'us:pa', 1881, 1885, false, 'nominate'
WHERE NOT EXISTS (SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = 'Out.');

INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk)
VALUES ('Out.', 'Out.', false),        -- 1,075 rows
       ('Outerbridge', 'Out.', false)  -- 51 rows
ON CONFLICT (reporter_found) DO NOTHING;

INSERT INTO legalhist.reporters_diffvols (reporter_title, vol, cap_vol, cap_reporter, reporter_standard)
SELECT v.reporter_title, v.vol, v.cap_vol, 'Pa.', v.reporter_standard
FROM (VALUES
    ('Harris''s Pennsylvania Reports', 1, 13, 'Harris'),
    ('Harris''s Pennsylvania Reports', 2, 14, 'Harris'),
    ('Harris''s Pennsylvania Reports', 3, 15, 'Harris'),
    ('Harris''s Pennsylvania Reports', 4, 16, 'Harris'),
    ('Harris''s Pennsylvania Reports', 5, 17, 'Harris'),
    ('Harris''s Pennsylvania Reports', 6, 18, 'Harris'),
    ('Harris''s Pennsylvania Reports', 7, 19, 'Harris'),
    ('Harris''s Pennsylvania Reports', 8, 20, 'Harris'),
    ('Harris''s Pennsylvania Reports', 9, 21, 'Harris'),
    ('Harris''s Pennsylvania Reports', 10, 22, 'Harris'),
    ('Harris''s Pennsylvania Reports', 11, 23, 'Harris'),
    ('Harris''s Pennsylvania Reports', 12, 24, 'Harris'),
    ('Casey''s Pennsylvania Reports', 1, 25, 'Casey'),
    ('Casey''s Pennsylvania Reports', 2, 26, 'Casey'),
    ('Casey''s Pennsylvania Reports', 3, 27, 'Casey'),
    ('Casey''s Pennsylvania Reports', 4, 28, 'Casey'),
    ('Casey''s Pennsylvania Reports', 5, 29, 'Casey'),
    ('Casey''s Pennsylvania Reports', 6, 30, 'Casey'),
    ('Casey''s Pennsylvania Reports', 7, 31, 'Casey'),
    ('Casey''s Pennsylvania Reports', 8, 32, 'Casey'),
    ('Casey''s Pennsylvania Reports', 9, 33, 'Casey'),
    ('Casey''s Pennsylvania Reports', 10, 34, 'Casey'),
    ('Casey''s Pennsylvania Reports', 11, 35, 'Casey'),
    ('Casey''s Pennsylvania Reports', 12, 36, 'Casey'),
    ('Outerbridge''s Pennsylvania State Reports', 1, 97, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 2, 98, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 3, 99, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 4, 100, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 5, 101, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 6, 102, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 7, 103, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 8, 104, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 9, 105, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 10, 106, 'Out.'),
    ('Outerbridge''s Pennsylvania State Reports', 11, 107, 'Out.')
) AS v(reporter_title, vol, cap_vol, reporter_standard)
WHERE EXISTS (
    SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = v.reporter_standard
)
AND NOT EXISTS (
    SELECT 1 FROM legalhist.reporters_diffvols d
    WHERE d.reporter_standard = v.reporter_standard AND d.vol = v.vol
);

-- migrate:down
SET ROLE = law_admin;

-- Remove exactly the 35 rows seeded above, then the Outerbridge whitelist and
-- reporter rows (in that order, for the foreign keys).
DELETE FROM legalhist.reporters_diffvols
 WHERE cap_reporter = 'Pa.'
   AND (reporter_standard, vol) IN (
    ('Harris', 1),
    ('Harris', 2),
    ('Harris', 3),
    ('Harris', 4),
    ('Harris', 5),
    ('Harris', 6),
    ('Harris', 7),
    ('Harris', 8),
    ('Harris', 9),
    ('Harris', 10),
    ('Harris', 11),
    ('Harris', 12),
    ('Casey', 1),
    ('Casey', 2),
    ('Casey', 3),
    ('Casey', 4),
    ('Casey', 5),
    ('Casey', 6),
    ('Casey', 7),
    ('Casey', 8),
    ('Casey', 9),
    ('Casey', 10),
    ('Casey', 11),
    ('Casey', 12),
    ('Out.', 1),
    ('Out.', 2),
    ('Out.', 3),
    ('Out.', 4),
    ('Out.', 5),
    ('Out.', 6),
    ('Out.', 7),
    ('Out.', 8),
    ('Out.', 9),
    ('Out.', 10),
    ('Out.', 11)
   );

DELETE FROM legalhist.whitelist WHERE reporter_found IN ('Out.', 'Outerbridge') AND reporter_standard = 'Out.';
DELETE FROM legalhist.reporters WHERE reporter_standard = 'Out.' AND type = 'nominate';
