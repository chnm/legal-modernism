-- migrate:up
SET ROLE = law_admin;

-- Keep one copy of each edition MOML scanned twice (issue #142).
--
-- Gale sometimes catalogued one edition under two bibliographicids and
-- scanned a copy for each, often from different libraries (Harvard's and
-- Yale's), and five of its Part II records hold two scans of one book. Both
-- copies were OCRed and run through the citation detector, so the citations
-- in them were counted twice: Kent's Commentaries of 1884 had some 34,000 in
-- each copy, Parsons on Contracts was doubled in both its 1864 and its 1893
-- printing, and in any network of treatises each pair looks like the
-- strongest coupling there is.
--
-- The rule, which db/queries/duplicate-copies.sql states and reproduces: two
-- Gale editions are twin editions when their full title, author, imprint and
-- edition statement are equal once lower-cased and stripped of everything but
-- letters and digits, and their total page counts are within 5% of each
-- other; two volumes of one edition are same-edition copies when they have the
-- same volume number and page counts within 5%. Of each pair the copy with
-- more linked citations is kept (for twins, the whole edition, counted over
-- its volumes), as measured on 2026-09-25; a tie, mostly between pamphlets
-- with no links, goes to the higher mean OCR confidence and then to the lower
-- bibliographicid or psmid.
--
-- That is 46 twin editions and 4 same-edition copies: 61 volumes and 46
-- editions are removed, with 26,157 pages and the 141,157 citations detected
-- in them, 71,194 of which were linked. Eight near-duplicates fail the page
-- test and are left alone for review: CTRG95-B2468/CTRG96-B840,
-- ocm11955467/ocm12569264, ocm15927110/ocm15927138, ocm18247383/ocm32077362,
-- ocm21320099/ocm32920876, ocm31703162/ocm31703193, and within CTRG97-B240
-- and ocm32147515 (whose two "vol. 2" copies are in fact volumes 1 and 2).
--
-- The decisions are listed below rather than computed here, so that the
-- migration does the same thing whenever it runs, and they are kept in
-- moml.duplicate_copies: each removed volume, the volume kept in its place,
-- and the linked-citation counts the choice rested on. The migration first
-- checks that every pair still meets the rule.
--
-- Nothing is deleted without a copy. Every removed row -- the volumes and
-- editions and their subjects and volume sets, the pages, OCR text and section
-- headers, and the detected citations and their links -- is copied into a
-- moml_archive.removed_* table first, and the down migration puts it back.
-- The 28 rows of legalhist.textbooks_vols that pointed at a removed copy are
-- repointed to the kept one (psmid, bibliographicid and webid); the rows as
-- they were are in moml_archive.removed_textbooks_vols.
--
-- The removed citations are derived data: a rerun of the detector would not
-- find them, since their pages are gone. The linker's materialized views and
-- legalhist.all_page_id are stale until make db-maintenance refreshes them.

CREATE TABLE IF NOT EXISTS moml.duplicate_copies (
    removed_psmid           text PRIMARY KEY,
    removed_bibliographicid text NOT NULL,
    kept_bibliographicid    text NOT NULL REFERENCES moml.editions (bibliographicid),
    kept_psmid              text NOT NULL REFERENCES moml.volumes (psmid),
    kind                    text NOT NULL CHECK (kind IN ('twin_edition', 'same_edition')),
    removed_linked          integer NOT NULL,
    kept_linked             integer NOT NULL,
    CHECK (removed_psmid <> kept_psmid)
);

COMMENT ON TABLE moml.duplicate_copies IS
  'Volumes removed because MOML held a second copy of the same edition (issue #142), with the volume kept in their place; the removed data is in moml_archive.removed_*';
COMMENT ON COLUMN moml.duplicate_copies.kind IS
  'twin_edition: the edition was catalogued twice and the whole removed edition is gone; same_edition: a second scan of one volume within an edition';
COMMENT ON COLUMN moml.duplicate_copies.removed_linked IS
  'Linked citations in the removed copy on 2026-09-25 (for a twin edition, counted over the whole edition)';

GRANT SELECT ON moml.duplicate_copies TO law_service;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON moml.duplicate_copies TO law_dev;

INSERT INTO moml.duplicate_copies
  (kind, removed_psmid, removed_bibliographicid, kept_bibliographicid, kept_psmid,
   removed_linked, kept_linked)
VALUES
  ('twin_edition', '20000076700', 'CTRG95-B1331', 'ocm12016409', '19000083800', 1058, 1295),
  ('twin_edition', '20000106100', 'CTRG95-B1372', 'ocm12779527', '19000506300', 207, 229),
  ('twin_edition', '20000741200', 'CTRG95-B2194', 'ocm17199031', '19002014000', 3413, 4280),
  ('twin_edition', '20002253600', 'CTRG95-B2398', 'CTRG95-B2417', '20000765200', 1043, 1142),
  ('twin_edition', '20001080500', 'CTRG95-B2547', 'ocm18417908', '19002994800', 760, 872),
  ('twin_edition', '20001094900', 'CTRG95-B2863', 'CTRG96-B841', '20000545800', 534, 543),
  ('twin_edition', '20001110600', 'CTRG95-B2864', 'CTRG95-B4109', '20000549200', 45, 56),
  ('twin_edition', '20000157600', 'CTRG95-B2928', 'CTRG95-B2914', '20000148300', 1912, 1935),
  ('twin_edition', '20000899600', 'CTRG95-B3092', 'ocm20625186', '19003595200', 578, 1578),
  ('twin_edition', '20000176600', 'CTRG95-B3383', 'ocm12396212', '19000152100', 0, 0),
  ('twin_edition', '20000190200', 'CTRG95-B3415', 'ocm13015547', '19000604800', 0, 0),
  ('twin_edition', '20001063900', 'CTRG95-B4237', 'CTRG96-B844', '20000547100', 431, 448),
  ('twin_edition', '20001046300', 'CTRG95-B4240', 'CTRG96-B839', '20000545000', 2, 2),
  ('twin_edition', '20001046600', 'CTRG95-B4241', 'CTRG96-B845', '20000547400', 74, 75),
  ('twin_edition', '20001087900', 'CTRG95-B4650', 'CTRG97-B382', '20002128600', 0, 0),
  ('twin_edition', '20000612100', 'CTRG95-B4659', 'CTRG95-B4247', '20001034900', 1461, 1484),
  ('twin_edition', '20002097400', 'CTRG96-B3380', 'ocm17367663', '19002145600', 1, 2),
  ('twin_edition', '20000214600', 'CTRG96-B373', 'ocm20592318', '19003607400', 1521, 1584),
  ('twin_edition', '20000546300', 'CTRG96-B842', 'CTRG95-B4248', '20001035200', 106, 107),
  ('twin_edition', '20000546600', 'CTRG96-B843', 'CTRG95-B2874', '20001032700', 425, 438),
  ('twin_edition', '20000547800', 'CTRG96-B846', 'CTRG95-B4249', '20001064200', 32, 35),
  ('twin_edition', '20003083000', 'CTRG98-B971', 'CTRG96-B1823', '20001630500', 1531, 1793),
  ('twin_edition', '20003823000', 'CTRG99-B296', 'CTRG00-B1516', '20004381800', 0, 0),
  ('twin_edition', '19000099300', 'ocm11978497', 'ocm12137112', '19000099400', 0, 0),
  ('twin_edition', '19000228800', 'ocm12144576', 'CTRG95-B2851', '20000144400', 0, 0),
  ('twin_edition', '19001603801', 'ocm15200524', 'ocm17162725', '19002006801', 15339, 19658),
  ('twin_edition', '19001603802', 'ocm15200524', 'ocm17162725', '19002006802', 15339, 19658),
  ('twin_edition', '19001603803', 'ocm15200524', 'ocm17162725', '19002006803', 15339, 19658),
  ('twin_edition', '19001565201', 'ocm15369283', 'ocm17162712', '19002062601', 3155, 27109),
  ('twin_edition', '19001565202', 'ocm15369283', 'ocm17162712', '19002062602', 3155, 27109),
  ('twin_edition', '19001565203', 'ocm15369283', 'ocm17162712', '19002062603', 3155, 27109),
  ('twin_edition', '19002226400', 'ocm17355880', 'ocm17849074', '19002515300', 1107, 1398),
  ('twin_edition', '19002562500', 'ocm17799477', 'ocm31615819', '19006377000', 2, 2),
  ('twin_edition', '19003051100', 'ocm18593070', 'ocm31620672', '19006289400', 2, 2),
  ('twin_edition', '19003002301', 'ocm18622383', 'ocm17794831', '19002536200', 10597, 11662),
  ('twin_edition', '19003002302', 'ocm18622383', 'ocm17794831', '19002536200', 10597, 11662),
  ('twin_edition', '19003048400', 'ocm18708092', 'ocm31529970', '19006368700', 0, 0),
  ('twin_edition', '19003633000', 'ocm21051737', 'ocm25420935', '19005199700', 0, 0),
  ('twin_edition', '19003853600', 'ocm21284396', 'ocm32507697', '19007984700', 211, 213),
  ('twin_edition', '19004019900', 'ocm21982247', 'ocm31529653', '19006368400', 0, 0),
  ('twin_edition', '19004423300', 'ocm22915702', 'ocm31809572', '19006625600', 7, 7),
  ('twin_edition', '19004488400', 'ocm23019863', 'ocm32777315', '19008119900', 25, 28),
  ('twin_edition', '19004709100', 'ocm23259232', 'ocm23094500', '19004532400', 1, 2),
  ('twin_edition', '19004747000', 'ocm23376795', 'ocm24358059', '19005104800', 1207, 1441),
  ('twin_edition', '19005252100', 'ocm25295625', 'ocm21132033', '19003751200', 0, 0),
  ('twin_edition', '19005463500', 'ocm26482606', 'ocm16929475', '19001940800', 1858, 2078),
  ('twin_edition', '19005563600', 'ocm26716818', 'ocm26716844', '19005563400', 0, 0),
  ('twin_edition', '19006732901', 'ocm31808567', 'ocm12102318', '19000260101', 418, 458),
  ('twin_edition', '19006732902', 'ocm31808567', 'ocm12102318', '19000260102', 418, 458),
  ('twin_edition', '19006732903', 'ocm31808567', 'ocm12102318', '19000260103', 418, 458),
  ('twin_edition', '19006732904', 'ocm31808567', 'ocm12102318', '19000260104', 418, 458),
  ('twin_edition', '19006551200', 'ocm31849454', 'ocm16666114', '19001985900', 2, 2),
  ('twin_edition', '19006951500', 'ocm32007957', 'ocm32832553', '19008351600', 951, 995),
  ('twin_edition', '19007338901', 'ocm32022321', 'ocm12090144', '19000050901', 20632, 22307),
  ('twin_edition', '19007338902', 'ocm32022321', 'ocm12090144', '19000050902', 20632, 22307),
  ('twin_edition', '19007338903', 'ocm32022321', 'ocm12090144', '19000050903', 20632, 22307),
  ('twin_edition', '19007338904', 'ocm32022321', 'ocm12090144', '19000050904', 20632, 22307),
  ('same_edition', '20003154600', 'CTRG97-B1237', 'CTRG97-B1237', '20002567500', 195, 1083),
  ('same_edition', '20003169200', 'CTRG97-B1238', 'CTRG97-B1238', '20002574100', 292, 329),
  ('same_edition', '20002573000', 'CTRG97-B1275', 'CTRG97-B1275', '20002328500', 41, 46),
  ('same_edition', '20003175700', 'CTRG97-B2489', 'CTRG97-B2489', '20002565400', 18, 21);

-- Check that every pair still meets the rule, and that the list is whole.
DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n
  FROM moml.duplicate_copies d
  LEFT JOIN moml.volumes rv ON rv.psmid = d.removed_psmid
  JOIN moml.volumes kv ON kv.psmid = d.kept_psmid
  WHERE rv.psmid IS NULL
     OR rv.bibliographicid <> d.removed_bibliographicid
     OR kv.bibliographicid <> d.kept_bibliographicid
     OR rv.gale_id IS NULL OR kv.gale_id IS NULL;
  IF n <> 0 THEN
    RAISE EXCEPTION '% duplicate_copies rows do not match moml.volumes', n;
  END IF;

  IF EXISTS (SELECT 1 FROM moml.duplicate_copies d
             JOIN moml.duplicate_copies k ON k.removed_psmid = d.kept_psmid) THEN
    RAISE EXCEPTION 'a kept volume is also listed for removal';
  END IF;

  -- A twin edition goes whole, and its partner keeps all of its volumes.
  SELECT count(*) INTO n
  FROM moml.volumes v
  WHERE v.bibliographicid IN (SELECT removed_bibliographicid FROM moml.duplicate_copies
                              WHERE kind = 'twin_edition')
    AND v.psmid NOT IN (SELECT removed_psmid FROM moml.duplicate_copies);
  IF n <> 0 THEN
    RAISE EXCEPTION '% volumes of a removed twin edition are not listed for removal', n;
  END IF;

  -- Twin editions: equal keys, and pages within 5%.
  SELECT count(*) INTO n FROM (
    WITH edition AS (
      SELECT v.bibliographicid,
             regexp_replace(lower(min(v.full_title)), '[^a-z0-9]+', '', 'g') AS title_key,
             regexp_replace(lower(coalesce(min(e.author), '')), '[^a-z]+', '', 'g') AS author_key,
             regexp_replace(lower(min(v.imprint)), '[^a-z0-9]+', '', 'g') AS imprint_key,
             regexp_replace(lower(coalesce(min(v.edition_statement), '')), '[^a-z0-9]+', '', 'g') AS edition_key,
             sum(v.total_pages) AS pages
      FROM moml.volumes v JOIN moml.editions e USING (bibliographicid)
      GROUP BY v.bibliographicid)
    SELECT DISTINCT d.removed_bibliographicid
    FROM moml.duplicate_copies d
    JOIN edition r ON r.bibliographicid = d.removed_bibliographicid
    JOIN edition k ON k.bibliographicid = d.kept_bibliographicid
    WHERE d.kind = 'twin_edition'
      AND ((r.title_key, r.author_key, r.imprint_key, r.edition_key)
           IS DISTINCT FROM (k.title_key, k.author_key, k.imprint_key, k.edition_key)
           OR abs(r.pages - k.pages)::numeric / greatest(r.pages, k.pages) > 0.05)) bad;
  IF n <> 0 THEN
    RAISE EXCEPTION '% twin-edition pairs no longer meet the rule', n;
  END IF;

  -- Same-edition copies: one edition, one volume number, pages within 5%.
  SELECT count(*) INTO n
  FROM moml.duplicate_copies d
  JOIN moml.volumes r ON r.psmid = d.removed_psmid
  JOIN moml.volumes k ON k.psmid = d.kept_psmid
  WHERE d.kind = 'same_edition'
    AND (r.bibliographicid <> k.bibliographicid
         OR r.current_volume <> k.current_volume
         OR abs(r.total_pages - k.total_pages)::numeric / greatest(r.total_pages, k.total_pages) > 0.05);
  IF n <> 0 THEN
    RAISE EXCEPTION '% same-edition pairs no longer meet the rule', n;
  END IF;

  -- The textbook rows to repoint agree with the volume they point at.
  SELECT count(*) INTO n
  FROM legalhist.textbooks_vols t
  JOIN moml.volumes v ON v.psmid = t.psmid
  WHERE t.psmid IN (SELECT removed_psmid FROM moml.duplicate_copies)
    AND (t.bibliographicid IS DISTINCT FROM v.bibliographicid
         OR t.webid IS DISTINCT FROM v.webid);
  IF n <> 0 THEN
    RAISE EXCEPTION '% textbooks_vols rows disagree with the volume they reference', n;
  END IF;

  IF (SELECT count(*) FROM moml.duplicate_copies) <> 61
     OR (SELECT count(DISTINCT removed_bibliographicid) FROM moml.duplicate_copies
         WHERE kind = 'twin_edition') <> 46 THEN
    RAISE EXCEPTION 'expected 61 volumes and 46 twin editions in moml.duplicate_copies';
  END IF;
END $$;

-- Deleting a page makes PostgreSQL look for the rows of moml.page_content
-- that reference it, and deleting a volume the rows of moml.page. Neither table
-- had an index to find them by, so each check read the whole table: about 0.2
-- seconds a page and 2 seconds a volume against the live tables, which for
-- 26,157 pages would have held the migration for hours. These two indexes
-- serve those foreign keys, and they stay.
CREATE INDEX IF NOT EXISTS page_content_psmid_pageid_idx ON moml.page_content (psmid, pageid);
CREATE INDEX IF NOT EXISTS page_psmid_idx ON moml.page (psmid);

-- Archive tables for everything removed.
CREATE TABLE IF NOT EXISTS moml_archive.removed_editions (LIKE moml.editions);
CREATE TABLE IF NOT EXISTS moml_archive.removed_volumes (LIKE moml.volumes);
CREATE TABLE IF NOT EXISTS moml_archive.removed_edition_subjects (LIKE moml.edition_subjects);
CREATE TABLE IF NOT EXISTS moml_archive.removed_edition_loc_subjects (LIKE moml.edition_loc_subjects);
CREATE TABLE IF NOT EXISTS moml_archive.removed_volume_sets (LIKE moml.volume_sets);
CREATE TABLE IF NOT EXISTS moml_archive.removed_page (LIKE moml.page);
CREATE TABLE IF NOT EXISTS moml_archive.removed_page_ocrtext (LIKE moml.page_ocrtext);
CREATE TABLE IF NOT EXISTS moml_archive.removed_page_content (LIKE moml.page_content);
CREATE TABLE IF NOT EXISTS moml_archive.removed_citations_unlinked (LIKE moml_citations.citations_unlinked);
CREATE TABLE IF NOT EXISTS moml_archive.removed_citation_links (LIKE moml_citations.citation_links);
CREATE TABLE IF NOT EXISTS moml_archive.removed_textbooks_vols (LIKE legalhist.textbooks_vols);

GRANT SELECT
   ON moml_archive.removed_editions, moml_archive.removed_volumes,
      moml_archive.removed_edition_subjects, moml_archive.removed_edition_loc_subjects,
      moml_archive.removed_volume_sets, moml_archive.removed_page,
      moml_archive.removed_page_ocrtext, moml_archive.removed_page_content,
      moml_archive.removed_citations_unlinked, moml_archive.removed_citation_links,
      moml_archive.removed_textbooks_vols
   TO law_service, law_dev;

-- Citations and their links.
INSERT INTO moml_archive.removed_citation_links
SELECT cl.*
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
WHERE cu.moml_treatise IN (SELECT removed_psmid FROM moml.duplicate_copies);

DELETE FROM moml_citations.citation_links cl
USING moml_citations.citations_unlinked cu
WHERE cu.id = cl.citation_id
  AND cu.moml_treatise IN (SELECT removed_psmid FROM moml.duplicate_copies);

INSERT INTO moml_archive.removed_citations_unlinked
SELECT * FROM moml_citations.citations_unlinked
WHERE moml_treatise IN (SELECT removed_psmid FROM moml.duplicate_copies);

DELETE FROM moml_citations.citations_unlinked
WHERE moml_treatise IN (SELECT removed_psmid FROM moml.duplicate_copies);

-- Pages, their text and their section headers.
INSERT INTO moml_archive.removed_page_content
SELECT * FROM moml.page_content
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

DELETE FROM moml.page_content
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

INSERT INTO moml_archive.removed_page_ocrtext
SELECT * FROM moml.page_ocrtext
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

DELETE FROM moml.page_ocrtext
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

INSERT INTO moml_archive.removed_page
SELECT * FROM moml.page
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

DELETE FROM moml.page
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

-- Textbook adoptions move to the kept copy.
INSERT INTO moml_archive.removed_textbooks_vols
SELECT * FROM legalhist.textbooks_vols
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

UPDATE legalhist.textbooks_vols t
SET psmid = d.kept_psmid,
    bibliographicid = d.kept_bibliographicid,
    webid = kv.webid
FROM moml.duplicate_copies d
JOIN moml.volumes kv ON kv.psmid = d.kept_psmid
WHERE t.psmid = d.removed_psmid;

-- Volume sets and volumes.
INSERT INTO moml_archive.removed_volume_sets
SELECT * FROM moml.volume_sets
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies)
   OR sibling_psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

DELETE FROM moml.volume_sets
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies)
   OR sibling_psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

