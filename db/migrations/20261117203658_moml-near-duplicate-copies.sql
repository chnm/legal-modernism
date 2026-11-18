-- migrate:up
SET ROLE = law_admin;

-- Settle the eight near-duplicate MOML copies that the duplicate rule left
-- for review (issue #334).
--
-- 20260925140200_moml-remove-duplicate-copies.sql removed the copies that
-- matched on title, author, imprint and edition statement with page counts
-- within 5% (issue #142). Eight pairs matched on everything but the page
-- count and were left alone. Kellen Funk read them, and this migration
-- carries out his decisions, which override the rule's preference for the
-- copy with more linked citations:
--
--   A general view of the law of property (1908): the same book, with more
--     advertising in the longer copy. CTRG96-B840, the shorter, is kept.
--   A practical treatise on the bankrupt law (1841): the longer copy is a
--     botched scan. ocm21320099, the shorter, is kept.
--   Principles of the law of real property (1845): the shorter copy is a
--     botched scan. ocm18247383, the longer, is kept.
--   Federal rule book (1917): of the two volumes in CTRG97-B240, the shorter
--     (20002059600) carries the rule book's metadata but is a copy of Bender's
--     Federal Revenue Law, 1916 (webid F0151687899, psmid 20002972500 in
--     CTRG97-B279), page for page. It is removed in favour of that copy,
--     and the rule book edition keeps its one true volume, 20002773800.
--   Divorce legislation (1889), Later leaves (1891) and the speech of Julien
--     Poydras (1810): unimportant either way (none is in moml.treatises).
--     One copy of each is removed under the old rule's tie-breaks (linked
--     citations, then OCR confidence), which in each case keeps the longer one.
--   The new pocket conveyancer (1810): not a duplicate. Both volumes of
--     ocm32147515 were catalogued as vol. 2, but 19007239001 is volume 1: its
--     table of contents reads "VOLUME THE FIRST", and its psmid suffix and
--     moml.volume_sets.filmed_volume agree. Its current_volume becomes 1.
--
-- The removals are recorded in moml.duplicate_copies with the kept volume and
-- a note giving the reason, and the removed rows are archived in the same
-- moml_archive.removed_* tables as before. The linked-citation counts were
-- measured on 2026-11-17. A new kind, 'miscatalogued', covers the rule book
-- volume: a volume whose record belongs to another edition, removed in favour
-- of the copy in the edition it belongs to.
--
-- As before, the materialized views over the citations are stale until
-- make db-maintenance refreshes them.

ALTER TABLE moml.duplicate_copies ADD COLUMN IF NOT EXISTS note text;

COMMENT ON COLUMN moml.duplicate_copies.note IS
  'Why a copy was removed by hand rather than by the duplicate rule (issue #334); NULL for the rule''s own decisions';
COMMENT ON COLUMN moml.duplicate_copies.removed_linked IS
  'Linked citations in the removed copy when it was removed (for a twin edition, counted over the whole edition): 2026-09-25 for the rule''s decisions, 2026-11-17 for the reviewed ones';
COMMENT ON COLUMN moml.duplicate_copies.kind IS
  'twin_edition: the edition was catalogued twice and the whole removed edition is gone; same_edition: a second scan of one volume within an edition; miscatalogued: a volume catalogued under the wrong edition, a copy of a volume of the edition it belongs to';

ALTER TABLE moml.duplicate_copies DROP CONSTRAINT IF EXISTS duplicate_copies_kind_check;
ALTER TABLE moml.duplicate_copies ADD CONSTRAINT duplicate_copies_kind_check
  CHECK (kind IN ('twin_edition', 'same_edition', 'miscatalogued'));

INSERT INTO moml.duplicate_copies
  (kind, removed_psmid, removed_bibliographicid, kept_bibliographicid, kept_psmid,
   removed_linked, kept_linked, note)
VALUES
  ('twin_edition', '20000778800', 'CTRG95-B2468', 'CTRG96-B840', '20000545300', 336, 314,
   'Reviewed (issue #334): the same book, with more advertising in the longer copy; the shorter is kept'),
  ('twin_edition', '19008349900', 'ocm32920876', 'ocm21320099', '19003832000', 868, 998,
   'Reviewed (issue #334): the longer copy is a botched scan'),
  ('twin_edition', '19007012600', 'ocm32077362', 'ocm18247383', '19002820500', 158, 75,
   'Reviewed (issue #334): the shorter copy is a botched scan'),
  ('miscatalogued', '20002059600', 'CTRG97-B240', 'CTRG97-B279', '20002972500', 1317, 1253,
   'Reviewed (issue #334): catalogued as Federal rule book, annotated, but a copy of Bender''s Federal Revenue Law, 1916 (webid F0151687899)'),
  ('twin_edition', '19000146600', 'ocm11955467', 'ocm12569264', '19000146700', 0, 0,
   'Reviewed (issue #334): unimportant either way; the longer copy is kept'),
  ('twin_edition', '19001798200', 'ocm15927110', 'ocm15927138', '19001798600', 0, 2,
   'Reviewed (issue #334): unimportant either way; the longer copy is kept'),
  ('twin_edition', '19006266000', 'ocm31703193', 'ocm31703162', '19006265800', 0, 0,
   'Reviewed (issue #334): unimportant either way; the longer copy is kept')
ON CONFLICT (removed_psmid) DO NOTHING;

-- The pocket conveyancer's first volume. Only a volume still numbered 2
-- changes, so a second run does nothing.
UPDATE moml.volumes SET current_volume = 1
WHERE psmid = '19007239001' AND current_volume = 2;

-- Check that the pairs still look as they did when they were reviewed, while
-- the removed volumes are still there to look at.
DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n
  FROM moml.duplicate_copies d
  JOIN moml.volumes rv ON rv.psmid = d.removed_psmid
  JOIN moml.volumes kv ON kv.psmid = d.kept_psmid
  WHERE d.note IS NOT NULL
    AND (rv.bibliographicid <> d.removed_bibliographicid
         OR kv.bibliographicid <> d.kept_bibliographicid
         OR rv.year IS DISTINCT FROM kv.year);
  IF n <> 0 THEN
    RAISE EXCEPTION '% reviewed duplicate_copies rows do not match moml.volumes', n;
  END IF;

  -- A twin edition here is a single-volume edition, removed whole.
  SELECT count(*) INTO n
  FROM moml.volumes v
  WHERE v.bibliographicid IN (SELECT removed_bibliographicid FROM moml.duplicate_copies
                              WHERE kind = 'twin_edition' AND note IS NOT NULL)
    AND v.psmid NOT IN (SELECT removed_psmid FROM moml.duplicate_copies);
  IF n <> 0 THEN
    RAISE EXCEPTION '% volumes of a removed twin edition are not listed for removal', n;
  END IF;

  -- Each twin pair shares its work (moml.works), and the miscatalogued
  -- volume has the page count of the copy it duplicates.
  SELECT count(*) INTO n
  FROM moml.duplicate_copies d
  JOIN moml.editions re ON re.bibliographicid = d.removed_bibliographicid
  JOIN moml.editions ke ON ke.bibliographicid = d.kept_bibliographicid
  JOIN moml.volumes rv ON rv.psmid = d.removed_psmid
  JOIN moml.volumes kv ON kv.psmid = d.kept_psmid
  WHERE d.note IS NOT NULL
    AND ((d.kind = 'twin_edition' AND re.work_id <> ke.work_id)
         OR (d.kind = 'miscatalogued' AND rv.total_pages <> kv.total_pages));
  IF n <> 0 THEN
    RAISE EXCEPTION '% reviewed pairs are no longer the pairs that were reviewed', n;
  END IF;

  IF EXISTS (SELECT 1 FROM legalhist.textbooks_vols
             WHERE psmid IN (SELECT removed_psmid FROM moml.duplicate_copies)) THEN
    RAISE EXCEPTION 'a textbooks_vols row points at a volume to be removed';
  END IF;

  IF (SELECT count(*) FROM moml.duplicate_copies WHERE note IS NOT NULL) <> 7 THEN
    RAISE EXCEPTION 'expected 7 reviewed rows in moml.duplicate_copies';
  END IF;
END $$;

-- Two of the archive tables were made before their live tables grew columns:
-- citations_unlinked gained the case name (issue #387) and editions the work
-- (issue #335). They gain the same columns, in the same order, so that rows
-- can be copied between them whole. The earlier archived rows have NULLs.
ALTER TABLE moml_archive.removed_citations_unlinked
  ADD COLUMN IF NOT EXISTS case_name text,
  ADD COLUMN IF NOT EXISTS case_name_source text;
ALTER TABLE moml_archive.removed_editions
  ADD COLUMN IF NOT EXISTS work_id integer,
  ADD COLUMN IF NOT EXISTS derivative boolean;

-- Archive and remove, in the order the foreign keys require. Only the rows
-- this migration added to duplicate_copies (note IS NOT NULL) are touched; the
-- earlier removals are long gone from the live tables.
CREATE TEMPORARY TABLE reviewed_removal ON COMMIT DROP AS
SELECT removed_psmid AS psmid, removed_bibliographicid AS bibliographicid, kind
FROM moml.duplicate_copies
WHERE note IS NOT NULL;

INSERT INTO moml_archive.removed_citation_links
SELECT cl.*
FROM moml_citations.citation_links cl
JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
WHERE cu.moml_treatise IN (SELECT psmid FROM reviewed_removal);

DELETE FROM moml_citations.citation_links cl
USING moml_citations.citations_unlinked cu
WHERE cu.id = cl.citation_id
  AND cu.moml_treatise IN (SELECT psmid FROM reviewed_removal);

INSERT INTO moml_archive.removed_citations_unlinked
SELECT * FROM moml_citations.citations_unlinked
WHERE moml_treatise IN (SELECT psmid FROM reviewed_removal);

DELETE FROM moml_citations.citations_unlinked
WHERE moml_treatise IN (SELECT psmid FROM reviewed_removal);

INSERT INTO moml_archive.removed_page_content
SELECT * FROM moml.page_content
WHERE psmid IN (SELECT psmid FROM reviewed_removal);

DELETE FROM moml.page_content
WHERE psmid IN (SELECT psmid FROM reviewed_removal);

INSERT INTO moml_archive.removed_page_ocrtext
SELECT * FROM moml.page_ocrtext
WHERE psmid IN (SELECT psmid FROM reviewed_removal);

DELETE FROM moml.page_ocrtext
WHERE psmid IN (SELECT psmid FROM reviewed_removal);

INSERT INTO moml_archive.removed_page
SELECT * FROM moml.page
WHERE psmid IN (SELECT psmid FROM reviewed_removal);

DELETE FROM moml.page
WHERE psmid IN (SELECT psmid FROM reviewed_removal);

INSERT INTO moml_archive.removed_volume_sets
SELECT * FROM moml.volume_sets
WHERE psmid IN (SELECT psmid FROM reviewed_removal)
   OR sibling_psmid IN (SELECT psmid FROM reviewed_removal);

DELETE FROM moml.volume_sets
WHERE psmid IN (SELECT psmid FROM reviewed_removal)
   OR sibling_psmid IN (SELECT psmid FROM reviewed_removal);

INSERT INTO moml_archive.removed_volumes
SELECT * FROM moml.volumes
WHERE psmid IN (SELECT psmid FROM reviewed_removal);

DELETE FROM moml.volumes
WHERE psmid IN (SELECT psmid FROM reviewed_removal);

-- The twin editions, now without volumes, and their subjects. The rule book
-- edition keeps its other volume and stays.
INSERT INTO moml_archive.removed_edition_subjects
SELECT * FROM moml.edition_subjects s
WHERE s.bibliographicid IN (SELECT bibliographicid FROM reviewed_removal WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = s.bibliographicid);

DELETE FROM moml.edition_subjects s
WHERE s.bibliographicid IN (SELECT bibliographicid FROM reviewed_removal WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = s.bibliographicid);

INSERT INTO moml_archive.removed_edition_loc_subjects
SELECT * FROM moml.edition_loc_subjects s
WHERE s.bibliographicid IN (SELECT bibliographicid FROM reviewed_removal WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = s.bibliographicid);

DELETE FROM moml.edition_loc_subjects s
WHERE s.bibliographicid IN (SELECT bibliographicid FROM reviewed_removal WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = s.bibliographicid);

INSERT INTO moml_archive.removed_editions
SELECT * FROM moml.editions e
WHERE e.bibliographicid IN (SELECT bibliographicid FROM reviewed_removal WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = e.bibliographicid);

DELETE FROM moml.editions e
WHERE e.bibliographicid IN (SELECT bibliographicid FROM reviewed_removal WHERE kind = 'twin_edition')
  AND NOT EXISTS (SELECT 1 FROM moml.volumes v WHERE v.bibliographicid = e.bibliographicid);

DO $$
BEGIN
  IF (SELECT count(*) FROM moml_archive.removed_volumes
      WHERE psmid IN (SELECT psmid FROM reviewed_removal)) <> 7
     OR (SELECT count(*) FROM moml_archive.removed_editions
         WHERE bibliographicid IN (SELECT bibliographicid FROM reviewed_removal)) <> 6 THEN
    RAISE EXCEPTION 'expected 7 volumes and 6 editions in the archive';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM moml.editions WHERE bibliographicid = 'CTRG97-B240') THEN
    RAISE EXCEPTION 'the Federal rule book edition was removed';
  END IF;
  RAISE NOTICE 'removed % pages, % citations and % links',
    (SELECT count(*) FROM moml_archive.removed_page
     WHERE psmid IN (SELECT psmid FROM reviewed_removal)),
    (SELECT count(*) FROM moml_archive.removed_citations_unlinked
     WHERE moml_treatise IN (SELECT psmid FROM reviewed_removal)),
    (SELECT count(*) FROM moml_archive.removed_citation_links cl
     JOIN moml_archive.removed_citations_unlinked cu ON cu.id = cl.citation_id
     WHERE cu.moml_treatise IN (SELECT psmid FROM reviewed_removal));
END $$;

-- migrate:down
SET ROLE = law_admin;

-- Put the seven copies back and take them out of the archive, leaving the
-- earlier removals where they are.
CREATE TEMPORARY TABLE reviewed_removal ON COMMIT DROP AS
SELECT removed_psmid AS psmid, removed_bibliographicid AS bibliographicid
FROM moml.duplicate_copies
WHERE note IS NOT NULL;

INSERT INTO moml.editions
SELECT * FROM moml_archive.removed_editions
WHERE bibliographicid IN (SELECT bibliographicid FROM reviewed_removal);
INSERT INTO moml.volumes
SELECT * FROM moml_archive.removed_volumes
WHERE psmid IN (SELECT psmid FROM reviewed_removal);
INSERT INTO moml.edition_subjects
SELECT * FROM moml_archive.removed_edition_subjects
WHERE bibliographicid IN (SELECT bibliographicid FROM reviewed_removal);
INSERT INTO moml.edition_loc_subjects
SELECT * FROM moml_archive.removed_edition_loc_subjects
WHERE bibliographicid IN (SELECT bibliographicid FROM reviewed_removal);
INSERT INTO moml.volume_sets
SELECT * FROM moml_archive.removed_volume_sets
WHERE psmid IN (SELECT psmid FROM reviewed_removal)
   OR sibling_psmid IN (SELECT psmid FROM reviewed_removal);
INSERT INTO moml.page
SELECT * FROM moml_archive.removed_page
WHERE psmid IN (SELECT psmid FROM reviewed_removal);
INSERT INTO moml.page_ocrtext
SELECT * FROM moml_archive.removed_page_ocrtext
WHERE psmid IN (SELECT psmid FROM reviewed_removal);
INSERT INTO moml.page_content
SELECT * FROM moml_archive.removed_page_content
WHERE psmid IN (SELECT psmid FROM reviewed_removal);
INSERT INTO moml_citations.citations_unlinked
SELECT * FROM moml_archive.removed_citations_unlinked
WHERE moml_treatise IN (SELECT psmid FROM reviewed_removal);
INSERT INTO moml_citations.citation_links
SELECT cl.* FROM moml_archive.removed_citation_links cl
JOIN moml_archive.removed_citations_unlinked cu ON cu.id = cl.citation_id
WHERE cu.moml_treatise IN (SELECT psmid FROM reviewed_removal);

DELETE FROM moml_archive.removed_citation_links cl
USING moml_archive.removed_citations_unlinked cu
WHERE cu.id = cl.citation_id
  AND cu.moml_treatise IN (SELECT psmid FROM reviewed_removal);
DELETE FROM moml_archive.removed_citations_unlinked
WHERE moml_treatise IN (SELECT psmid FROM reviewed_removal);
DELETE FROM moml_archive.removed_page_content
WHERE psmid IN (SELECT psmid FROM reviewed_removal);
DELETE FROM moml_archive.removed_page_ocrtext
WHERE psmid IN (SELECT psmid FROM reviewed_removal);
DELETE FROM moml_archive.removed_page
WHERE psmid IN (SELECT psmid FROM reviewed_removal);
DELETE FROM moml_archive.removed_volume_sets
WHERE psmid IN (SELECT psmid FROM reviewed_removal)
   OR sibling_psmid IN (SELECT psmid FROM reviewed_removal);
DELETE FROM moml_archive.removed_volumes
WHERE psmid IN (SELECT psmid FROM reviewed_removal);
DELETE FROM moml_archive.removed_edition_loc_subjects
WHERE bibliographicid IN (SELECT bibliographicid FROM reviewed_removal);
DELETE FROM moml_archive.removed_edition_subjects
WHERE bibliographicid IN (SELECT bibliographicid FROM reviewed_removal);
DELETE FROM moml_archive.removed_editions
WHERE bibliographicid IN (SELECT bibliographicid FROM reviewed_removal);

UPDATE moml.volumes SET current_volume = 2
WHERE psmid = '19007239001' AND current_volume = 1;

DELETE FROM moml.duplicate_copies WHERE note IS NOT NULL;

ALTER TABLE moml.duplicate_copies DROP CONSTRAINT IF EXISTS duplicate_copies_kind_check;
ALTER TABLE moml.duplicate_copies ADD CONSTRAINT duplicate_copies_kind_check
  CHECK (kind IN ('twin_edition', 'same_edition'));

ALTER TABLE moml.duplicate_copies DROP COLUMN IF EXISTS note;

ALTER TABLE moml_archive.removed_citations_unlinked
  DROP COLUMN IF EXISTS case_name,
  DROP COLUMN IF EXISTS case_name_source;
ALTER TABLE moml_archive.removed_editions
  DROP COLUMN IF EXISTS work_id,
  DROP COLUMN IF EXISTS derivative;

COMMENT ON COLUMN moml.duplicate_copies.removed_linked IS
  'Linked citations in the removed copy on 2026-09-25 (for a twin edition, counted over the whole edition)';
COMMENT ON COLUMN moml.duplicate_copies.kind IS
  'twin_edition: the edition was catalogued twice and the whole removed edition is gone; same_edition: a second scan of one volume within an edition';
