-- migrate:up
SET ROLE = law_admin;

-- Findings from the #246/#247/#275 review that PR #276 first left for a
-- decision, now acted on (part of #165). Each block is independent and
-- measured against the live database on 2026-09-05.

-- 1. "Gal." is Gallison, not California. 853 rows, 443 of them at volumes
--    1-2 (Gallison's Circuit Court Reports have two volumes); the OCR-variant
--    harness read it as "Cal." by a G/C confusion. Gallison is whitelisted
--    as "Gall." and CAP holds 161 cites in that form.
INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk)
VALUES ('Gal.', 'Gall.', false)
ON CONFLICT (reporter_found) DO NOTHING;

-- 2. The Central Reporter and the Western Reporter (both 1885-1888) have no
--    reporter row, so the whitelist sent "Cent. Rep." and "Cent." to Jenkins'
--    Exchequer Reports and "West. Rep." to West's House of Lords Reports,
--    one-volume English reporters, where they can never link and where the
--    single-volume probe could link a bare page to the wrong case. The rows
--    are cited at volumes 1-13 (Cent.: 2,213 + 965 rows) and 1-14 (West.
--    Rep.: 2,402 rows), the runs of the two regional reporters. Neither is
--    in CAP, so after this they are honest us_reporter_absent misses and
--    candidates for the stub table (#248) or an import (#252).
INSERT INTO legalhist.reporters
    (reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, single_vol, type)
SELECT v.reporter_standard, v.reporter_title, 'state', 'us:mc', 1885, 1888, false, 'official'
FROM (VALUES
    ('Cent. Rep.', 'Central Reporter'),
    ('West. Rep.', 'Western Reporter')
) AS v(reporter_standard, reporter_title)
WHERE NOT EXISTS (
    SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = v.reporter_standard
);

UPDATE legalhist.whitelist SET reporter_standard = 'Cent. Rep.'
 WHERE reporter_found IN ('Cent. Rep.', 'Cent.') AND reporter_standard = 'Jenk';
UPDATE legalhist.whitelist SET reporter_standard = 'West. Rep.'
 WHERE reporter_found = 'West. Rep.' AND reporter_standard = 'West';

-- 3. Wright's Pennsylvania Reports (Pa. 37-50). The whitelist sends every
--    "Wright" spelling to Wright's Ohio Reports, which has one volume, but
--    the 6,340 rows are cited at volumes 1-14 spread evenly, which is the
--    Pennsylvania run: 5,588 of them are us_volume_absent. Volume 1 is the
--    only ambiguous one, and 389 rows at volume 1 do link to the Ohio
--    volume, so volumes 2-14 are mapped to Pa. 38-50 through diffvols on the
--    existing reporter row and volume 1 is left alone. A volume-1 miss will
--    read as us_diffvols_missing from here on, which is the truth: no row
--    covers it, and none can until the two volume-1 reporters are told apart.
--    CAP holds 1,177 first-page cites in Pa. 38-50. Jones (Pa. 11-12) gets
--    the same treatment only if its two Pennsylvania volumes can be told
--    from the eight of Jones' North Carolina Law Reports, which the cited
--    volumes (1-8, spread evenly) cannot do, so it is left as it is.
INSERT INTO legalhist.reporters_diffvols (reporter_title, vol, cap_vol, cap_reporter, reporter_standard)
SELECT v.reporter_title, v.vol, v.cap_vol, 'Pa.', 'Wright'
FROM (VALUES
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 2, 38),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 3, 39),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 4, 40),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 5, 41),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 6, 42),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 7, 43),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 8, 44),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 9, 45),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 10, 46),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 11, 47),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 12, 48),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 13, 49),
    ('Wright''s Pennsylvania State Reports (volume 1 is left to Wright''s Ohio Reports)', 14, 50)
) AS v(reporter_title, vol, cap_vol)
WHERE EXISTS (SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = 'Wright')
  AND NOT EXISTS (
    SELECT 1 FROM legalhist.reporters_diffvols d
    WHERE d.reporter_standard = 'Wright' AND d.vol = v.vol
  );

-- 4. Two single-volume flags whose bare detectors produce surnames. The
--    single-volume detector stems its abbreviation (\w*), so the detector
--    for "Smith" (Smith's New Hampshire Reports) records "Smith 45" wherever
--    a Smith is followed by a number: 337,385 bare "Smith" rows today, which
--    the whitelist then sends to Smith Pa. as volume-less citations. The
--    detector for "Wil." and its alternate "Will." does the same for Wilson
--    and Williams, and every spelling that reached Williams' Massachusetts
--    Reports is now a statute (20260905120000). Neither reporter links a
--    single bare citation through these detectors, so the flag comes off and
--    the next detection run no longer builds them. Their alternates stay and
--    are still probed as alternate spellings on the linking route.
UPDATE legalhist.reporters
   SET single_vol = false
 WHERE reporter_standard IN ('Wil.', 'Smith')
   AND single_vol = true;

-- migrate:down
SET ROLE = law_admin;

UPDATE legalhist.reporters
   SET single_vol = true
 WHERE reporter_standard IN ('Wil.', 'Smith')
   AND single_vol = false;

DELETE FROM legalhist.reporters_diffvols
 WHERE reporter_standard = 'Wright' AND cap_reporter = 'Pa.' AND vol BETWEEN 2 AND 14;

UPDATE legalhist.whitelist SET reporter_standard = 'West'
 WHERE reporter_found = 'West. Rep.' AND reporter_standard = 'West. Rep.';
UPDATE legalhist.whitelist SET reporter_standard = 'Jenk'
 WHERE reporter_found IN ('Cent. Rep.', 'Cent.') AND reporter_standard = 'Cent. Rep.';
DELETE FROM legalhist.reporters
 WHERE reporter_standard IN ('Cent. Rep.', 'West. Rep.') AND reporter_title IN ('Central Reporter', 'Western Reporter');

DELETE FROM legalhist.whitelist WHERE reporter_found = 'Gal.' AND reporter_standard = 'Gall.';