INSERT INTO moml_archive.removed_volumes
SELECT * FROM moml.volumes
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

DELETE FROM moml.volumes
WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies);

-- Editions left without volumes, with their subjects.
INSERT INTO moml_archive.removed_edition_subjects
SELECT * FROM moml.edition_subjects s
WHERE s.bibliographicid IN (SELECT removed_bibliographicid FROM moml.duplicate_copies
                            WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = s.bibliographicid);

DELETE FROM moml.edition_subjects s
WHERE s.bibliographicid IN (SELECT removed_bibliographicid FROM moml.duplicate_copies
                            WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = s.bibliographicid);

INSERT INTO moml_archive.removed_edition_loc_subjects
SELECT * FROM moml.edition_loc_subjects s
WHERE s.bibliographicid IN (SELECT removed_bibliographicid FROM moml.duplicate_copies
                            WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = s.bibliographicid);

DELETE FROM moml.edition_loc_subjects s
WHERE s.bibliographicid IN (SELECT removed_bibliographicid FROM moml.duplicate_copies
                            WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = s.bibliographicid);

INSERT INTO moml_archive.removed_editions
SELECT * FROM moml.editions e
WHERE e.bibliographicid IN (SELECT removed_bibliographicid FROM moml.duplicate_copies
                            WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = e.bibliographicid);

