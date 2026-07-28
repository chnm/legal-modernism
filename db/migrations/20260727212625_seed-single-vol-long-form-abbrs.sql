-- migrate:up
SET ROLE = law_admin;

-- Issue #226 (with #218): the single-volume detector no longer stems, so an
-- abbreviation must now be followed directly by the page number. Stemming was
-- how "Al" came to match "Alienation" and "Ala.", but it was also the only way
-- the long forms below were ever detected: each is a longer spelling of a
-- single-volume reporter that is NOT registered in reporters_abbreviations, so
-- without stemming no detector matches it at all.
--
-- Seeding them gives each long form its own detector, which matches the exact
-- spelling instead of anything sharing a prefix. legalhist.whitelist already
-- maps every one of these strings to the reporter_standard shown, so nothing
-- downstream of detection needs to change.
--
-- The list was derived from the 7.7M existing volume-IS-NULL rows in
-- moml_citations.citations_unlinked: forms that (a) stop matching once the stem
-- is removed, (b) resolve through the whitelist to a non-junk reporter, and
-- (c) whose reporter has single_vol = true. Together they account for 192,283
-- detected citations. Row counts at time of writing are in the trailing comment
-- on each pair.
--
-- reporters_abbreviations has no unique constraint, so each row is inserted only
-- when the exact (reporter_standard, alt_abbr) pair is absent. The EXISTS guard
-- respects the FK to legalhist.reporters; the WHERE clause respects the
-- reporter_standard <> alt_abbr CHECK.
INSERT INTO legalhist.reporters_abbreviations (reporter_standard, alt_abbr)
SELECT v.reporter_standard, v.alt_abbr
FROM (VALUES
    ('Dav',        'Davis'),          -- 88174
    ('Edw',        'Edwards'),        -- 38480
    ('Dan',        'Daniel'),         -- 12938
    ('Vern',       'Vernon'),         -- 10399
    ('Dud.',       'Dudley'),         --  7730
    ('Edw',        'Edward'),         --  6776
    ('Comst.',     'Comstock'),       --  6554
    ('Andr',       'Andrew'),         --  4615
    ('Swab',       'Swabey'),         --  2213
    ('Wilm',       'Wilmot'),         --  2055
    ('Dan',        'Daniell'),        --  1739
    ('Carth',      'Carthew'),        --  1706
    ('Lush',       'Lushington'),     --  1682
    ('Sav',        'Saville'),        --  1186
    ('Toth',       'Tothill'),        --   914
    ('Godb',       'Godbolt'),        --   715
    ('Wight',      'Wightwick'),      --   674
    ('Busb.',      'Busbee'),         --   647
    ('Taml',       'Tamlyn'),         --   639
    ('Tay.',       'Tayl.'),          --   511
    ('Het',        'Hetley'),         --   447
    ('Keil',       'Keilway'),        --   387
    ('McMul.',     'McMull.'),        --   309
    ('Peck',       'Peckw.'),         --   227
    ('Calth',      'Calthrop'),       --   172
    ('Dears',      'Dearsly'),        --   129
    ('Comb',       'Comberbach'),     --   125
    ('M''Cle',     'M''Cleland'),     --    51
    ('Cas T H',    'Cas. T. Hard.'),  --    41
    ('Gould',      'Gouldsborough'),  --    31
    ('Hay & M',    'Hay & Marriott'), --    16
    ('Hoff. Ch.',  'Hoff. Chan.')     --     1
) AS v(reporter_standard, alt_abbr)
WHERE v.reporter_standard <> v.alt_abbr
  AND EXISTS (
    SELECT 1 FROM legalhist.reporters r
    WHERE r.reporter_standard = v.reporter_standard
  )
  AND NOT EXISTS (
    SELECT 1 FROM legalhist.reporters_abbreviations ra
    WHERE ra.reporter_standard = v.reporter_standard
      AND ra.alt_abbr = v.alt_abbr
  );

-- migrate:down
SET ROLE = law_admin;

-- Remove exactly the pairs seeded above.
DELETE FROM legalhist.reporters_abbreviations ra
USING (VALUES
    ('Dav',        'Davis'),
    ('Edw',        'Edwards'),
    ('Dan',        'Daniel'),
    ('Vern',       'Vernon'),
    ('Dud.',       'Dudley'),
    ('Edw',        'Edward'),
    ('Comst.',     'Comstock'),
    ('Andr',       'Andrew'),
    ('Swab',       'Swabey'),
    ('Wilm',       'Wilmot'),
    ('Dan',        'Daniell'),
    ('Carth',      'Carthew'),
    ('Lush',       'Lushington'),
    ('Sav',        'Saville'),
    ('Toth',       'Tothill'),
    ('Godb',       'Godbolt'),
    ('Wight',      'Wightwick'),
    ('Busb.',      'Busbee'),
    ('Taml',       'Tamlyn'),
    ('Tay.',       'Tayl.'),
    ('Het',        'Hetley'),
    ('Keil',       'Keilway'),
    ('McMul.',     'McMull.'),
    ('Peck',       'Peckw.'),
    ('Calth',      'Calthrop'),
    ('Dears',      'Dearsly'),
    ('Comb',       'Comberbach'),
    ('M''Cle',     'M''Cleland'),
    ('Cas T H',    'Cas. T. Hard.'),
    ('Gould',      'Gouldsborough'),
    ('Hay & M',    'Hay & Marriott'),
    ('Hoff. Ch.',  'Hoff. Chan.')
) AS v(reporter_standard, alt_abbr)
WHERE ra.reporter_standard = v.reporter_standard
  AND ra.alt_abbr = v.alt_abbr;
