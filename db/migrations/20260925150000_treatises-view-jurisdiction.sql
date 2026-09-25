-- migrate:up
SET ROLE = law_admin;

-- One view of the treatises, U.S. and English, with their jurisdiction
-- (issue #142).
--
-- moml.treatises listed every edition, and moml.us_treatises filtered it to
-- the U.S. editions that count as treatises. This replaces both with a single
-- moml.treatises that applies the same filters to every edition and says in a
-- jurisdiction column whether it is 'US' or 'UK'. Every edition carries
-- exactly one of the Gale subjects US and UK, which the migration checks, and
-- that subject is the jurisdiction.
--
-- The filters are those of us_treatises, less its exclusion of the UK: an
-- edition is not a treatise if it has the subject Biography, Collected Essays
-- or Trials, or if its title matches '\Wremarks of\W' or names an address,
-- oration, eulogy, sermon, memorial, in memoriam or obituary (migration
-- 20260906120300 explains that pattern). Measured on 2026-09-25 that leaves
-- 19,612 treatises, 10,328 U.S. and 9,284 English, of the 21,769 editions.
--
-- The view drops the psmid and subjects arrays, which duplicated tables that
-- now exist: an edition's volumes are moml.volumes joined on bibliographicid,
-- and its subjects moml.edition_subjects. An edition's year is its earliest
-- volume's, its title is the first of its volumes' titles in sort order, and
-- vols is its highest volume number (1 for a work not in numbered volumes), as
-- before. The migration asserts that the U.S. rows are exactly the rows of
-- us_treatises.

CREATE TEMPORARY TABLE us_treatises_before ON COMMIT DROP AS
  SELECT bibliographicid, year, title, vols FROM moml.us_treatises;

DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n
  FROM moml.editions e
  WHERE (SELECT count(*) FROM moml.edition_subjects s
         WHERE s.bibliographicid = e.bibliographicid
           AND s.subject IN ('US', 'UK')) <> 1;
  IF n <> 0 THEN
    RAISE EXCEPTION '% editions do not have exactly one of the subjects US and UK', n;
  END IF;
END $$;

DROP VIEW IF EXISTS moml.us_treatises;
DROP VIEW IF EXISTS moml.treatises;

CREATE VIEW moml.treatises AS
 SELECT v.bibliographicid,
    j.subject AS jurisdiction,
    min(v.year) AS year,
    min(v.display_title) AS title,
        CASE
            WHEN max(v.current_volume) = 0 THEN 1
            ELSE max(v.current_volume)
        END AS vols
   FROM moml.volumes v
     JOIN moml.edition_subjects j
       ON j.bibliographicid = v.bibliographicid AND j.subject IN ('US', 'UK')
  WHERE NOT EXISTS (
          SELECT 1 FROM moml.edition_subjects s
           WHERE s.bibliographicid = v.bibliographicid
             AND s.subject IN ('Biography', 'Collected Essays', 'Trials'))
  GROUP BY v.bibliographicid, j.subject
 HAVING NOT min(v.display_title) ~* '\Wremarks of\W'
    AND NOT min(v.display_title) ~* '\y(address|oration|eulogy|sermon|memorial|in memoriam|obituary)\y'
  ORDER BY (min(v.year)), (min(v.display_title));

COMMENT ON VIEW moml.treatises IS
  'Editions that count as legal treatises, with their jurisdiction (US or UK); excludes biographies, collected essays, trials and commemorative pieces. Join moml.volumes on bibliographicid for the volumes.';

GRANT SELECT ON moml.treatises TO law_service;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON moml.treatises TO law_dev;

DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n FROM (
    (SELECT * FROM us_treatises_before
     EXCEPT ALL
     SELECT bibliographicid, year, title, vols FROM moml.treatises WHERE jurisdiction = 'US')
    UNION ALL
    (SELECT bibliographicid, year, title, vols FROM moml.treatises WHERE jurisdiction = 'US'
     EXCEPT ALL
     SELECT * FROM us_treatises_before)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'the U.S. treatises differ from moml.us_treatises: % rows', n;
  END IF;
END $$;

-- migrate:down
SET ROLE = law_admin;

DROP VIEW IF EXISTS moml.treatises;

CREATE VIEW moml.treatises AS
 SELECT v.bibliographicid::character varying(510) AS bibliographicid,
    min(v.year) AS year,
    (array_agg(DISTINCT v.display_title))[1] AS title,
        CASE
            WHEN (max(v.current_volume) = 0) THEN 1
            ELSE max(v.current_volume)
        END AS vols,
    array_agg(DISTINCT (s.subject)::character varying) AS subjects,
    array_agg(DISTINCT (v.psmid)::character varying) AS psmid
   FROM moml.volumes v
     LEFT JOIN moml.edition_subjects s ON s.bibliographicid = v.bibliographicid
  GROUP BY v.bibliographicid
  ORDER BY (min(v.year)), ((array_agg(DISTINCT v.display_title))[1]);

COMMENT ON VIEW moml.treatises IS 'Treatises aggregated from their individual volumes';

CREATE VIEW moml.us_treatises AS
 SELECT bibliographicid::text AS bibliographicid,
    year,
    title,
    vols,
    subjects,
    psmid
   FROM moml.treatises
  WHERE ('UK'::text <> ALL (subjects::text[]))
    AND ('Biography'::text <> ALL (subjects::text[]))
    AND ('Collected Essays'::text <> ALL (subjects::text[]))
    AND ('Trials'::text <> ALL (subjects::text[]))
    AND NOT title ~* '\Wremarks of\W'::text
    AND NOT title ~* '\y(address|oration|eulogy|sermon|memorial|in memoriam|obituary)\y'::text;

GRANT SELECT ON moml.treatises, moml.us_treatises TO law_service;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON moml.treatises, moml.us_treatises TO law_dev;
