-- migrate:up
SET ROLE = law_admin;

-- Copies of an edition (issue #402).
--
-- Until now a row of moml.editions was an edition, and the database held one
-- scan of each: the second scans Gale made were deleted (moml.duplicate_copies).
-- The HathiTrust volumes of Stephen on Pleading are second scans we keep, each
-- with its own row of moml.editions, so an edition can now have several copies:
-- the same printing scanned by Gale and by HathiTrust, or another printing of
-- the same edition (Kirkpatrick's 1857 and Kay's 1859 printings of the 8th
-- American edition; the 1875, 1882, 1897 and 1900 printings of Tyler's 3rd).
--
-- One copy of an edition is the official one, and stands for the edition in
-- display and in statistics. editions.copy_of is NULL on the official copy and
-- names it on every other copy, so "the editions" are the rows where copy_of
-- IS NULL, and an edition's copies are the rows whose copy_of is its id.
-- moml.treatises now holds official copies only, which is how everything
-- that reads it (edition_citation_counts, and through it the works, cases and
-- reporters of chambers) counts an edition once. Every copy is still scanned
-- by the detector and linked, and edition_case_citations and
-- edition_reporter_citations still have a row for each copy, so two copies of
-- one edition can be compared.
--
-- A HathiTrust copy is preferred to Gale's, because it is public; among
-- several HathiTrust printings the earliest is official.
-- moml.prefer_hathitrust_copies() applies that rule from
-- moml.hathitrust_volumes, and scripts/hathitrust-import calls it after an
-- import. The official copy's year is the edition's year in moml.treatises.

ALTER TABLE moml.editions
  ADD COLUMN IF NOT EXISTS copy_of text REFERENCES moml.editions (bibliographicid);

ALTER TABLE moml.editions DROP CONSTRAINT IF EXISTS editions_copy_of_check;
ALTER TABLE moml.editions
  ADD CONSTRAINT editions_copy_of_check CHECK (copy_of <> bibliographicid);

CREATE INDEX IF NOT EXISTS editions_copy_of_idx
  ON moml.editions (copy_of) WHERE copy_of IS NOT NULL;

COMMENT ON COLUMN moml.editions.copy_of IS
  'NULL when this row is the official copy of its edition, the one shown and counted; otherwise the bibliographicid of the official copy, of which this row is another scan or another printing (issue #402). The official copy is never itself a copy, and is in the same work.';

-- An official copy must not be a copy itself, or its own copies would hang
-- from a row no reader looks at, and must be in the same work. Checked at
-- commit, so a statement may re-point several rows of one edition at once.
CREATE OR REPLACE FUNCTION moml.check_edition_copy() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  official moml.editions%ROWTYPE;
  cur moml.editions%ROWTYPE;
BEGIN
  SELECT * INTO cur FROM moml.editions WHERE bibliographicid = NEW.bibliographicid;
  IF NOT FOUND THEN
    RETURN NULL;
  END IF;

  IF cur.copy_of IS NOT NULL THEN
    SELECT * INTO official FROM moml.editions WHERE bibliographicid = cur.copy_of;
    IF official.copy_of IS NOT NULL THEN
      RAISE EXCEPTION 'edition % is a copy of %, which is itself a copy of %',
        cur.bibliographicid, cur.copy_of, official.copy_of;
    END IF;
    IF official.work_id <> cur.work_id THEN
      RAISE EXCEPTION 'edition % (work %) is a copy of % (work %); copies share a work',
        cur.bibliographicid, cur.work_id, cur.copy_of, official.work_id;
    END IF;
    IF EXISTS (SELECT 1 FROM moml.editions WHERE copy_of = cur.bibliographicid) THEN
      RAISE EXCEPTION 'edition % is a copy of % and has copies of its own',
        cur.bibliographicid, cur.copy_of;
    END IF;
  ELSIF EXISTS (SELECT 1 FROM moml.editions
                WHERE copy_of = cur.bibliographicid AND work_id <> cur.work_id) THEN
    RAISE EXCEPTION 'edition % is in work %, and a copy of it is in another work',
      cur.bibliographicid, cur.work_id;
  END IF;
  RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS editions_copy_check ON moml.editions;
CREATE CONSTRAINT TRIGGER editions_copy_check
  AFTER INSERT OR UPDATE ON moml.editions
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION moml.check_edition_copy();

-- The rule for the HathiTrust volumes. A HathiTrust volume names the Gale
-- volume of the same edition (hathitrust_volumes.moml_psmid), whatever the
-- printing; the Gale edition and the HathiTrust volumes that name it are the
-- copies of one edition. The earliest HathiTrust copy is official. Only a Gale
-- edition of a single volume is paired, since a HathiTrust volume is its own
-- edition. Returns the number of rows whose copy_of changed.
CREATE OR REPLACE FUNCTION moml.prefer_hathitrust_copies() RETURNS integer
LANGUAGE plpgsql AS $$
DECLARE
  n integer;
BEGIN
  WITH member AS (
    -- (the Gale edition that names the group, a copy in it, the copy's rank)
    SELECT mv.bibliographicid AS grp, hv.bibliographicid AS copy,
           row_number() OVER (PARTITION BY mv.bibliographicid
                              ORDER BY hv.year NULLS LAST, hv.psmid) AS rank
    FROM moml.hathitrust_volumes h
    JOIN moml.volumes hv ON hv.psmid = h.psmid
    JOIN moml.volumes mv ON mv.psmid = h.moml_psmid
    WHERE (SELECT count(*) FROM moml.volumes x WHERE x.bibliographicid = mv.bibliographicid) = 1
  ), official AS (
    SELECT grp, copy AS official FROM member WHERE rank = 1
  ), want AS (
    SELECT m.copy AS bibliographicid, NULLIF(o.official, m.copy) AS copy_of
    FROM member m JOIN official o USING (grp)
    UNION ALL
    SELECT o.grp, o.official FROM official o
  )
  UPDATE moml.editions e
     SET copy_of = w.copy_of
    FROM want w
   WHERE w.bibliographicid = e.bibliographicid
     AND e.copy_of IS DISTINCT FROM w.copy_of;
  GET DIAGNOSTICS n = ROW_COUNT;
  RETURN n;
END $$;

COMMENT ON FUNCTION moml.prefer_hathitrust_copies() IS
  'Makes the earliest HathiTrust copy of an edition its official copy and points the Gale edition and the other HathiTrust copies at it (editions.copy_of), from moml.hathitrust_volumes (issue #402). Safe to repeat.';

SELECT moml.prefer_hathitrust_copies();

-- One row per row of moml.editions, saying which edition it is a copy of and
-- where the scan came from.
CREATE OR REPLACE VIEW moml.edition_copies AS
SELECT e.bibliographicid,
       COALESCE(e.copy_of, e.bibliographicid) AS edition,
       e.copy_of IS NULL AS official,
       CASE
         WHEN EXISTS (SELECT 1 FROM moml.volumes v
                      JOIN moml.hathitrust_volumes h ON h.psmid = v.psmid
                      WHERE v.bibliographicid = e.bibliographicid) THEN 'HathiTrust'
         WHEN EXISTS (SELECT 1 FROM moml.volumes v
                      WHERE v.bibliographicid = e.bibliographicid AND v.gale_id IS NOT NULL) THEN 'MOML'
         ELSE 'side corpus'
       END AS source,
       e.work_id
FROM moml.editions e;

COMMENT ON VIEW moml.edition_copies IS
  'Every row of moml.editions as a copy of an edition (issue #402): edition is the official copy''s bibliographicid (its own when official), and source is where the scan came from, MOML (Gale), HathiTrust or the side corpus. An edition''s copies are the rows sharing its edition.';

GRANT SELECT ON moml.edition_copies TO law_service;

-- The treatise view, as in 20260925160000, with the copies that are not
-- official left out.
CREATE OR REPLACE VIEW moml.treatises AS
 WITH edition AS (
         SELECT v.bibliographicid,
            min(v.year) AS year,
            min(v.display_title) AS title,
                CASE
                    WHEN max(v.current_volume) = 0 THEN 1
                    ELSE max(v.current_volume)
                END AS vols,
            sum(v.total_pages) AS pages
           FROM moml.volumes v
          GROUP BY v.bibliographicid
        )
 SELECT e.bibliographicid,
    j.subject AS jurisdiction,
    e.year,
    e.title,
    e.vols
   FROM edition e
     JOIN moml.edition_subjects j
       ON j.bibliographicid = e.bibliographicid AND j.subject IN ('US', 'UK')
  WHERE NOT EXISTS (
          SELECT 1 FROM moml.editions c
           WHERE c.bibliographicid = e.bibliographicid AND c.copy_of IS NOT NULL)
    AND NOT EXISTS (
          SELECT 1 FROM moml.edition_subjects s
           WHERE s.bibliographicid = e.bibliographicid
             AND s.subject IN ('Biography', 'Collected Essays', 'Trials'))
    AND NOT e.title ~* '\Wremarks of\W'
    AND NOT e.title ~* '\y(address|oration|eulogy|sermon|memorial|in memoriam|obituary)\y'
    AND (e.title ~* '\ytreatise\y'
         OR (coalesce(e.pages, 50) >= 50
             AND NOT e.title ~* (
                   '^(the )?((first|second|third|fourth|fifth|final|annual|special|preliminary|majority|minority|supplementary) )*reports? (of|from|to) (the )?(\w+ ){0,6}(committee|commission|commissioners|board|council|delegates|attorney|secretary|comptroller|superintendent)'
                || '|^(the )?(hearings?|message|debates?|journal of the|proceedings of the (senate|house|convention|legislature))\y'
                || '|^(the )?(argument|closing argument|opening argument|brief|reply brief)s? (of|for|on behalf of|in|by|against|submitted)\y'
                || '|^in the (supreme )?court\y'
                || '|(^|: )(the )?speech(es)? (of|delivered|in the|on)\y'
                || '|(^|: )(a |an |the )?(second |third |open |plain )?letters? (to|addressed to)\y')
             AND NOT EXISTS (
                   SELECT 1 FROM moml.edition_loc_subjects l
                    WHERE l.bibliographicid = e.bibliographicid
                      AND ((l.subfield IN ('v', 'x')
                            AND btrim(l.locsubject) IN ('Biography', 'Speeches in Congress'))
                        OR (l.subfield = 'a'
                            AND (l.locsubject IN ('Campaign biography', 'Forensic orations',
                                                  'Fourth of July orations', 'Baccalaureate addresses')
                                 OR l.locsubject ~ '^Campaign speeches'))))))
  ORDER BY e.year, e.title;

COMMENT ON VIEW moml.treatises IS
  'Editions that count as legal treatises, one row for each by its official copy (editions.copy_of IS NULL), with their jurisdiction (US or UK); excludes pamphlets under 50 pages, public documents and arguments in a case, biographies, speeches, collected essays, trials and commemorative pieces, unless the title calls the work a treatise. Join moml.volumes on bibliographicid for the volumes.';

-- work_citation_counts counts all of a work's editions, which must not count
-- a copy. Nothing reads it but chambers, so it is dropped and made again;
-- make db-maintenance fills it.
DROP MATERIALIZED VIEW IF EXISTS moml_citations.work_citation_counts;

CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.work_citation_counts AS
WITH cases AS (
  SELECT ecc.work_id,
         count(DISTINCT c.source || ':' || COALESCE(c.cap_case_id::text, c.er_case_id,
                                                    c.code_reporter_id::text, c.stub_cite)) AS cases
  FROM moml_citations.edition_case_citations c
  JOIN moml_citations.edition_citation_counts ecc ON ecc.bibliographicid = c.bibliographicid
  GROUP BY ecc.work_id
),
all_editions AS (
  SELECT work_id, count(*) AS n FROM moml.editions WHERE copy_of IS NULL GROUP BY work_id
)
SELECT ecc.work_id,
       count(*)                                    AS editions,
       ae.n                                        AS all_editions,
       count(*) FILTER (WHERE ecc.derivative)      AS derivative_editions,
       min(ecc.year)                               AS first_year,
       max(ecc.year)                               AS last_year,
       bool_or(ecc.jurisdiction = 'US')            AS us,
       bool_or(ecc.jurisdiction = 'UK')            AS uk,
       sum(ecc.cites)::bigint                      AS cites,
       sum(ecc.linked)::bigint                     AS linked,
       COALESCE(max(cases.cases), 0)               AS cases
FROM moml_citations.edition_citation_counts ecc
JOIN all_editions ae ON ae.work_id = ecc.work_id
LEFT JOIN cases ON cases.work_id = ecc.work_id
GROUP BY ecc.work_id, ae.n
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW moml_citations.work_citation_counts IS 'One row per work with at least one treatise edition: its treatise editions, all its editions (each counted once, by its official copy), the span of years, jurisdictions, citation totals and distinct cases cited (issue #305). Refreshed by make db-maintenance.';

CREATE UNIQUE INDEX IF NOT EXISTS work_citation_counts_uq
  ON moml_citations.work_citation_counts (work_id);
CREATE INDEX IF NOT EXISTS work_citation_counts_cites_idx
  ON moml_citations.work_citation_counts (cites DESC);

-- migrate:down
SET ROLE = law_admin;

DROP MATERIALIZED VIEW IF EXISTS moml_citations.work_citation_counts;

CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.work_citation_counts AS
WITH cases AS (
  SELECT ecc.work_id,
         count(DISTINCT c.source || ':' || COALESCE(c.cap_case_id::text, c.er_case_id,
                                                    c.code_reporter_id::text, c.stub_cite)) AS cases
  FROM moml_citations.edition_case_citations c
  JOIN moml_citations.edition_citation_counts ecc ON ecc.bibliographicid = c.bibliographicid
  GROUP BY ecc.work_id
),
all_editions AS (
  SELECT work_id, count(*) AS n FROM moml.editions GROUP BY work_id
)
SELECT ecc.work_id,
       count(*)                                    AS editions,
       ae.n                                        AS all_editions,
       count(*) FILTER (WHERE ecc.derivative)      AS derivative_editions,
       min(ecc.year)                               AS first_year,
       max(ecc.year)                               AS last_year,
       bool_or(ecc.jurisdiction = 'US')            AS us,
       bool_or(ecc.jurisdiction = 'UK')            AS uk,
       sum(ecc.cites)::bigint                      AS cites,
       sum(ecc.linked)::bigint                     AS linked,
       COALESCE(max(cases.cases), 0)               AS cases
FROM moml_citations.edition_citation_counts ecc
JOIN all_editions ae ON ae.work_id = ecc.work_id
LEFT JOIN cases ON cases.work_id = ecc.work_id
GROUP BY ecc.work_id, ae.n
WITH NO DATA;

COMMENT ON MATERIALIZED VIEW moml_citations.work_citation_counts IS 'One row per work with at least one treatise edition: its treatise editions, all its editions, the span of years, jurisdictions, citation totals and distinct cases cited (issue #305). Refreshed by make db-maintenance.';

CREATE UNIQUE INDEX IF NOT EXISTS work_citation_counts_uq
  ON moml_citations.work_citation_counts (work_id);
CREATE INDEX IF NOT EXISTS work_citation_counts_cites_idx
  ON moml_citations.work_citation_counts (cites DESC);

CREATE OR REPLACE VIEW moml.treatises AS
 WITH edition AS (
         SELECT v.bibliographicid,
            min(v.year) AS year,
            min(v.display_title) AS title,
                CASE
                    WHEN max(v.current_volume) = 0 THEN 1
                    ELSE max(v.current_volume)
                END AS vols,
            sum(v.total_pages) AS pages
           FROM moml.volumes v
          GROUP BY v.bibliographicid
        )
 SELECT e.bibliographicid,
    j.subject AS jurisdiction,
    e.year,
    e.title,
    e.vols
   FROM edition e
     JOIN moml.edition_subjects j
       ON j.bibliographicid = e.bibliographicid AND j.subject IN ('US', 'UK')
  WHERE NOT EXISTS (
          SELECT 1 FROM moml.edition_subjects s
           WHERE s.bibliographicid = e.bibliographicid
             AND s.subject IN ('Biography', 'Collected Essays', 'Trials'))
    AND NOT e.title ~* '\Wremarks of\W'
    AND NOT e.title ~* '\y(address|oration|eulogy|sermon|memorial|in memoriam|obituary)\y'
    AND (e.title ~* '\ytreatise\y'
         OR (coalesce(e.pages, 50) >= 50
             AND NOT e.title ~* (
                   '^(the )?((first|second|third|fourth|fifth|final|annual|special|preliminary|majority|minority|supplementary) )*reports? (of|from|to) (the )?(\w+ ){0,6}(committee|commission|commissioners|board|council|delegates|attorney|secretary|comptroller|superintendent)'
                || '|^(the )?(hearings?|message|debates?|journal of the|proceedings of the (senate|house|convention|legislature))\y'
                || '|^(the )?(argument|closing argument|opening argument|brief|reply brief)s? (of|for|on behalf of|in|by|against|submitted)\y'
                || '|^in the (supreme )?court\y'
                || '|(^|: )(the )?speech(es)? (of|delivered|in the|on)\y'
                || '|(^|: )(a |an |the )?(second |third |open |plain )?letters? (to|addressed to)\y')
             AND NOT EXISTS (
                   SELECT 1 FROM moml.edition_loc_subjects l
                    WHERE l.bibliographicid = e.bibliographicid
                      AND ((l.subfield IN ('v', 'x')
                            AND btrim(l.locsubject) IN ('Biography', 'Speeches in Congress'))
                        OR (l.subfield = 'a'
                            AND (l.locsubject IN ('Campaign biography', 'Forensic orations',
                                                  'Fourth of July orations', 'Baccalaureate addresses')
                                 OR l.locsubject ~ '^Campaign speeches'))))))
  ORDER BY e.year, e.title;

COMMENT ON VIEW moml.treatises IS
  'Editions that count as legal treatises, with their jurisdiction (US or UK); excludes pamphlets under 50 pages, public documents and arguments in a case, biographies, speeches, collected essays, trials and commemorative pieces, unless the title calls the work a treatise. Join moml.volumes on bibliographicid for the volumes.';

DROP VIEW IF EXISTS moml.edition_copies;
DROP FUNCTION IF EXISTS moml.prefer_hathitrust_copies();
DROP TRIGGER IF EXISTS editions_copy_check ON moml.editions;
DROP FUNCTION IF EXISTS moml.check_edition_copy();
DROP INDEX IF EXISTS moml.editions_copy_of_idx;
ALTER TABLE moml.editions DROP CONSTRAINT IF EXISTS editions_copy_of_check;
ALTER TABLE moml.editions DROP COLUMN IF EXISTS copy_of;
