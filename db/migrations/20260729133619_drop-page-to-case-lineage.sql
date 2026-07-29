-- migrate:up
SET ROLE = law_admin;

-- Drop the dead moml_citations.page_to_case lineage (issue #235).
--
-- page_to_case is a superseded predecessor of the current treatise-to-case
-- pipeline (moml_citations.citations_unlinked -> citation_links, written by
-- cite-detector-moml and cite-linker). It holds 0 rows, nothing in the
-- codebase reads or writes it, and it was never created by a migration --
-- it predates dbmate and existed only in the db/schema.sql dump. Its three
-- indexes still carry the name of an even earlier table, moml_page_to_cap_case.
--
-- Everything downstream of it is dead or misleading:
--
--   page_to_case (table)
--     `-- volume_to_case (view, 0 rows)
--           |-- bibliocouple_treatises (matview, 0 rows -- so any
--           |     bibliographic-coupling analysis built on it returns nothing)
--           `-- schools_to_cases (matview, 4,087 rows, but joined to
--                 volume_to_case with a LEFT JOIN, so moml_treatise and "case"
--                 are NULL in every single row -- it looks populated and is not)
--
-- Dropped dependents-first rather than with CASCADE, so this cannot silently
-- take down an object the audit did not anticipate. Dropping the table takes
-- its primary key and its three indexes with it.
--
-- This removes the dead path only. Rebuilding bibliographic coupling on top of
-- citation_links (issue #235's second option) remains open as separate work.

DROP MATERIALIZED VIEW IF EXISTS textbooks.schools_to_cases;
DROP MATERIALIZED VIEW IF EXISTS moml_citations.bibliocouple_treatises;
DROP VIEW IF EXISTS moml_citations.volume_to_case;
DROP TABLE IF EXISTS moml_citations.page_to_case;

-- migrate:down
SET ROLE = law_admin;

-- Recreates the four objects exactly as db/schema.sql defined them before this
-- migration, in dependency order. This restores structure, not data:
-- page_to_case and bibliocouple_treatises were empty anyway, and
-- schools_to_cases reproduces its 4,087 rows identically after a REFRESH,
-- because every column of it that was ever non-NULL comes from
-- legalhist.textbooks_works and legalhist.textbooks_vols, which this migration
-- does not touch.

CREATE TABLE IF NOT EXISTS moml_citations.page_to_case (
    id uuid NOT NULL,
    moml_treatise text,
    moml_page text,
    cite_in_moml text,
    cap_link_cite text,
    "case" bigint
);

ALTER TABLE ONLY moml_citations.page_to_case
    DROP CONSTRAINT IF EXISTS moml_page_to_cap_case_pkey;

ALTER TABLE ONLY moml_citations.page_to_case
    ADD CONSTRAINT moml_page_to_cap_case_pkey PRIMARY KEY (id);

CREATE INDEX IF NOT EXISTS moml_page_to_cap_case_case_idx
    ON moml_citations.page_to_case USING btree ("case");

CREATE INDEX IF NOT EXISTS moml_page_to_cap_case_moml_page_idx
    ON moml_citations.page_to_case USING btree (moml_page);

CREATE INDEX IF NOT EXISTS moml_page_to_cap_case_moml_treatise_idx
    ON moml_citations.page_to_case USING btree (moml_treatise);

CREATE OR REPLACE VIEW moml_citations.volume_to_case AS
 SELECT moml_treatise,
    "case",
    count(*) AS n
   FROM moml_citations.page_to_case
  GROUP BY moml_treatise, "case";

CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.bibliocouple_treatises AS
 WITH ut2c AS (
         SELECT DISTINCT t2c.bibliographicid,
            t2c."case"
           FROM ( SELECT t.bibliographicid,
                    c.moml_treatise AS psmid,
                    c."case"
                   FROM ((moml_citations.volume_to_case c
                     LEFT JOIN moml.book_info bi ON ((c.moml_treatise = (bi.psmid)::text)))
                     LEFT JOIN moml.us_treatises t ON (((bi.bibliographicid)::text = t.bibliographicid)))) t2c
        )
 SELECT cites1.bibliographicid AS t1,
    cites2.bibliographicid AS t2,
    count(*) AS n
   FROM (ut2c cites1
     LEFT JOIN ut2c cites2 ON ((cites1."case" = cites2."case")))
  WHERE (cites1.bibliographicid <> cites2.bibliographicid)
  GROUP BY cites1.bibliographicid, cites2.bibliographicid
  WITH NO DATA;

CREATE MATERIALIZED VIEW IF NOT EXISTS textbooks.schools_to_cases AS
 SELECT s2w2v.school,
    s2w2v.school_state,
    s2w2v.region,
    s2w2v.course,
    s2w2v.topic,
    s2w2v.subtopic,
    s2w2v.year_begin,
    s2w2v.year_end,
    s2w2v.workid,
    s2w2v.work_title,
    s2w2v.moml_webid,
    m6.moml_treatise,
    m6."case",
    c.name_abbreviation,
    c.decision_year,
    j.name_long AS jurisdiction_name
   FROM (((( SELECT tw.workid,
            tw.work_title,
            tw.moml_webid,
            tv.bibliographicid,
            tv.psmid,
            tv.webid,
            tv.school,
            tv.title,
            tv.edition,
            tv.topic,
            tv.year_begin,
            tv.year_end,
            tv.course,
            tv.subtopic,
            tv.school_state,
            tv.region,
            tv.class_year
           FROM (( SELECT textbooks_works.workid,
                    textbooks_works.work_title,
                    unnest(textbooks_works.moml_webids) AS moml_webid
                   FROM legalhist.textbooks_works) tw
             LEFT JOIN legalhist.textbooks_vols tv ON ((tw.moml_webid = tv.webid)))) s2w2v
     LEFT JOIN moml_citations.volume_to_case m6 ON ((s2w2v.psmid = m6.moml_treatise)))
     LEFT JOIN cap.cases c ON ((m6."case" = c.id)))
     LEFT JOIN cap.jurisdictions j ON ((c.jurisdiction = j.id)))
  WITH NO DATA;