DELETE FROM moml.editions e
WHERE e.bibliographicid IN (SELECT removed_bibliographicid FROM moml.duplicate_copies
                            WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = e.bibliographicid);

DO $$
DECLARE
  n_volumes bigint;
  n_editions bigint;
BEGIN
  SELECT count(*) INTO n_volumes FROM moml_archive.removed_volumes;
  SELECT count(*) INTO n_editions FROM moml_archive.removed_editions;
  IF n_volumes <> 61 OR n_editions <> 46 THEN
    RAISE EXCEPTION 'expected to remove 61 volumes and 46 editions, removed % and %',
      n_volumes, n_editions;
  END IF;
  IF EXISTS (SELECT 1 FROM moml.duplicate_copies d
             JOIN moml_archive.removed_editions e ON e.bibliographicid = d.removed_bibliographicid
             WHERE d.kind = 'same_edition') THEN
    RAISE EXCEPTION 'an edition with a same-edition copy was removed';
  END IF;
  RAISE NOTICE 'removed % pages, % citations and % links; repointed % textbook rows',
    (SELECT count(*) FROM moml_archive.removed_page),
    (SELECT count(*) FROM moml_archive.removed_citations_unlinked),
    (SELECT count(*) FROM moml_archive.removed_citation_links),
    (SELECT count(*) FROM moml_archive.removed_textbooks_vols);
