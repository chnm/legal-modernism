-- migrate:up
SET ROLE = law_admin;

-- Leave pamphlets, public documents, biographies and speeches out of
-- moml.treatises (issue #142).
--
-- moml.treatises already left out editions with the Gale subject Biography,
-- Collected Essays or Trials and commemorative pieces by title. Many of the
-- editions it kept were still not treatises: a treatise is a book-length
-- exposition of the law, and MOML also holds pamphlets, legislative and
-- executive documents, arguments in particular cases, and lives of judges and
-- lawyers. Three rules now leave them out:
--
--   * Length. An edition whose volumes total fewer than 50 pages is a pamphlet:
--     a speech, a letter to a minister, a petition, a charge to a grand jury, a
--     paper read before a bar association, a committee's report. An edition
--     with no page count (the side corpus) is kept.
--   * Titles that name a document rather than a book: the report of a
--     committee or commission; a hearing, message, debate or legislative
--     journal; the argument or brief of counsel in a case; a speech; a letter
--     to someone.
--   * The Library of Congress subject headings, used sparingly: an edition
--     catalogued as biography (form subdivision Biography, or the heading
--     Campaign biography) or as speeches (Speeches in Congress, Forensic
--     orations, Fourth of July orations, Baccalaureate addresses, Campaign
--     speeches). Works for practitioners and students that the headings mark as
--     digests, casebooks, dictionaries, forms or outlines stay: they are how
--     the profession used the law, whatever their form.
--
-- None of the three removes an edition whose title calls it a treatise. That
-- keeps twenty short treatises, such as A treatise on the law of the descent
-- of an intestate's real estate (49 pages), and one treatise catalogued with a
-- biography of its author.
--
-- Measured on 2026-09-25, the rules remove 3,473 of the 19,612 editions: 3,046
-- by length, 218 by title and 209 as biographies or speeches. That leaves
-- 16,139 treatises, 7,893 U.S. and 8,246 English, which carry 56,032,703 of the
-- 56,214,167 citations detected in the view's editions; the editions removed
-- carry 181,464. The view's columns are unchanged, and the migration checks
-- that it only removes rows. db/queries/treatises-excluded.sql lists each
-- edition the rules remove and why.

CREATE TEMPORARY TABLE treatises_before ON COMMIT DROP AS
  SELECT * FROM moml.treatises;

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

DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n FROM (
    SELECT * FROM moml.treatises EXCEPT ALL SELECT * FROM treatises_before) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.treatises gained or changed % rows; the new rules may only remove editions', n;
  END IF;
END $$;

-- migrate:down
SET ROLE = law_admin;

CREATE OR REPLACE VIEW moml.treatises AS
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
