-- migrate:up
SET ROLE = law_admin;

-- Track 1 of issue #220: record CourtListener (FreeLaw) reporter spellings as
-- alternate abbreviations so the cite-linker can probe freelaw.cite_to_cap under
-- the spelling FreeLaw actually uses. The crosswalk is keyed on CourtListener's
-- reporter string, which differs from our reporter_standard/reporter_cap for
-- these reporters, so the FreeLaw fallback could never match them before.
-- Approved by @kfunk074 on the issue.
--
-- Only the four pairs that are NOT already in reporters_abbreviations are seeded
-- here. Four other approved pairs (Martin->Mart., Porter->Port., Stewart->Stew.,
-- Story C.C.->Story) already exist in the table and are recovered automatically
-- once the new linker code probes them; they are intentionally not touched.
--
-- reporters_abbreviations has no unique constraint, so each row is inserted only
-- when the exact (reporter_standard, alt_abbr) pair is absent. The EXISTS guard
-- respects the FK to legalhist.reporters; the WHERE clause respects the
-- reporter_standard <> alt_abbr CHECK. None of these four reporters are
-- single_vol, so this does not change the single-volume detector's behavior.
INSERT INTO legalhist.reporters_abbreviations (reporter_standard, alt_abbr)
SELECT v.reporter_standard, v.alt_abbr
FROM (VALUES
    ('Serg. & Rawl.', 'Serg. & Rawle'),
    ('Woods.',        'Woods'),
    ('Walker',        'Walk.'),
    ('Green',         'Greene')
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

-- Remove exactly the four pairs seeded above (and only those — the pre-existing
-- Martin/Porter/Stewart/Story rows are never listed here, so they are untouched).
DELETE FROM legalhist.reporters_abbreviations ra
USING (VALUES
    ('Serg. & Rawl.', 'Serg. & Rawle'),
    ('Woods.',        'Woods'),
    ('Walker',        'Walk.'),
    ('Green',         'Greene')
) AS v(reporter_standard, alt_abbr)
WHERE ra.reporter_standard = v.reporter_standard
  AND ra.alt_abbr = v.alt_abbr;
