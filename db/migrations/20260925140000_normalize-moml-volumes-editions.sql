-- migrate:up
SET ROLE = law_admin;

-- Normalize the MOML metadata into volumes and editions (issue #142).
--
-- In the Making of Modern Law, psmid identifies a volume and bibliographicid
-- an edition, which has one volume or many: 21,815 editions hold the 25,025
-- volumes, 1,967 of them with more than one (up to 18). A work -- Kent's
-- Commentaries across its editions -- is a third level that needs judgement
-- about which editions belong together, and it is left for later.
--
-- The metadata arrived as per-volume tables, so the edition existed only as a
-- column and the moml.treatises view. This builds moml.editions and
-- moml.volumes from book_info, book_citation and legal_treatises_metadata;
-- moml.edition_subjects and moml.edition_loc_subjects from book_subject and
-- book_locsubjecthead; and moml.volume_sets from book_volumeset. The six old
-- tables are then moved, untouched, into a new schema, moml_archive. A later
-- migration drops the archive once the new tables have been checked.
--
-- Nothing is lost, and the migration proves it: before anything is moved it
-- rebuilds each of the six old tables from the new ones and aborts unless the
-- rebuild equals the original, row for row and value for value (EXCEPT ALL in
-- both directions, so duplicate rows count). The original side of each
-- comparison is SELECT *, so a column the rebuild forgot would change the
-- arity and abort as well.
--
-- Where a column lives. A column goes on editions only if no edition has two
-- values for it, which the migration asserts before collapsing. Author,
-- author role, total volumes, collation, place of publication, the language,
-- category and collection codes are such columns. Title, full title, edition
-- statement, imprint, date statement, year, source library and fiche range
-- differ between the volumes of a few editions, and so stay on volumes: some
-- volumes have their own title page (Chitty's Pleading, whose first volume is
-- on the parties to actions and the rest on precedents), some sets mix
-- editions (Campbell's Lives of the Lord Chancellors, vols. 2-3 of the 4th
-- edition and the rest of the 5th), and some came from different libraries.
-- The edition-level title and year remain derived in moml.treatises, as
-- before.
--
-- Where a value comes from:
--
--   * author is legal_treatises_metadata.author_by_line, and edition_statement
--     is legal_treatises_metadata.edition. They exist nowhere else: in
--     book_citation the author, edition and edition statement columns are NULL
--     in every row. Empty strings become NULL (447 volumes have no author, and
--     17,106 no edition statement).
--   * Dropped because they are NULL in every row: book_citation.author_composed,
--     author_first, author_middle, author_last, author_birthdate,
--     author_deathdate, varianttitle, edition, editionstatement,
--     imprintpublisher; book_info.notes and pubdate_year.
--   * Dropped because they repeat another column: book_citation.volume (equal
--     to currentvolume, and NULL for the side corpus); book_volumeset.assetid
--     (the sibling volume's asset_id); legal_treatises_metadata.title (the
--     full title without surrounding whitespace), imprint, book_collation and
--     pages (equal to the book_citation columns), and current_volume ('' for
--     a volume numbered 0, and '1' for the side corpus).
--   * currentvolume, totalvolume, totalpages and filmedvolume become integers
--     and the OCR confidence a numeric; the migration checks that each is a
--     plain number, so the conversion round-trips.
--   * current_volume 0 means the volume is not part of a numbered set. It is
--     not unique within an edition: six editions hold two copies of one volume.
--
-- The side corpus. The eight volumes momlextra001 to momlextra008 were added
-- from HathiTrust by scripts/sidecorpus-import. They have no Gale record, so
-- every Gale field is NULL, and each is its own edition (bibliographicid =
-- psmid). The rebuild identifies them by their missing Gale id.
--
-- The four orphans. legal_treatises_metadata has catalog records for four
-- psmids with no book_info row, no pages and no citations (19000513300,
-- 20002284100, 20002284400, 20002284500). They are not volumes, so they stay
-- only in the archive; the migration checks that nothing refers to them.
--
-- Subjects. The subjects and Library of Congress subject headings of an
-- edition's volumes are identical, multiplicity included, which the migration
-- asserts, so each edition's rows are copied once from one of its volumes. A
-- LoC heading is stored as one row per MARC subfield, and the heading that
-- groups them was never recorded; 14,963 rows repeat another row of the same
-- volume because the same subfield appears in two headings, and they are
-- kept. The physical order of the rows does carry the grouping: in ctid order
-- a volume's rows run in MARC order, and each subfield "a" begins a heading.
-- So both subject tables get a position column taken from that order. It is
-- exact for book_subject, which the migration checks. For the LoC headings the
-- representative volume is the one whose rows span the fewest heap blocks, and
-- the order is best effort where every volume's rows cross a block boundary.
--
-- Volume sets. book_volumeset lists, for each volume of a multi-volume set,
-- its siblings. Most of it could be derived from bibliographicid, but not all:
-- six pairs of volumes are siblings across two editions (a work and its
-- supplement, catalogued separately), and it records a filmed volume number
-- that once contradicts the catalogue. It is kept as moml.volume_sets.
--
-- The views. moml.treatises is redefined over the new tables and moml.us_treatises
-- is unchanged. Both are snapshotted first, and the migration asserts that
-- their output is identical afterwards. CREATE OR REPLACE VIEW itself refuses a
-- change to a column's type.
--
-- Foreign keys. moml.page and legalhist.textbooks_vols now reference
-- moml.volumes. page_ocrtext's key onto legal_treatises_metadata is dropped:
-- page_ocrtext already references moml.page, which references moml.volumes.
-- Nothing outside moml_archive refers to the archived tables afterwards, which
-- the migration asserts, so the later DROP SCHEMA needs no CASCADE. The
-- archived tables are made read-only for law_dev.
--
-- Running it takes well under a minute. The foreign key on moml.page is
-- validated against its 10.5 million rows, and the migration holds brief
-- exclusive locks on moml.page, moml.page_ocrtext and legalhist.textbooks_vols,
-- so apply it while no detector or linker job is running. Programs built
-- before this change read moml.book_info and must be rebuilt.
--
-- The whole migration runs in one transaction, so any failed assertion rolls
-- all of it back.

CREATE SCHEMA IF NOT EXISTS moml_archive;
GRANT USAGE ON SCHEMA moml_archive TO law_service, law_dev;
COMMENT ON SCHEMA moml_archive IS
  'Original MOML metadata tables, kept unchanged after the move to moml.volumes and moml.editions (issue #142); to be dropped by a later migration';

-- Snapshot the views so their output can be compared after the change.
CREATE TEMPORARY TABLE treatises_before ON COMMIT DROP AS
  SELECT * FROM moml.treatises;
CREATE TEMPORARY TABLE us_treatises_before ON COMMIT DROP AS
  SELECT * FROM moml.us_treatises;

-- Check the assumptions the new structure rests on.
DO $$
DECLARE
  n bigint;
  orphans text[];
BEGIN
  SELECT count(*) INTO n FROM moml.book_info WHERE bibliographicid IS NULL;
  IF n <> 0 THEN
    RAISE EXCEPTION '% volumes have no bibliographicid', n;
  END IF;

  SELECT count(*) - count(DISTINCT psmid) INTO n FROM moml.book_citation;
  IF n <> 0 OR EXISTS (SELECT 1 FROM moml.book_citation WHERE psmid IS NULL) THEN
    RAISE EXCEPTION 'moml.book_citation is not one row per psmid';
  END IF;

  SELECT count(*) INTO n
  FROM moml.book_info bi
  JOIN moml.book_citation bc USING (psmid)
  JOIN moml.legal_treatises_metadata l USING (psmid);
  IF n <> (SELECT count(*) FROM moml.book_info)
     OR n <> (SELECT count(*) FROM moml.book_citation) THEN
    RAISE EXCEPTION 'book_info, book_citation and legal_treatises_metadata do not cover the same volumes';
  END IF;

  SELECT array_agg(psmid ORDER BY psmid) INTO orphans
  FROM moml.legal_treatises_metadata l
  WHERE NOT EXISTS (SELECT 1 FROM moml.book_info bi WHERE bi.psmid = l.psmid);
  IF orphans IS DISTINCT FROM
     ARRAY['19000513300', '20002284100', '20002284400', '20002284500']::text[] THEN
    RAISE EXCEPTION 'unexpected legal_treatises_metadata rows without a volume: %', orphans;
  END IF;
  IF EXISTS (SELECT 1 FROM moml_citations.citations_unlinked
             WHERE moml_treatise = ANY (orphans)) THEN
    RAISE EXCEPTION 'citations refer to the orphan catalog records';
  END IF;

  IF EXISTS (SELECT 1 FROM moml.book_info
             WHERE (id IS NULL) <> (psmid LIKE 'momlextra%')
                OR (id IS NULL AND bibliographicid <> psmid)) THEN
    RAISE EXCEPTION 'the side-corpus volumes are not exactly those without a Gale id';
  END IF;

  SELECT count(*) INTO n FROM moml.book_citation
  WHERE currentvolume IS NULL
     OR currentvolume !~ '^(0|[1-9][0-9]*)$'
     OR totalvolume !~ '^(0|[1-9][0-9]*)$'
     OR totalpages !~ '^(0|[1-9][0-9]*)$';
  IF n <> 0 THEN
    RAISE EXCEPTION '% book_citation rows have a volume or page count that is not a plain number', n;
  END IF;
  IF EXISTS (SELECT 1 FROM moml.book_volumeset
             WHERE filmedvolume IS NULL OR filmedvolume !~ '^(0|[1-9][0-9]*)$') THEN
    RAISE EXCEPTION 'a book_volumeset.filmedvolume is not a plain number';
  END IF;
  IF EXISTS (SELECT 1 FROM moml.book_info WHERE ocr !~ '^[0-9]+(\.[0-9]+)?$') THEN
    RAISE EXCEPTION 'a book_info.ocr value is not a plain number';
  END IF;

  -- One value per edition for every column that moves to moml.editions.
  SELECT count(*) INTO n FROM (
    SELECT DISTINCT bi.bibliographicid, bi.bibliographicid_type, bi.contenttype,
           bi.faid, bi.colid, bi.dvicollectionid, bi.unit, bi.mcode, bi.releasedate,
           bi.language, bi.language_ocr, bi.language_primary, bi.documenttype,
           bi.categorycode, bi.categorycode_source, bc.author_role, bc.totalvolume,
           bc.book_collation, bc.publicationplacecity, bc.publicationplacecomposed,
           l.author_by_line
    FROM moml.book_info bi
    JOIN moml.book_citation bc USING (psmid)
    JOIN moml.legal_treatises_metadata l USING (psmid)) d;
  IF n <> (SELECT count(DISTINCT bibliographicid) FROM moml.book_info) THEN
    RAISE EXCEPTION 'an edition-level column has more than one value in some edition';
  END IF;

  -- Every volume of an edition has the same subjects and LoC headings as the
  -- edition's first volume, duplicates included.
  SELECT count(*) INTO n FROM (
    (SELECT bi.psmid, s.subject, s.source
     FROM moml.book_info bi
     JOIN moml.book_info f ON f.psmid = (SELECT min(psmid) FROM moml.book_info x
                                         WHERE x.bibliographicid = bi.bibliographicid)
     JOIN moml.book_subject s ON s.psmid = f.psmid
     EXCEPT ALL
     SELECT psmid, subject, source FROM moml.book_subject)
    UNION ALL
    (SELECT psmid, subject, source FROM moml.book_subject
     EXCEPT ALL
     SELECT bi.psmid, s.subject, s.source
     FROM moml.book_info bi
     JOIN moml.book_info f ON f.psmid = (SELECT min(psmid) FROM moml.book_info x
                                         WHERE x.bibliographicid = bi.bibliographicid)
     JOIN moml.book_subject s ON s.psmid = f.psmid)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'subjects differ between the volumes of an edition (% rows)', n;
  END IF;

  SELECT count(*) INTO n FROM (
    (SELECT bi.psmid, s.type, s.subfield, s.locsubject
     FROM moml.book_info bi
     JOIN moml.book_info f ON f.psmid = (SELECT min(psmid) FROM moml.book_info x
                                         WHERE x.bibliographicid = bi.bibliographicid)
     JOIN moml.book_locsubjecthead s ON s.psmid = f.psmid
     EXCEPT ALL
     SELECT psmid, type, subfield, locsubject FROM moml.book_locsubjecthead)
    UNION ALL
    (SELECT psmid, type, subfield, locsubject FROM moml.book_locsubjecthead
     EXCEPT ALL
     SELECT bi.psmid, s.type, s.subfield, s.locsubject
     FROM moml.book_info bi
     JOIN moml.book_info f ON f.psmid = (SELECT min(psmid) FROM moml.book_info x
                                         WHERE x.bibliographicid = bi.bibliographicid)
     JOIN moml.book_locsubjecthead s ON s.psmid = f.psmid)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'LoC subject headings differ between the volumes of an edition (% rows)', n;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS moml.editions (
    bibliographicid      text PRIMARY KEY,
    bibliographicid_type text,
    author               text,
    author_role          text,
    total_volumes        integer CHECK (total_volumes >= 0),
    book_collation       text,
    publication_place    text,
    publication_city     text,
    language             text,
    language_ocr         text,
    language_primary     text,
    content_type         text,
    document_type        text,
    category_code        text,
    category_code_source text,
    unit                 text,
    mcode                text,
    faid                 text,
    colid                text,
    dvi_collection_id    text,
    release_date         text
);

COMMENT ON TABLE moml.editions IS
  'A MOML edition, identified by its bibliographicid; it has one or more volumes in moml.volumes. Only attributes shared by every volume of the edition are kept here.';
COMMENT ON COLUMN moml.editions.author IS
  'The author line as catalogued (legal_treatises_metadata.author_by_line); NULL where it was empty.';
COMMENT ON COLUMN moml.editions.total_volumes IS
  'Number of volumes the catalogue gives for the edition; 0 for a work not in numbered volumes. MOML may hold fewer.';

CREATE TABLE IF NOT EXISTS moml.volumes (
    psmid                   text PRIMARY KEY,
    bibliographicid         text NOT NULL REFERENCES moml.editions (bibliographicid),
    current_volume          integer NOT NULL CHECK (current_volume >= 0),
    display_title           text NOT NULL,
    full_title              text NOT NULL,
    edition_statement       text,
    imprint                 text,
    year                    integer,
    pubdate_composed        text,
    pubdate_start           text,
    total_pages             integer CHECK (total_pages > 0),
    source_library          text,
    source_library_location text,
    gale_id                 text UNIQUE,
    asset_id                text UNIQUE,
    asset_id_etoc           text,
    webid                   text NOT NULL UNIQUE,
    product_link            text NOT NULL,
    fiche_range             text,
    ocr_confidence          numeric
);

CREATE INDEX IF NOT EXISTS volumes_bibliographicid_idx ON moml.volumes (bibliographicid);

COMMENT ON TABLE moml.volumes IS
  'A MOML volume, identified by its psmid, belonging to the edition moml.editions.bibliographicid. Attributes that can differ between the volumes of an edition (title, edition statement, imprint, dates) are kept here.';
COMMENT ON COLUMN moml.volumes.current_volume IS
  'Volume number within the edition; 0 means the volume is not part of a numbered set. Not unique within an edition.';
COMMENT ON COLUMN moml.volumes.year IS
  'Publication year of the volume, used by cite-linker''s anachronism rule.';
COMMENT ON COLUMN moml.volumes.edition_statement IS
  'Edition statement as catalogued (legal_treatises_metadata.edition), e.g. ''9th ed., rev.''; NULL where it was empty.';
COMMENT ON COLUMN moml.volumes.gale_id IS
  'Gale''s document id (book_info.id); NULL for the side-corpus volumes, which have no Gale record.';

CREATE TABLE IF NOT EXISTS moml.edition_subjects (
    bibliographicid text NOT NULL REFERENCES moml.editions (bibliographicid),
    position        integer NOT NULL CHECK (position > 0),
    subject         text NOT NULL,
    source          text,
    PRIMARY KEY (bibliographicid, subject),
    UNIQUE (bibliographicid, position)
);

CREATE INDEX IF NOT EXISTS edition_subjects_subject_idx ON moml.edition_subjects (subject);

COMMENT ON TABLE moml.edition_subjects IS
  'Gale subject terms of an edition, in catalogue order (sub-topic, topic, jurisdiction).';

CREATE TABLE IF NOT EXISTS moml.edition_loc_subjects (
    bibliographicid text NOT NULL REFERENCES moml.editions (bibliographicid),
    position        integer NOT NULL CHECK (position > 0),
    type            text NOT NULL,
    subfield        text NOT NULL,
    locsubject      text,
    PRIMARY KEY (bibliographicid, position)
);

COMMENT ON TABLE moml.edition_loc_subjects IS
  'Library of Congress subject headings of an edition, one row per MARC subfield in catalogue order; subfield ''a'' begins a heading. Rows may repeat.';

CREATE TABLE IF NOT EXISTS moml.volume_sets (
    psmid         text NOT NULL REFERENCES moml.volumes (psmid),
    sibling_psmid text NOT NULL REFERENCES moml.volumes (psmid),
    filmed_volume integer NOT NULL,
    PRIMARY KEY (psmid, sibling_psmid),
    CHECK (psmid <> sibling_psmid)
);

CREATE INDEX IF NOT EXISTS volume_sets_sibling_psmid_idx ON moml.volume_sets (sibling_psmid);

COMMENT ON TABLE moml.volume_sets IS
  'For each volume of a multi-volume set, its sibling volumes as Gale records them. Siblings are usually volumes of the same edition, but a few pairs cross two editions (a work and its separately catalogued supplement).';

GRANT SELECT ON moml.editions, moml.volumes, moml.edition_subjects,
                moml.edition_loc_subjects, moml.volume_sets TO law_service;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE
   ON moml.editions, moml.volumes, moml.edition_subjects,
      moml.edition_loc_subjects, moml.volume_sets TO law_dev;

INSERT INTO moml.editions (
    bibliographicid, bibliographicid_type, author, author_role, total_volumes,
    book_collation, publication_place, publication_city, language, language_ocr,
    language_primary, content_type, document_type, category_code,
    category_code_source, unit, mcode, faid, colid, dvi_collection_id, release_date)
SELECT DISTINCT
    bi.bibliographicid, bi.bibliographicid_type, NULLIF(l.author_by_line, ''),
    bc.author_role, bc.totalvolume::integer, bc.book_collation,
    bc.publicationplacecomposed, bc.publicationplacecity, bi.language,
    bi.language_ocr, bi.language_primary, bi.contenttype, bi.documenttype,
    bi.categorycode, bi.categorycode_source, bi.unit, bi.mcode, bi.faid, bi.colid,
    bi.dvicollectionid, bi.releasedate
FROM moml.book_info bi
JOIN moml.book_citation bc USING (psmid)
JOIN moml.legal_treatises_metadata l USING (psmid);

INSERT INTO moml.volumes (
    psmid, bibliographicid, current_volume, display_title, full_title,
    edition_statement, imprint, year, pubdate_composed, pubdate_start,
    total_pages, source_library, source_library_location, gale_id, asset_id,
    asset_id_etoc, webid, product_link, fiche_range, ocr_confidence)
SELECT
    bi.psmid, bi.bibliographicid, bc.currentvolume::integer, bc.displaytitle,
    bc.fulltitle, NULLIF(l.edition, ''), bc.imprintfull, bi.year,
    bi.pubdate_composed, bi.pubdate_pubdatestart, bc.totalpages::integer,
    bi.sourcelibrary_libraryname, bi.sourcelibrary_librarylocation, bi.id,
    bi.assetid, bi.assetidetoc, bi.webid, bi.productlink, bi.ficherange,
    bi.ocr::numeric
FROM moml.book_info bi
JOIN moml.book_citation bc USING (psmid)
JOIN moml.legal_treatises_metadata l USING (psmid);

-- Subjects come from each edition's first volume, in physical order.
INSERT INTO moml.edition_subjects (bibliographicid, position, subject, source)
SELECT bi.bibliographicid,
       row_number() OVER (PARTITION BY s.psmid ORDER BY s.ctid),
       s.subject, s.source
FROM moml.book_subject s
JOIN moml.book_info bi ON bi.psmid = s.psmid
WHERE s.psmid = (SELECT min(psmid) FROM moml.book_info x
                 WHERE x.bibliographicid = bi.bibliographicid);

-- LoC headings come from the volume whose rows span the fewest heap blocks,
-- since its physical order is most likely to be the catalogue's.
INSERT INTO moml.edition_loc_subjects (bibliographicid, position, type, subfield, locsubject)
SELECT r.bibliographicid,
       row_number() OVER (PARTITION BY s.psmid ORDER BY s.ctid),
       s.type, s.subfield, s.locsubject
FROM moml.book_locsubjecthead s
JOIN (SELECT DISTINCT ON (bi.bibliographicid) bi.bibliographicid, l.psmid
      FROM moml.book_locsubjecthead l
      JOIN moml.book_info bi ON bi.psmid = l.psmid
      GROUP BY bi.bibliographicid, l.psmid
      ORDER BY bi.bibliographicid,
               count(DISTINCT (l.ctid::text::point)[0]),
               l.psmid) r ON r.psmid = s.psmid;

INSERT INTO moml.volume_sets (psmid, sibling_psmid, filmed_volume)
SELECT psmid, volumeid, filmedvolume::integer
FROM moml.book_volumeset;

-- Prove the six old tables can be rebuilt exactly from the new ones.
DO $$
DECLARE
  n bigint;
BEGIN
  -- moml.book_info
  SELECT count(*) INTO n FROM (
    (SELECT * FROM moml.book_info
     EXCEPT ALL
     SELECT v.psmid, e.content_type, v.gale_id, e.faid, e.colid,
            v.ocr_confidence::text, v.asset_id, v.asset_id_etoc,
            e.dvi_collection_id, v.bibliographicid, e.bibliographicid_type,
            e.unit, v.fiche_range, e.mcode, NULL::text, v.pubdate_composed,
            v.pubdate_start, e.release_date, v.source_library,
            v.source_library_location, e.language, e.language_ocr,
            e.language_primary, e.document_type, NULL::text, e.category_code,
            e.category_code_source, v.product_link, v.webid, v.year
     FROM moml.volumes v JOIN moml.editions e USING (bibliographicid))
    UNION ALL
    (SELECT v.psmid, e.content_type, v.gale_id, e.faid, e.colid,
            v.ocr_confidence::text, v.asset_id, v.asset_id_etoc,
            e.dvi_collection_id, v.bibliographicid, e.bibliographicid_type,
            e.unit, v.fiche_range, e.mcode, NULL::text, v.pubdate_composed,
            v.pubdate_start, e.release_date, v.source_library,
            v.source_library_location, e.language, e.language_ocr,
            e.language_primary, e.document_type, NULL::text, e.category_code,
            e.category_code_source, v.product_link, v.webid, v.year
     FROM moml.volumes v JOIN moml.editions e USING (bibliographicid)
     EXCEPT ALL
     SELECT * FROM moml.book_info)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.book_info cannot be rebuilt from the new tables: % rows differ', n;
  END IF;

  -- moml.book_citation
  SELECT count(*) INTO n FROM (
    (SELECT * FROM moml.book_citation
     EXCEPT ALL
     SELECT v.psmid, e.author_role, NULL::text, NULL::text, NULL::text,
            NULL::text, NULL::text, NULL::text, v.full_title, v.display_title,
            NULL::text, NULL::text, NULL::text, v.current_volume::text,
            CASE WHEN v.gale_id IS NULL THEN NULL ELSE v.current_volume::text END,
            e.total_volumes::text, v.imprint, NULL::text, e.book_collation,
            e.publication_city, e.publication_place, v.total_pages::text
     FROM moml.volumes v JOIN moml.editions e USING (bibliographicid))
    UNION ALL
    (SELECT v.psmid, e.author_role, NULL::text, NULL::text, NULL::text,
            NULL::text, NULL::text, NULL::text, v.full_title, v.display_title,
            NULL::text, NULL::text, NULL::text, v.current_volume::text,
            CASE WHEN v.gale_id IS NULL THEN NULL ELSE v.current_volume::text END,
            e.total_volumes::text, v.imprint, NULL::text, e.book_collation,
            e.publication_city, e.publication_place, v.total_pages::text
     FROM moml.volumes v JOIN moml.editions e USING (bibliographicid)
     EXCEPT ALL
     SELECT * FROM moml.book_citation)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.book_citation cannot be rebuilt from the new tables: % rows differ', n;
  END IF;

  -- moml.legal_treatises_metadata, less the four orphans that stay in the archive
  SELECT count(*) INTO n FROM (
    (SELECT * FROM moml.legal_treatises_metadata
     WHERE psmid <> ALL (ARRAY['19000513300', '20002284100', '20002284400', '20002284500'])
     EXCEPT ALL
     SELECT v.psmid, coalesce(e.author, ''), btrim(v.full_title),
            CASE WHEN v.gale_id IS NULL THEN v.edition_statement
                 ELSE coalesce(v.edition_statement, '') END,
            CASE WHEN v.current_volume <> 0 THEN v.current_volume::text
                 WHEN v.gale_id IS NULL THEN '1'
                 ELSE '' END,
            v.imprint, e.book_collation, v.total_pages::text
     FROM moml.volumes v JOIN moml.editions e USING (bibliographicid))
    UNION ALL
    (SELECT v.psmid, coalesce(e.author, ''), btrim(v.full_title),
            CASE WHEN v.gale_id IS NULL THEN v.edition_statement
                 ELSE coalesce(v.edition_statement, '') END,
            CASE WHEN v.current_volume <> 0 THEN v.current_volume::text
                 WHEN v.gale_id IS NULL THEN '1'
                 ELSE '' END,
            v.imprint, e.book_collation, v.total_pages::text
     FROM moml.volumes v JOIN moml.editions e USING (bibliographicid)
     EXCEPT ALL
     SELECT * FROM moml.legal_treatises_metadata
     WHERE psmid <> ALL (ARRAY['19000513300', '20002284100', '20002284400', '20002284500']))) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.legal_treatises_metadata cannot be rebuilt from the new tables: % rows differ', n;
  END IF;

  -- moml.book_subject
  SELECT count(*) INTO n FROM (
    (SELECT * FROM moml.book_subject
     EXCEPT ALL
     SELECT v.psmid, s.subject, s.source
     FROM moml.volumes v JOIN moml.edition_subjects s USING (bibliographicid))
    UNION ALL
    (SELECT v.psmid, s.subject, s.source
     FROM moml.volumes v JOIN moml.edition_subjects s USING (bibliographicid)
     EXCEPT ALL
     SELECT * FROM moml.book_subject)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.book_subject cannot be rebuilt from the new tables: % rows differ', n;
  END IF;

  -- moml.book_locsubjecthead
  SELECT count(*) INTO n FROM (
    (SELECT * FROM moml.book_locsubjecthead
     EXCEPT ALL
     SELECT v.psmid, s.type, s.subfield, s.locsubject
     FROM moml.volumes v JOIN moml.edition_loc_subjects s USING (bibliographicid))
    UNION ALL
    (SELECT v.psmid, s.type, s.subfield, s.locsubject
     FROM moml.volumes v JOIN moml.edition_loc_subjects s USING (bibliographicid)
     EXCEPT ALL
     SELECT * FROM moml.book_locsubjecthead)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.book_locsubjecthead cannot be rebuilt from the new tables: % rows differ', n;
  END IF;

  -- moml.book_volumeset
  SELECT count(*) INTO n FROM (
    (SELECT * FROM moml.book_volumeset
     EXCEPT ALL
     SELECT vs.psmid, vs.sibling_psmid, sib.asset_id, vs.filmed_volume::text
     FROM moml.volume_sets vs JOIN moml.volumes sib ON sib.psmid = vs.sibling_psmid)
    UNION ALL
    (SELECT vs.psmid, vs.sibling_psmid, sib.asset_id, vs.filmed_volume::text
     FROM moml.volume_sets vs JOIN moml.volumes sib ON sib.psmid = vs.sibling_psmid
     EXCEPT ALL
     SELECT * FROM moml.book_volumeset)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.book_volumeset cannot be rebuilt from the new tables: % rows differ', n;
  END IF;

  -- The order of each volume's subjects survives in position.
  SELECT count(*) INTO n FROM (
    SELECT s.psmid FROM moml.book_subject s
    GROUP BY s.psmid
    HAVING array_agg(ROW(s.subject, s.source)::text ORDER BY s.ctid)
           IS DISTINCT FROM
           (SELECT array_agg(ROW(es.subject, es.source)::text ORDER BY es.position)
            FROM moml.volumes v JOIN moml.edition_subjects es USING (bibliographicid)
            WHERE v.psmid = s.psmid)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'the subject order of % volumes is not preserved', n;
  END IF;

  -- The LoC order is best effort; report how many volumes differ from it.
  SELECT count(*) INTO n FROM (
    SELECT s.psmid FROM moml.book_locsubjecthead s
    GROUP BY s.psmid
    HAVING array_agg(ROW(s.type, s.subfield, s.locsubject)::text ORDER BY s.ctid)
           IS DISTINCT FROM
           (SELECT array_agg(ROW(el.type, el.subfield, el.locsubject)::text ORDER BY el.position)
            FROM moml.volumes v JOIN moml.edition_loc_subjects el USING (bibliographicid)
            WHERE v.psmid = s.psmid)) d;
  RAISE NOTICE 'LoC subject headings: % volumes have rows in a physical order other than their edition''s', n;
END $$;

-- Redefine moml.treatises over the new tables, with the same output.
CREATE OR REPLACE VIEW moml.treatises AS
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

DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n FROM (
    (SELECT * FROM treatises_before EXCEPT ALL SELECT * FROM moml.treatises)
    UNION ALL
    (SELECT * FROM moml.treatises EXCEPT ALL SELECT * FROM treatises_before)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.treatises changed: % rows differ', n;
  END IF;

  SELECT count(*) INTO n FROM (
    (SELECT * FROM us_treatises_before EXCEPT ALL SELECT * FROM moml.us_treatises)
    UNION ALL
    (SELECT * FROM moml.us_treatises EXCEPT ALL SELECT * FROM us_treatises_before)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.us_treatises changed: % rows differ', n;
  END IF;
END $$;

-- Point the foreign keys at moml.volumes.
ALTER TABLE moml.page DROP CONSTRAINT IF EXISTS page_psmid_fkey;
ALTER TABLE moml.page
  ADD CONSTRAINT page_psmid_fkey FOREIGN KEY (psmid) REFERENCES moml.volumes (psmid);

ALTER TABLE legalhist.textbooks_vols DROP CONSTRAINT IF EXISTS textbooks_psmid_fkey;
ALTER TABLE legalhist.textbooks_vols
  ADD CONSTRAINT textbooks_psmid_fkey FOREIGN KEY (psmid) REFERENCES moml.volumes (psmid);

ALTER TABLE moml.page_ocrtext DROP CONSTRAINT IF EXISTS page_ocrtext_psmid_fkey;

-- Move the old tables into the archive, unchanged, and make them read-only
-- for law_dev.
ALTER TABLE IF EXISTS moml.book_volumeset SET SCHEMA moml_archive;
ALTER TABLE IF EXISTS moml.book_locsubjecthead SET SCHEMA moml_archive;
ALTER TABLE IF EXISTS moml.book_subject SET SCHEMA moml_archive;
ALTER TABLE IF EXISTS moml.book_citation SET SCHEMA moml_archive;
ALTER TABLE IF EXISTS moml.legal_treatises_metadata SET SCHEMA moml_archive;
ALTER TABLE IF EXISTS moml.book_info SET SCHEMA moml_archive;

REVOKE INSERT, UPDATE, DELETE, TRUNCATE
    ON moml_archive.book_info, moml_archive.book_citation,
       moml_archive.legal_treatises_metadata, moml_archive.book_subject,
       moml_archive.book_locsubjecthead, moml_archive.book_volumeset
  FROM law_dev;

-- Nothing outside the archive may depend on it.
DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n
  FROM pg_constraint c
  JOIN pg_class r ON r.oid = c.conrelid
  JOIN pg_class f ON f.oid = c.confrelid
  WHERE c.contype = 'f'
    AND f.relnamespace = 'moml_archive'::regnamespace
    AND r.relnamespace <> 'moml_archive'::regnamespace;
  IF n <> 0 THEN
    RAISE EXCEPTION '% foreign keys outside moml_archive reference it', n;
  END IF;

  SELECT count(*) INTO n
  FROM pg_depend d
  JOIN pg_rewrite w ON w.oid = d.objid
  JOIN pg_class v ON v.oid = w.ev_class
  JOIN pg_class t ON t.oid = d.refobjid
  WHERE d.classid = 'pg_rewrite'::regclass
    AND t.relnamespace = 'moml_archive'::regnamespace
    AND v.relnamespace <> 'moml_archive'::regnamespace;
  IF n <> 0 THEN
    RAISE EXCEPTION '% views outside moml_archive read from it', n;
  END IF;
END $$;

ANALYZE moml.editions, moml.volumes, moml.edition_subjects,
        moml.edition_loc_subjects, moml.volume_sets;

-- migrate:down
SET ROLE = law_admin;

-- Put the original tables back. The new tables are dropped, so first check
-- that they still hold exactly what the archive holds: anything written to
-- them since the up migration would be lost, and must be moved first.
DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n FROM (
    (SELECT * FROM moml_archive.book_info
     EXCEPT ALL
     SELECT v.psmid, e.content_type, v.gale_id, e.faid, e.colid,
            v.ocr_confidence::text, v.asset_id, v.asset_id_etoc,
            e.dvi_collection_id, v.bibliographicid, e.bibliographicid_type,
            e.unit, v.fiche_range, e.mcode, NULL::text, v.pubdate_composed,
            v.pubdate_start, e.release_date, v.source_library,
            v.source_library_location, e.language, e.language_ocr,
            e.language_primary, e.document_type, NULL::text, e.category_code,
            e.category_code_source, v.product_link, v.webid, v.year
     FROM moml.volumes v JOIN moml.editions e USING (bibliographicid))
    UNION ALL
    (SELECT v.psmid, e.content_type, v.gale_id, e.faid, e.colid,
            v.ocr_confidence::text, v.asset_id, v.asset_id_etoc,
            e.dvi_collection_id, v.bibliographicid, e.bibliographicid_type,
            e.unit, v.fiche_range, e.mcode, NULL::text, v.pubdate_composed,
            v.pubdate_start, e.release_date, v.source_library,
            v.source_library_location, e.language, e.language_ocr,
            e.language_primary, e.document_type, NULL::text, e.category_code,
            e.category_code_source, v.product_link, v.webid, v.year
     FROM moml.volumes v JOIN moml.editions e USING (bibliographicid)
     EXCEPT ALL
     SELECT * FROM moml_archive.book_info)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.volumes and moml.editions have changed since the migration (% rows differ from the archive); move those changes before rolling back', n;
  END IF;

  SELECT count(*) INTO n FROM (
    (SELECT psmid, subject, source FROM moml_archive.book_subject
     EXCEPT ALL
     SELECT v.psmid, s.subject, s.source
     FROM moml.volumes v JOIN moml.edition_subjects s USING (bibliographicid))
    UNION ALL
    (SELECT v.psmid, s.subject, s.source
     FROM moml.volumes v JOIN moml.edition_subjects s USING (bibliographicid)
     EXCEPT ALL
     SELECT psmid, subject, source FROM moml_archive.book_subject)) d;
  IF n <> 0 THEN
    RAISE EXCEPTION 'moml.edition_subjects has changed since the migration (% rows differ from the archive)', n;
  END IF;

  IF (SELECT count(*) FROM moml.edition_loc_subjects el JOIN moml.volumes v USING (bibliographicid))
     <> (SELECT count(*) FROM moml_archive.book_locsubjecthead)
     OR (SELECT count(*) FROM moml.volume_sets)
     <> (SELECT count(*) FROM moml_archive.book_volumeset) THEN
    RAISE EXCEPTION 'moml.edition_loc_subjects or moml.volume_sets has changed since the migration';
  END IF;
END $$;

GRANT INSERT, UPDATE, DELETE, TRUNCATE
   ON moml_archive.book_info, moml_archive.book_citation,
      moml_archive.legal_treatises_metadata, moml_archive.book_subject,
      moml_archive.book_locsubjecthead, moml_archive.book_volumeset
   TO law_dev;

ALTER TABLE IF EXISTS moml_archive.book_info SET SCHEMA moml;
ALTER TABLE IF EXISTS moml_archive.legal_treatises_metadata SET SCHEMA moml;
ALTER TABLE IF EXISTS moml_archive.book_citation SET SCHEMA moml;
ALTER TABLE IF EXISTS moml_archive.book_subject SET SCHEMA moml;
ALTER TABLE IF EXISTS moml_archive.book_locsubjecthead SET SCHEMA moml;
ALTER TABLE IF EXISTS moml_archive.book_volumeset SET SCHEMA moml;

CREATE OR REPLACE VIEW moml.treatises AS
 SELECT bi.bibliographicid,
    min(bi.year) AS year,
    (array_agg(DISTINCT bc.displaytitle))[1] AS title,
        CASE
            WHEN (max((bc.currentvolume)::integer) = 0) THEN 1
            ELSE max((bc.currentvolume)::integer)
        END AS vols,
    array_agg(DISTINCT bs.subject) AS subjects,
    array_agg(DISTINCT bi.psmid) AS psmid
   FROM ((moml.book_info bi
     LEFT JOIN moml.book_citation bc ON (((bi.psmid)::text = (bc.psmid)::text)))
     LEFT JOIN moml.book_subject bs ON (((bi.psmid)::text = (bs.psmid)::text)))
  GROUP BY bi.bibliographicid
  ORDER BY (min(bi.year)), ((array_agg(DISTINCT bc.displaytitle))[1]);

ALTER TABLE moml.page DROP CONSTRAINT IF EXISTS page_psmid_fkey;
ALTER TABLE moml.page
  ADD CONSTRAINT page_psmid_fkey FOREIGN KEY (psmid) REFERENCES moml.book_info (psmid);

ALTER TABLE legalhist.textbooks_vols DROP CONSTRAINT IF EXISTS textbooks_psmid_fkey;
ALTER TABLE legalhist.textbooks_vols
  ADD CONSTRAINT textbooks_psmid_fkey FOREIGN KEY (psmid) REFERENCES moml.book_info (psmid);

-- Validating this key reads all of moml.page_ocrtext and takes several minutes.
ALTER TABLE moml.page_ocrtext DROP CONSTRAINT IF EXISTS page_ocrtext_psmid_fkey;
ALTER TABLE moml.page_ocrtext
  ADD CONSTRAINT page_ocrtext_psmid_fkey FOREIGN KEY (psmid)
  REFERENCES moml.legal_treatises_metadata (psmid);

DROP TABLE IF EXISTS moml.volume_sets;
DROP TABLE IF EXISTS moml.edition_loc_subjects;
DROP TABLE IF EXISTS moml.edition_subjects;
DROP TABLE IF EXISTS moml.volumes;
DROP TABLE IF EXISTS moml.editions;

DROP SCHEMA IF EXISTS moml_archive;