END $$;

-- migrate:down
SET ROLE = law_admin;

-- Put the removed copies back, and return the textbook rows to them.
INSERT INTO moml.editions SELECT * FROM moml_archive.removed_editions;
INSERT INTO moml.volumes SELECT * FROM moml_archive.removed_volumes;
INSERT INTO moml.edition_subjects SELECT * FROM moml_archive.removed_edition_subjects;
INSERT INTO moml.edition_loc_subjects SELECT * FROM moml_archive.removed_edition_loc_subjects;
INSERT INTO moml.volume_sets SELECT * FROM moml_archive.removed_volume_sets;
INSERT INTO moml.page SELECT * FROM moml_archive.removed_page;
INSERT INTO moml.page_ocrtext SELECT * FROM moml_archive.removed_page_ocrtext;
INSERT INTO moml.page_content SELECT * FROM moml_archive.removed_page_content;
INSERT INTO moml_citations.citations_unlinked SELECT * FROM moml_archive.removed_citations_unlinked;
INSERT INTO moml_citations.citation_links SELECT * FROM moml_archive.removed_citation_links;

-- textbooks_vols has no key and can hold identical rows, so each repointed row
-- is matched by value: for every archived row, one row equal to its repointed
-- form is deleted, and the archived row is inserted in its place.
DO $$
DECLARE
  n bigint;
