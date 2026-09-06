-- migrate:up
SET ROLE = law_admin;

-- Re-key five English Reports reporters on the spelling the linker probes
-- (issue #308, found while sizing the stub table for #248).
--
-- On the UK route the linker builds one probe per citation, "{volume}
-- {reporter_standard} {page}", and looks it up among the nominate parallel
-- cites of english_reports.cases, so a UK reporter links only when its
-- reporter_standard is the spelling the English Reports use ('Ad & E',
-- 'Burr', 'Ves Sen'). These five were keyed on a conventional abbreviation
-- instead, and every one of their 110,488 citations was a uk_reporter_absent
-- miss -- the largest "uncovered" reporters in the pool the stub table was
-- built for. Measured 2026-09-06 with the English Reports spelling
-- substituted for the reporter_standard:
--
--   reporter       citations   exact first-page hit      inside a covered volume   volume not in ER
--   Stark. N. P.      69,014    54,182 (+946 ambiguous)          13,280                  606
--   Russ. & M.        16,654    12,047 (+91)                      4,419                   97
--   Anstr.            13,608     5,380 (+741)                     7,290                  197
--   Strange            7,609     1,580 (+4,828)                   1,142                   59
--   H. & C.            3,603     2,390                              743                  470
--
-- "Ambiguous" hits are cite strings several English Reports cases share,
-- which the linker reports as uk_page_ambiguous rather than resolving; most
-- of Strange's are of that kind, its pages carrying many short cases. The
-- interior citations are the pool page-range matching draws on.
--
-- Each reporter's whitelist spellings and alternate abbreviations move onto
-- a row keyed on the English Reports spelling: created here with the old
-- row's metadata for four of them, while Strange's row 'Str' already exists
-- (behind 'Str.' and 'Stra.') and already links. The old row is dropped once
-- nothing references it. Every statement is guarded, so re-applying this, or
-- applying it after a hand edit that did part of the work, changes nothing
-- further. The data step is a truncate-and-relink; where legalhist.stub_cases
-- exists (#248), its rows for these reporters are removed here, since a
-- stub exists only for a reporter no source holds.

INSERT INTO legalhist.reporters
    (reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, single_vol, type, reporter_cap)
SELECT m.new, r.reporter_title, r.level, r.jurisdiction, r.year_start, r.year_end, r.single_vol, r.type, r.reporter_cap
FROM (VALUES
    ('Stark. N. P.', 'Stark'),
    ('Russ. & M.',   'Russ & My'),
    ('Anstr.',       'Anst'),
    ('H. & C.',      'H & C')
) AS m(old, new)
JOIN legalhist.reporters r ON r.reporter_standard = m.old
WHERE NOT EXISTS (SELECT 1 FROM legalhist.reporters n WHERE n.reporter_standard = m.new);

UPDATE legalhist.whitelist w
   SET reporter_standard = m.new
  FROM (VALUES
    ('Stark. N. P.', 'Stark'),
    ('Russ. & M.',   'Russ & My'),
    ('Anstr.',       'Anst'),
    ('H. & C.',      'H & C'),
    ('Strange',      'Str')
) AS m(old, new)
 WHERE w.reporter_standard = m.old
   AND EXISTS (SELECT 1 FROM legalhist.reporters n WHERE n.reporter_standard = m.new);

-- The alternates are only probed on the US route, so this is tidiness: the
-- rows follow their reporter rather than being orphaned with the old key.
INSERT INTO legalhist.reporters_abbreviations (reporter_standard, alt_abbr)
SELECT m.new, a.alt_abbr
FROM (VALUES
    ('Stark. N. P.', 'Stark'),
    ('Russ. & M.',   'Russ & My'),
    ('Anstr.',       'Anst'),
    ('H. & C.',      'H & C'),
    ('Strange',      'Str')
) AS m(old, new)
JOIN legalhist.reporters_abbreviations a ON a.reporter_standard = m.old
WHERE a.alt_abbr <> m.new
  AND EXISTS (SELECT 1 FROM legalhist.reporters n WHERE n.reporter_standard = m.new)
  AND NOT EXISTS (
    SELECT 1 FROM legalhist.reporters_abbreviations b
    WHERE b.reporter_standard = m.new AND b.alt_abbr = a.alt_abbr
  );

DELETE FROM legalhist.reporters_abbreviations a
 WHERE a.reporter_standard IN ('Stark. N. P.', 'Russ. & M.', 'Anstr.', 'H. & C.', 'Strange')
   AND NOT EXISTS (SELECT 1 FROM legalhist.whitelist w WHERE w.reporter_standard = a.reporter_standard);

DO $$
BEGIN
  IF to_regclass('legalhist.stub_cases') IS NOT NULL THEN
    DELETE FROM legalhist.stub_cases
     WHERE reporter_standard IN ('Stark. N. P.', 'Russ. & M.', 'Anstr.', 'H. & C.', 'Strange');
  END IF;
END $$;

DELETE FROM legalhist.reporters r
 WHERE r.reporter_standard IN ('Stark. N. P.', 'Russ. & M.', 'Anstr.', 'H. & C.', 'Strange')
   AND NOT EXISTS (SELECT 1 FROM legalhist.whitelist w WHERE w.reporter_standard = r.reporter_standard)
   AND NOT EXISTS (SELECT 1 FROM legalhist.reporters_abbreviations a WHERE a.reporter_standard = r.reporter_standard)
   AND NOT EXISTS (SELECT 1 FROM legalhist.reporters_diffvols d WHERE d.reporter_standard = r.reporter_standard);

-- migrate:down
SET ROLE = law_admin;

-- Restore the old rows with the metadata they carried on 2026-09-06, move the
-- spellings and alternates back by name, and drop the rows this migration
-- created. 'Str' predates it and stays.
INSERT INTO legalhist.reporters
    (reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, single_vol, type)
SELECT v.*
FROM (VALUES
    ('Stark. N. P.', 'Starkie''s Nisi Prius Reports',            'national', 'uk:np', 1814, 1822, false, 'nominate'),
    ('Russ. & M.',   'Russell and Mylne''s Chancery Reports',    'national', 'uk:ch', 1829, 1831, false, 'nominate'),
    ('Anstr.',       'Anstruther''s Exchequer Reports',          'national', 'uk:ex', 1792, 1797, NULL,  'nominate'),
    ('H. & C.',      'Hurlstone & Coltman’s Exchequer Reports',  'national', 'uk:ex', 1862, 1866, NULL,  'nominate'),
    ('Strange',      'Strange''s King''s Bench Reports',         'national', 'uk:kb', 1716, 1749, false, 'nominate')
) AS v(reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, single_vol, type)
WHERE NOT EXISTS (SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = v.reporter_standard);

UPDATE legalhist.whitelist SET reporter_standard = 'Stark. N. P.'
 WHERE reporter_standard = 'Stark'
   AND reporter_found IN ('Star.', 'Stark,', 'Stark.', 'Starkie', 'Starkie,', 'Stark. N. P.', 'Stark. N. P. C.', 'Stark. R.', 'Stark. Rep.');
UPDATE legalhist.whitelist SET reporter_standard = 'Russ. & M.'
 WHERE reporter_standard = 'Russ & My'
   AND reporter_found IN ('R. & M.', 'R. & My.', 'Rnss. & M.', 'Russ. & M.', 'Russ. & My.', 'Russ. & Myl.', 'Russ. & Mylne,');
UPDATE legalhist.whitelist SET reporter_standard = 'Anstr.'
 WHERE reporter_standard = 'Anst'
   AND reporter_found IN ('Anftr.', 'Anslr.', 'Anst.', 'Anstr.', 'Austr.');
UPDATE legalhist.whitelist SET reporter_standard = 'H. & C.'
 WHERE reporter_standard = 'H & C'
   AND reporter_found IN ('Hurl. & C.', 'II. & C.');
UPDATE legalhist.whitelist SET reporter_standard = 'Strange'
 WHERE reporter_standard = 'Str'
   AND reporter_found IN ('Strangc', 'Strange', 'Strange,');

INSERT INTO legalhist.reporters_abbreviations (reporter_standard, alt_abbr)
SELECT v.reporter_standard, v.alt_abbr
FROM (VALUES
    ('Stark. N. P.', 'Star.'), ('Stark. N. P.', 'Stark.'), ('Stark. N. P.', 'Starkie'), ('Stark. N. P.', 'Starkie''s'),
    ('Russ. & M.', 'R.& My.'), ('Russ. & M.', 'Russ.& My.'),
    ('Anstr.', 'ANST'), ('Anstr.', 'Anst.'),
    ('H. & C.', 'Hurl.& C.'), ('H. & C.', 'Hurl.& Colt.'), ('H. & C.', 'Hurl.Colt.'), ('H. & C.', 'Hurlst.& C.'),
    ('Strange', 'Str.'), ('Strange', 'Stran.')
) AS v(reporter_standard, alt_abbr)
WHERE NOT EXISTS (
    SELECT 1 FROM legalhist.reporters_abbreviations a
    WHERE a.reporter_standard = v.reporter_standard AND a.alt_abbr = v.alt_abbr
);
DELETE FROM legalhist.reporters_abbreviations
 WHERE (reporter_standard, alt_abbr) IN (
    ('Stark', 'Star.'), ('Stark', 'Stark.'), ('Stark', 'Starkie'), ('Stark', 'Starkie''s'),
    ('Russ & My', 'R.& My.'), ('Russ & My', 'Russ.& My.'),
    ('Anst', 'ANST'), ('Anst', 'Anst.'),
    ('H & C', 'Hurl.& C.'), ('H & C', 'Hurl.& Colt.'), ('H & C', 'Hurl.Colt.'), ('H & C', 'Hurlst.& C.'),
    ('Str', 'Str.'), ('Str', 'Stran.')
 );

DELETE FROM legalhist.reporters r
 WHERE r.reporter_standard IN ('Stark', 'Russ & My', 'Anst', 'H & C')
   AND NOT EXISTS (SELECT 1 FROM legalhist.whitelist w WHERE w.reporter_standard = r.reporter_standard)
   AND NOT EXISTS (SELECT 1 FROM legalhist.reporters_abbreviations a WHERE a.reporter_standard = r.reporter_standard)
   AND NOT EXISTS (SELECT 1 FROM legalhist.reporters_diffvols d WHERE d.reporter_standard = r.reporter_standard);
