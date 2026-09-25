-- migrate:up
SET ROLE = law_admin;

-- Give each volume of a multi-year set its own publication year (issue #142).
--
-- moml.volumes.year is not a Gale field. It was derived when the data was
-- loaded as the first year in pubdate_composed, the date statement of the
-- catalogue record, and that holds for every Gale volume. For a set published
-- over several years the date statement is the edition's span ("1861-1879"),
-- so every volume of the set got its first year. Gale's per-volume
-- pubdate_start (pubdate_pubdatestart, e.g. '18790000') has the year of the
-- volume itself.
--
-- The difference matters to cite-linker's anachronism rule (issue #319), which
-- refuses a link to a case decided after the citing volume's year: a volume
-- printed in 1879 but recorded as 1861 would have its citations to cases of
-- 1862-1879 refused. Measured on 2026-09-25 against the links of the last
-- run, the recorded years would refuse 7,694 links to CAP cases from these
-- volumes, and the volumes' own years 4,569.
--
-- The rule: for a Gale volume whose pubdate_start year is later than its
-- recorded year and no later than the last year in its date statement, the
-- volume's year becomes the pubdate_start year. That is 130 volumes in 67
-- editions. It leaves alone the volumes whose pubdate_start is earlier than
-- the year, which is a copyright date ("1873, c1850"), and two volumes whose
-- pubdate_start falls outside their date statement (19001448500, "1874" with
-- an imprint of 1875; 19000078502, "[1881-[188-?]").
--
-- moml.treatises gives an edition its earliest volume's year, which moves for
-- two editions whose every volume in MOML is later than the set's first year:
-- ocm16990544 (1861-1879) from 1861 to 1862, and ocm29644128 (1845-1846)
-- from 1845 to 1846.
--
-- The old years are recorded in moml_archive.volume_year_changes, and the
-- original book_info.year is also in moml_archive.book_info.

CREATE TABLE IF NOT EXISTS moml_archive.volume_year_changes (
    psmid    text PRIMARY KEY,
    old_year integer NOT NULL,
    new_year integer NOT NULL CHECK (new_year > old_year)
);

COMMENT ON TABLE moml_archive.volume_year_changes IS
  'Volumes of multi-year sets whose year was changed from the set''s first year to their own pubdate_start year (issue #142)';

GRANT SELECT ON moml_archive.volume_year_changes TO law_service, law_dev;

INSERT INTO moml_archive.volume_year_changes (psmid, old_year, new_year)
SELECT psmid, year, start_year
FROM (
  SELECT psmid, year,
         left(pubdate_start, 4)::integer AS start_year,
         (SELECT max(m[1]::integer)
          FROM regexp_matches(pubdate_composed, '(1[4-9][0-9]{2})', 'g') AS m) AS last_year
  FROM moml.volumes
  WHERE gale_id IS NOT NULL
    AND pubdate_start ~ '^[0-9]{8}$'
) v
WHERE start_year > year
  AND start_year <= last_year;

DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n FROM moml_archive.volume_year_changes;
  IF n <> 130 THEN
    RAISE EXCEPTION 'expected 130 volumes to take their own year, found %', n;
  END IF;

  UPDATE moml.volumes v
  SET year = c.new_year
  FROM moml_archive.volume_year_changes c
  WHERE v.psmid = c.psmid
    AND v.year = c.old_year;
  GET DIAGNOSTICS n = ROW_COUNT;
  IF n <> 130 THEN
    RAISE EXCEPTION 'expected to change 130 years, changed %', n;
  END IF;
END $$;

-- migrate:down
SET ROLE = law_admin;

DO $$
DECLARE
  n bigint;
BEGIN
  UPDATE moml.volumes v
  SET year = c.old_year
  FROM moml_archive.volume_year_changes c
  WHERE v.psmid = c.psmid
    AND v.year = c.new_year;
  GET DIAGNOSTICS n = ROW_COUNT;
  IF n <> (SELECT count(*) FROM moml_archive.volume_year_changes) THEN
    RAISE EXCEPTION 'restored % of % years; a volume changed since the migration',
      n, (SELECT count(*) FROM moml_archive.volume_year_changes);
  END IF;
END $$;

DROP TABLE IF EXISTS moml_archive.volume_year_changes;
