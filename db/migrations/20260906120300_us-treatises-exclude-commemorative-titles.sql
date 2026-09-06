-- migrate:up
SET ROLE = law_admin;

-- Exclude commemorative and occasional pieces from moml.us_treatises by title.
--
-- The view already dropped two of these -- '\Woration\W' and '\Waddress\W' --
-- but \W requires a non-word character on BOTH sides of the match, so a title
-- that begins with the word was never excluded: "Address to the graduating
-- class of the Law School of Columbia College", "Oration on the centennial
-- anniversary of the Declaration of Independence". Postgres spells the
-- word-boundary assertion \y, which matches at the start of the string as well,
-- so the single \y pattern below covers those positions and the words added to
-- it in one predicate.
--
-- Measured on 2026-09-06, after the detection run of that morning, against the
-- view as it stands: the new predicate removes 200 volumes in 200 works,
-- leaving 10,352 works and 11,924 volumes. 63 of the 200 are volumes the
-- citation detector found no citations in at all, which is 12% of the 513 such
-- volumes then in the view -- so this is a modest cut at the genre, not a fix
-- for the whole tail. The other 137 do carry citations; they are addresses and
-- memorials that quote a case in passing, and the judgement recorded here is
-- that a eulogy is not a treatise whether or not it cites one.
--
-- Nothing else about the view changes. In particular '\Wremarks of\W' keeps its
-- old anchoring, since it was not part of this decision.
CREATE OR REPLACE VIEW moml.us_treatises AS
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

-- migrate:down
SET ROLE = law_admin;

CREATE OR REPLACE VIEW moml.us_treatises AS
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
    AND NOT title ~* '\Woration\W'::text
    AND NOT title ~* '\Wremarks of\W'::text
    AND NOT title ~* '\Waddress\W'::text;
