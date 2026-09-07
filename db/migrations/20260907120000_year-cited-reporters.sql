-- migrate:up
SET ROLE = law_admin;

-- Record the year of a citation to a reporter that is cited by year, and flag
-- those reporters so the detector looks for it (issue #312, the question
-- raised on #248).
--
-- The Law Reports after 1890 and the Irish Reports after 1893 restart their
-- volume numbers every year: "[1905] 2 K.B. 1" is page 1 of the second King's
-- Bench volume for 1905. The detector captured "2 K. B. 1" without the year,
-- so every year's case at that volume and page collapsed into one string --
-- "2 K.B. 1" has 795 citations from 368 treatises and is about fifty cases --
-- and the stub registry (#248) would have made that string a single case.
--
-- citations_unlinked.year is set by the year detector, which is built for
-- every whitelisted spelling of a flagged reporter, and is NULL for every
-- other detection. It joins the unique index so that one page can cite
-- "[1905] 2 K.B. 1" and "[1906] 2 K.B. 1" as two rows. Rebuilding the index
-- over the 56M rows takes minutes; after the TRUNCATE that the re-detection
-- needs anyway it is instant, so truncate first.
ALTER TABLE moml_citations.citations_unlinked
  ADD COLUMN IF NOT EXISTS year integer;

DROP INDEX IF EXISTS moml_citations.citations_unlinked_uq;
CREATE UNIQUE INDEX IF NOT EXISTS citations_unlinked_uq
    ON moml_citations.citations_unlinked
    USING btree (moml_treatise, moml_page, COALESCE(volume, '-1'::integer), reporter_abbr, page, COALESCE(year, '-1'::integer));

-- cited_by_year marks a reporter whose citations carry a year the case cannot
-- be identified without. cite-detector-moml builds a year detector for every
-- whitelisted spelling of such a reporter, and cite-linker and
-- db/stub_cases.sql put the year into the cite string, "[1905] 2 K.B. 1",
-- whenever a citation carries one. NULL means no, like single_vol.
ALTER TABLE legalhist.reporters
  ADD COLUMN IF NOT EXISTS cited_by_year boolean;

-- K.B. (1901-1952) is cited by year throughout. L.R.Ir. is a mixed row: the
-- volume-cited 4th series (1878-1893, "L. R. Ir.") and the year-cited Irish
-- Reports (1894-, "I. R.") share it, so the flag captures the year for the
-- latter and changes nothing for a citation that carries none. The other
-- year-cited series -- Q.B., Ch., A.C. and P. after 1890, S.L.T. after 1908 --
-- need reporter rows or whitelist moves before they can be flagged; the issue
-- lists what each needs.
UPDATE legalhist.reporters
   SET cited_by_year = true
 WHERE reporter_standard IN ('K.B.', 'L.R.Ir.')
   AND cited_by_year IS DISTINCT FROM true;

-- The stub registry keys on the same cite string, so a stub carries the year
-- too, and two stubs may share a reporter, volume and page across years.
ALTER TABLE legalhist.stub_cases
  ADD COLUMN IF NOT EXISTS year integer;
ALTER TABLE legalhist.stub_cases
  DROP CONSTRAINT IF EXISTS stub_cases_reporter_volume_page_uq;
ALTER TABLE legalhist.stub_cases
  DROP CONSTRAINT IF EXISTS stub_cases_reporter_year_volume_page_uq;
ALTER TABLE legalhist.stub_cases
  ADD CONSTRAINT stub_cases_reporter_year_volume_page_uq
  UNIQUE NULLS NOT DISTINCT (reporter_standard, year, volume, page);

-- migrate:down
SET ROLE = law_admin;

-- A stub keyed on a year cannot be expressed without the column.
DELETE FROM legalhist.stub_cases WHERE year IS NOT NULL;
ALTER TABLE legalhist.stub_cases
  DROP CONSTRAINT IF EXISTS stub_cases_reporter_year_volume_page_uq;
ALTER TABLE legalhist.stub_cases
  DROP COLUMN IF EXISTS year;
ALTER TABLE legalhist.stub_cases
  DROP CONSTRAINT IF EXISTS stub_cases_reporter_volume_page_uq;
ALTER TABLE legalhist.stub_cases
  ADD CONSTRAINT stub_cases_reporter_volume_page_uq UNIQUE (reporter_standard, volume, page);

ALTER TABLE legalhist.reporters
  DROP COLUMN IF EXISTS cited_by_year;

-- Rows that differ only by year collide under the narrower index. Keep one per
-- old key -- the year-less row where there is one, else the earliest year --
-- and drop the rest together with their links, which is what a detection run
-- without the year detector would have produced.
WITH ranked AS (
  SELECT id,
         row_number() OVER (
           PARTITION BY moml_treatise, moml_page, COALESCE(volume, -1), reporter_abbr, page
           ORDER BY year NULLS FIRST, id
         ) AS rk
  FROM moml_citations.citations_unlinked
),
doomed AS (SELECT id FROM ranked WHERE rk > 1),
links AS (
  DELETE FROM moml_citations.citation_links cl
   USING doomed d WHERE cl.citation_id = d.id
)
DELETE FROM moml_citations.citations_unlinked cu
 USING doomed d WHERE cu.id = d.id;

DROP INDEX IF EXISTS moml_citations.citations_unlinked_uq;
ALTER TABLE moml_citations.citations_unlinked
  DROP COLUMN IF EXISTS year;
CREATE UNIQUE INDEX IF NOT EXISTS citations_unlinked_uq
    ON moml_citations.citations_unlinked
    USING btree (moml_treatise, moml_page, COALESCE(volume, '-1'::integer), reporter_abbr, page);
