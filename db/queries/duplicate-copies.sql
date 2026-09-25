-- Duplicate copies in MOML: the same edition scanned twice (issue #142).
--
-- Gale sometimes catalogued one edition under two bibliographicids and scanned
-- a copy for each, often from different libraries, and five Part II records
-- hold two scans of one book. Both copies were OCRed and run through the
-- citation detector, so their citations were counted twice.
--
-- The rule. Two Gale editions are twin editions when their full title, author,
-- imprint and edition statement are equal once lower-cased and stripped of
-- everything but letters and digits, and their total page counts are within
-- 5% of each other. Two volumes of one edition are same-edition copies when
-- they have the same volume number and page counts within 5%. The page test
-- keeps apart records that share a title but not a text: a pamphlet of 8 pages
-- and another of 24, or a volume and one twice its length. Of each pair, the
-- copy (the whole edition, for twins) with more linked citations is kept; a tie
-- goes to the higher mean OCR confidence, then to the lower bibliographicid or
-- psmid.
--
-- This lists every candidate pair with its measurements and decision. Run on
-- 2026-09-25 it gave 46 twin editions and 4 same-edition copies meeting the
-- rule, which db/migrations/20260925140200_moml-remove-duplicate-copies.sql
-- removes; the kept copy of each is recorded in moml.duplicate_copies. After
-- that migration, the pairs listed here are the near-duplicates that failed the
-- page test and were left alone.

WITH edition AS (
  SELECT v.bibliographicid,
         regexp_replace(lower(min(v.full_title)), '[^a-z0-9]+', '', 'g') AS title_key,
         regexp_replace(lower(coalesce(min(e.author), '')), '[^a-z]+', '', 'g') AS author_key,
         regexp_replace(lower(min(v.imprint)), '[^a-z0-9]+', '', 'g') AS imprint_key,
         regexp_replace(lower(coalesce(min(v.edition_statement), '')), '[^a-z0-9]+', '', 'g') AS edition_key,
         sum(v.total_pages) AS pages,
         avg(v.ocr_confidence) AS ocr
  FROM moml.volumes v
  JOIN moml.editions e USING (bibliographicid)
  WHERE v.gale_id IS NOT NULL
  GROUP BY v.bibliographicid
),
linked AS (
  SELECT cu.moml_treatise AS psmid, count(*) AS n
  FROM moml_citations.citations_unlinked cu
  JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
  WHERE cl.status LIKE 'linked%'
    AND cu.moml_treatise IN (SELECT psmid FROM moml.volumes)
  GROUP BY cu.moml_treatise
),
pair AS (
  SELECT 'twin_edition' AS kind,
         a.bibliographicid AS a_key, b.bibliographicid AS b_key,
         a.pages AS a_pages, b.pages AS b_pages, a.ocr AS a_ocr, b.ocr AS b_ocr,
         (SELECT coalesce(sum(l.n), 0) FROM moml.volumes v JOIN linked l USING (psmid)
          WHERE v.bibliographicid = a.bibliographicid) AS a_linked,
         (SELECT coalesce(sum(l.n), 0) FROM moml.volumes v JOIN linked l USING (psmid)
          WHERE v.bibliographicid = b.bibliographicid) AS b_linked
  FROM edition a
  JOIN edition b
    ON (a.title_key, a.author_key, a.imprint_key, a.edition_key)
     = (b.title_key, b.author_key, b.imprint_key, b.edition_key)
   AND a.bibliographicid < b.bibliographicid
  UNION ALL
  SELECT 'same_edition',
         a.psmid, b.psmid, a.total_pages, b.total_pages, a.ocr_confidence, b.ocr_confidence,
         coalesce((SELECT n FROM linked WHERE psmid = a.psmid), 0),
         coalesce((SELECT n FROM linked WHERE psmid = b.psmid), 0)
  FROM moml.volumes a
  JOIN moml.volumes b
    ON a.bibliographicid = b.bibliographicid
   AND a.current_volume = b.current_volume
   AND a.psmid < b.psmid
)
SELECT kind, a_key, b_key, a_pages, b_pages, a_linked, b_linked,
       abs(a_pages - b_pages)::numeric / greatest(a_pages, b_pages) <= 0.05 AS meets_rule,
       CASE WHEN (a_linked, a_ocr, 1) > (b_linked, b_ocr, 0) THEN a_key ELSE b_key END AS keep,
       CASE WHEN (a_linked, a_ocr, 1) > (b_linked, b_ocr, 0) THEN b_key ELSE a_key END AS remove
FROM pair
ORDER BY kind, meets_rule DESC, a_key;