BEGIN
  WITH repointed AS (
    SELECT ROW(d.kept_bibliographicid, d.kept_psmid, kv.webid, a.school, a.title,
               a.edition, a.topic, a.year_begin, a.year_end, a.course, a.subtopic,
               a.school_state, a.region, a.class_year)::legalhist.textbooks_vols AS r
    FROM moml_archive.removed_textbooks_vols a
    JOIN moml.duplicate_copies d ON d.removed_psmid = a.psmid
    JOIN moml.volumes kv ON kv.psmid = d.kept_psmid),
  wanted AS (
    SELECT r, count(*) AS k FROM repointed GROUP BY r),
  current_rows AS (
    SELECT t.ctid AS row_ctid, t AS r,
           row_number() OVER (PARTITION BY t ORDER BY t.ctid) AS rn
    FROM legalhist.textbooks_vols t)
  DELETE FROM legalhist.textbooks_vols t
  USING current_rows c JOIN wanted w ON c.r = w.r
  WHERE t.ctid = c.row_ctid AND c.rn <= w.k;
  GET DIAGNOSTICS n = ROW_COUNT;
  IF n <> (SELECT count(*) FROM moml_archive.removed_textbooks_vols) THEN
    RAISE EXCEPTION 'found % of % repointed textbooks_vols rows; one changed since the migration',
      n, (SELECT count(*) FROM moml_archive.removed_textbooks_vols);
  END IF;
END $$;

INSERT INTO legalhist.textbooks_vols SELECT * FROM moml_archive.removed_textbooks_vols;

DROP TABLE IF EXISTS moml_archive.removed_textbooks_vols;
DROP TABLE IF EXISTS moml_archive.removed_citation_links;
DROP TABLE IF EXISTS moml_archive.removed_citations_unlinked;
DROP TABLE IF EXISTS moml_archive.removed_page_content;
DROP TABLE IF EXISTS moml_archive.removed_page_ocrtext;
DROP TABLE IF EXISTS moml_archive.removed_page;
DROP TABLE IF EXISTS moml_archive.removed_volume_sets;
DROP TABLE IF EXISTS moml_archive.removed_edition_loc_subjects;
DROP TABLE IF EXISTS moml_archive.removed_edition_subjects;
DROP TABLE IF EXISTS moml_archive.removed_volumes;
DROP TABLE IF EXISTS moml_archive.removed_editions;
DROP TABLE IF EXISTS moml.duplicate_copies;

DROP INDEX IF EXISTS moml.page_psmid_idx;
DROP INDEX IF EXISTS moml.page_content_psmid_pageid_idx;
