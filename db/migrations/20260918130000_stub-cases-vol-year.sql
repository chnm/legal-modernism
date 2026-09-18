-- migrate:up
SET ROLE = law_admin;

-- Rename legalhist.stub_cases.year to vol_year (issue #320).
--
-- The column holds the citation year that keys a stub in a reporter cited by
-- year (issues #312 and #314). In K.B. after 1901 the volume number restarts
-- every year, so "[1901] 2 K.B. 1" and "[1905] 2 K.B. 1" are different
-- cases, and the year is decomposed out of the cite string here just as the
-- volume and page are. It is the year of the volume, not the year the case
-- was decided; that is stub_case_metadata.year_decided. Beside that column
-- the bare name "year" read as the decision year, so the column is renamed
-- to say what it is. The two constraints that carry the column's name are
-- renamed to match; their definitions follow the column automatically.
--
-- RENAME has no IF EXISTS, so each step is guarded by a catalog check to
-- keep the migration idempotent. to_regclass is NULL if the table is
-- missing, and a NULL comparison finds nothing, so nothing then runs.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_attribute
              WHERE attrelid = to_regclass('legalhist.stub_cases')
                AND attname = 'year' AND NOT attisdropped) THEN
    ALTER TABLE legalhist.stub_cases RENAME COLUMN year TO vol_year;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint
              WHERE conrelid = to_regclass('legalhist.stub_cases')
                AND conname = 'stub_cases_reporter_year_volume_page_uq') THEN
    ALTER TABLE legalhist.stub_cases
      RENAME CONSTRAINT stub_cases_reporter_year_volume_page_uq
                     TO stub_cases_reporter_vol_year_volume_page_uq;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint
              WHERE conrelid = to_regclass('legalhist.stub_cases')
                AND conname = 'stub_cases_volume_or_year_check') THEN
    ALTER TABLE legalhist.stub_cases
      RENAME CONSTRAINT stub_cases_volume_or_year_check
                     TO stub_cases_volume_or_vol_year_check;
  END IF;
END
$$;

-- migrate:down
SET ROLE = law_admin;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint
              WHERE conrelid = to_regclass('legalhist.stub_cases')
                AND conname = 'stub_cases_volume_or_vol_year_check') THEN
    ALTER TABLE legalhist.stub_cases
      RENAME CONSTRAINT stub_cases_volume_or_vol_year_check
                     TO stub_cases_volume_or_year_check;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint
              WHERE conrelid = to_regclass('legalhist.stub_cases')
                AND conname = 'stub_cases_reporter_vol_year_volume_page_uq') THEN
    ALTER TABLE legalhist.stub_cases
      RENAME CONSTRAINT stub_cases_reporter_vol_year_volume_page_uq
                     TO stub_cases_reporter_year_volume_page_uq;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_attribute
              WHERE attrelid = to_regclass('legalhist.stub_cases')
                AND attname = 'vol_year' AND NOT attisdropped) THEN
    ALTER TABLE legalhist.stub_cases RENAME COLUMN vol_year TO year;
  END IF;
END
$$;
