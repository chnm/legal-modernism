-- Candidate works to join: pairs the rules and the review left apart (issue #335).
--
-- A work groups every edition of one treatise, with later editors' revisions,
-- translations, and what was derived from it (abridgments, analyses, question
-- books, supplements). db/migrations/20260925170000_moml-works.sql built the
-- works from three rules and a reviewed list of links:
--
--   1. Editions with the same first author (surname and first initial) and the
--      same main title (lower-cased, without a leading article or the author's
--      own name, letters and digits only) are one work.
--   2. So are editions by the same author whose main titles have the same
--      content words, ignoring "a treatise on the law of" and the like.
--   3. A title that names another author ("Archbold's ...", "Kerr on ...") joins
--      that author's work with the same content words, if there is exactly one.
--
-- This lists what may still belong together, for review: two works by the same
-- author whose titles share most of their content words; an edition whose title
-- names an author with a work of three or more editions, but which is not in
-- that work; and a volume set whose volumes Gale catalogued as two editions in
-- different works. A pair the review decided to keep apart appears here too;
-- the migration's comment names those decisions. Read-only; run with LAW_CLAUDE.

WITH keys AS (
  SELECT e.bibliographicid, e.work_id, e.author,
         btrim(regexp_replace(split_part(coalesce(e.author, ''), ';', 1), '\s+', ' ', 'g')) AS first_author,
         min(v.display_title) AS title
  FROM moml.editions e
  JOIN moml.volumes v USING (bibliographicid)
  GROUP BY e.bibliographicid, e.work_id, e.author
), k AS (
  SELECT keys.*,
         regexp_replace(lower(split_part(first_author, ',', 1)), '[^a-z]', '', 'g') AS surname,
         left(regexp_replace(lower(split_part(first_author, ',', 2)), '[^a-z]', '', 'g'), 1) AS initial,
         btrim(regexp_replace(lower(split_part(split_part(split_part(title, ' : ', 1), ' / ', 1), ' ; ', 1)),
                              '[^a-z0-9]+', ' ', 'g')) AS title_words
  FROM keys
), w AS (
  -- The content words of each work's titles, crudely singularized.
  SELECT k.work_id, min(k.surname || ' ' || k.initial) AS author_key,
         array_agg(DISTINCT regexp_replace(word, '(.{3,})s$', '\1')) AS words
  FROM k, regexp_split_to_table(k.title_words, ' ') AS word
  WHERE k.author IS NOT NULL
    AND word <> ALL (ARRAY['a','an','the','of','on','and','in','to','for','with','by','or','law','laws',
                           'treatise','practical','concise','relating','relative','respecting',
                           'concerning','upon','its'])
    AND length(word) > 2
  GROUP BY k.work_id
), similar_titles AS (
  SELECT 'same author, similar title' AS reason, a.work_id AS work_a, b.work_id AS work_b
  FROM w a
  JOIN w b ON a.author_key = b.author_key AND a.work_id < b.work_id
  WHERE cardinality(ARRAY(SELECT unnest(a.words) INTERSECT SELECT unnest(b.words)))::numeric
        / least(cardinality(a.words), cardinality(b.words)) >= 0.6
), big AS (
  SELECT k.surname, k.work_id
  FROM k
  GROUP BY k.surname, k.work_id
  HAVING count(*) >= 3
), name_refs AS (
  SELECT DISTINCT 'title names another author''s work' AS reason, k.work_id AS work_a, big.work_id AS work_b
  FROM k
  CROSS JOIN LATERAL regexp_matches(' ' || k.title_words || ' ', ' ([a-z]{3,}) (s|on) ', 'g') AS m
  JOIN big ON big.surname = m[1] AND big.surname <> k.surname AND big.work_id <> k.work_id
  JOIN w wa ON wa.work_id = big.work_id
  WHERE EXISTS (SELECT 1 FROM regexp_split_to_table(k.title_words, ' ') AS word
                WHERE regexp_replace(word, '(.{3,})s$', '\1') = ANY (wa.words))
), cross_sets AS (
  SELECT DISTINCT 'volume set across works' AS reason, least(a.work_id, b.work_id) AS work_a,
         greatest(a.work_id, b.work_id) AS work_b
  FROM moml.volume_sets vs
  JOIN moml.volumes va ON va.psmid = vs.psmid
  JOIN moml.volumes vb ON vb.psmid = vs.sibling_psmid
  JOIN moml.editions a ON a.bibliographicid = va.bibliographicid
  JOIN moml.editions b ON b.bibliographicid = vb.bibliographicid
  WHERE a.work_id <> b.work_id
), summary AS (
  SELECT e.work_id, count(DISTINCT e.bibliographicid) AS editions, min(v.year) AS first_year, max(v.year) AS last_year
  FROM moml.editions e JOIN moml.volumes v USING (bibliographicid)
  GROUP BY e.work_id
)
SELECT c.reason,
       c.work_a, sa.editions AS editions_a, sa.first_year AS year_a, wa.author AS author_a, wa.title AS title_a,
       c.work_b, sb.editions AS editions_b, sb.first_year AS year_b, wb.author AS author_b, wb.title AS title_b
FROM (SELECT * FROM similar_titles UNION SELECT * FROM name_refs UNION SELECT * FROM cross_sets) c
JOIN moml.works wa ON wa.work_id = c.work_a
JOIN moml.works wb ON wb.work_id = c.work_b
JOIN summary sa ON sa.work_id = c.work_a
JOIN summary sb ON sb.work_id = c.work_b
ORDER BY c.reason, wa.author, wa.title;
