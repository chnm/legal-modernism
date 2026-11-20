-- migrate:up
SET ROLE = law_admin;

-- Join the doubtful work groupings that review settled (issue #336).
--
-- The works migration (20260925170000_moml-works.sql, issue #335) left apart
-- any pair of editions it was unsure of, and issue #336 listed those pairs for
-- a second look. The review's rule is to be generous: a work is the one way to
-- see "Pomeroy on equity" whole, and the edition remains for anyone who wants
-- the finer distinction, so the same author on the same subject is one work.
-- This applies that rule to the pairs it settles. Each row below says that the
-- work of the edition on the left joins the work of the edition on the right,
-- taking all its editions with it:
--
--   * a later edition under a new title (Bishop's New commentaries on marriage,
--     Pierce's Law of railroads, Herman's Estoppel and res judicata, Burke's
--     Works and correspondence);
--   * a book on a narrower subject that continues the edition numbering of the
--     work it came out of (Wharton's Criminal evidence and Criminal pleading
--     out of his Criminal law; Woolrych's Law of sewers, styled the second
--     edition of his Waters and sewers);
--   * a companion with its own edition numbering, marked derivative: Tidd's
--     Forms beside his Practice, Daniell's Chancery forms beside his Chancery
--     practice;
--   * a casebook by the author of a treatise on the same subject, marked
--     derivative, though it does not say that it accompanies it (Fetter,
--     Wambaugh, Burdick).
--
-- 23 editions in 12 works move, and the 12 works they leave are deleted, since
-- every work has an edition. The works they join keep their ids and names: in
-- each, the principal edition (the earliest that is not derivative) is the one
-- it was. Work ids are not renumbered, so they now have gaps.
--
-- Left as they were, pending answers on the issue: the glosses (a book made
-- from two works, or "founded on" another's treatise), which the review wants
-- tagged rather than joined; Coke's Institutes, which stay apart from Coke upon
-- Littleton; the works that split in two (Fraser, Oke, Bays); and Lewis and
-- Nichols on eminent domain, which are different authors.
--
-- moml_citations.edition_citation_counts and work_citation_counts carry the
-- work id, so run make db-maintenance afterwards.

CREATE TEMPORARY TABLE work_joins (
  source_bibliographicid text PRIMARY KEY,
  target_bibliographicid text NOT NULL,
  derivative boolean NOT NULL,
  note text NOT NULL
) ON COMMIT DROP;

INSERT INTO work_joins (source_bibliographicid, target_bibliographicid, derivative, note)
VALUES
  ('ocm13275645', 'ocm13509560', false, 'Bishop, New commentaries on marriage, divorce, and separation (1891): his Commentaries on marriage and divorce, retitled'),
  ('ocm17794831', 'ocm17794826', false, 'Pierce, Law of railroads (1881): his American railroad law, retitled'),
  ('ocm13357455', 'ocm33240647', false, 'Herman, Estoppel and res judicata (1886): his Law of estoppel, retitled'),
  ('ocm13832305', 'ocm13831928', false, 'Burke, Works and correspondence (1852): his Works'),
  ('ocm32009859', 'ocm21010963', false, 'Wharton, Evidence in criminal issues: continues the edition numbering of his Criminal law'),
  ('ocm31476692', 'ocm21010963', false, 'Wharton, Criminal pleading and practice: continues the edition numbering of his Criminal law'),
  ('ocm32082697', 'ocm23045004', false, 'Woolrych, Law of sewers: styled the second edition of his Waters and sewers'),
  ('ocm28759717', 'ocm28759718', true,  'Tidd, Forms of practical proceedings: the companion to his Practice'),
  ('ocm32846880', 'ocm29628720', true,  'Daniell''s Chancery forms: the companion to his Chancery practice'),
  ('ocm23367633', 'ocm23367631', true,  'Fetter, Illustrative cases upon equity jurisprudence: casebook beside his Handbook'),
  ('ocm12779989', 'ocm12779428', true,  'Wambaugh, Cases for analysis: casebook beside his Study of cases'),
  ('ocm17348482', 'ocm17348477', true,  'Burdick, Selected cases on the law of partnership: casebook beside his Law of partnership');

-- The work each edition is in now. A row whose two editions already share a
-- work has been applied, and moves nothing.
CREATE TEMPORARY TABLE work_moves ON COMMIT DROP AS
SELECT j.source_bibliographicid, j.derivative,
       s.work_id AS from_work, t.work_id AS to_work
FROM work_joins j
JOIN moml.editions s ON s.bibliographicid = j.source_bibliographicid
JOIN moml.editions t ON t.bibliographicid = j.target_bibliographicid;

DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n FROM work_moves;
  IF n <> 12 THEN
    RAISE EXCEPTION 'found the editions of % of the 12 joins', n;
  END IF;
  -- No work is both moved and moved into, so one pass is enough.
  SELECT count(*) INTO n FROM work_moves a JOIN work_moves b ON a.to_work = b.from_work
  WHERE b.from_work <> b.to_work;
  IF n <> 0 THEN
    RAISE EXCEPTION '% joins target a work that is itself being moved', n;
  END IF;
END $$;

UPDATE moml.editions e
SET work_id = m.to_work,
    derivative = e.derivative OR m.derivative
FROM work_moves m
WHERE e.work_id = m.from_work
  AND m.from_work <> m.to_work;

DELETE FROM moml.works w
USING work_moves m
WHERE w.work_id = m.from_work
  AND m.from_work <> m.to_work;

-- Checks.
DO $$
DECLARE
  n bigint;
  r record;
BEGIN
  SELECT count(*) INTO n FROM moml.works w
  WHERE NOT EXISTS (SELECT 1 FROM moml.editions e WHERE e.work_id = w.work_id);
  IF n <> 0 THEN
    RAISE EXCEPTION '% works have no edition', n;
  END IF;

  -- The size of each work that was joined to, by one of its editions.
  FOR r IN
    SELECT x.bibliographicid, x.expected,
           (SELECT count(*) FROM moml.editions e
             WHERE e.work_id = (SELECT work_id FROM moml.editions WHERE bibliographicid = x.bibliographicid)) AS found
    FROM (VALUES ('ocm13509560', 7),    -- Bishop on marriage and divorce
                 ('ocm17794826', 3),    -- Pierce on railroads
                 ('ocm33240647', 2),    -- Herman on estoppel
                 ('ocm13831928', 3),    -- Burke's Works
                 ('ocm21010963', 16),   -- Wharton's Criminal law
                 ('ocm23045004', 5),    -- Woolrych on waters and sewers
                 ('ocm28759718', 19),   -- Tidd's Practice
                 ('ocm29628720', 18),   -- Daniell's Chancery practice
                 ('ocm23367631', 2),    -- Fetter on equity
                 ('ocm12779428', 4),    -- Wambaugh's Study of cases
                 ('ocm17348477', 5))    -- Burdick on partnership
         AS x (bibliographicid, expected)
  LOOP
    IF r.found <> r.expected THEN
      RAISE EXCEPTION 'the work of % has % editions, expected %', r.bibliographicid, r.found, r.expected;
    END IF;
  END LOOP;

  -- Each work is still named after its principal edition: none of the editions
  -- that joined is an earlier one that is not derivative.
  SELECT count(*) INTO n
  FROM moml.works w
  JOIN LATERAL (
    SELECT btrim(regexp_replace(split_part(coalesce(e.author, ''), ';', 1), '\s+', ' ', 'g')) AS first_author
    FROM moml.editions e
    JOIN moml.volumes v USING (bibliographicid)
    WHERE e.work_id = w.work_id AND NOT e.derivative
    GROUP BY e.bibliographicid, e.author
    ORDER BY min(v.year), e.bibliographicid
    LIMIT 1
  ) p ON true
  WHERE w.work_id IN (SELECT to_work FROM work_moves)
    AND p.first_author IS DISTINCT FROM coalesce(w.author, '');
  IF n <> 0 THEN
    RAISE EXCEPTION '% joined works are no longer named after their principal edition', n;
  END IF;
END $$;

ANALYZE moml.works, moml.editions;

-- migrate:down
SET ROLE = law_admin;

-- The twelve works as they were, and the editions that were in them.
INSERT INTO moml.works (work_id, author, title)
VALUES
  (152,   'Tidd, William', 'Appendix to the Practice of the Court of King''s Bench in personal actions, &c.'),
  (3221,  'Woolrych, Humphry W. (Humphry William)', 'A treatise of the law of sewers'),
  (3467,  'Burke, Edmund', 'The works and correspondence of the Right Honourable Edmund Burke'),
  (6675,  'Upjohn, William Henry', 'Daniell''s Chancery forms'),
  (6845,  'Wharton, Francis', 'A treatise on criminal pleading and practice'),
  (6846,  'Wharton, Francis', 'A treatise on the law of evidence in criminal issues'),
  (6964,  'Pierce, Edward Lillie', 'A treatise on the law of railroads'),
  (7849,  'Herman, Henry M. (Henry Morrison)', 'Commentaries on the law of estoppel and res judicata'),
  (8729,  'Bishop, Joel Prentiss', 'New commentaries on marriage, divorce, and separation'),
  (9442,  'Wambaugh, Eugene', 'Cases for analysis'),
  (9549,  'Fetter, Norman', 'Illustrative cases upon equity jurisprudence'),
  (10168, 'Burdick, Francis M', 'Selected cases on the law of partnership')
ON CONFLICT (work_id) DO NOTHING;

UPDATE moml.editions e
SET work_id = o.work_id,
    derivative = o.derivative
FROM (VALUES
  ('ocm30820555', 152,   false),
  ('ocm31813978', 152,   true),
  ('ocm28759717', 152,   false),
  ('ocm30409272', 152,   false),
  ('ocm28759724', 152,   false),
  ('ocm32082697', 3221,  false),
  ('ocm32812938', 3221,  false),
  ('ocm13832305', 3467,  false),
  ('ocm32846880', 6675,  false),
  ('ocm32847066', 6675,  false),
  ('CTRG97-B435', 6675,  false),
  ('CTRG97-B577', 6675,  false),
  ('ocm31476692', 6845,  false),
  ('ocm31476841', 6845,  false),
  ('ocm32009859', 6846,  false),
  ('ocm32009945', 6846,  false),
  ('CTRG97-B967', 6846,  false),
  ('ocm17794831', 6964,  false),
  ('ocm13357455', 7849,  false),
  ('ocm13275645', 8729,  false),
  ('ocm12779989', 9442,  false),
  ('ocm23367633', 9549,  false),
  ('ocm17348482', 10168, false)
) AS o (bibliographicid, work_id, derivative)
WHERE e.bibliographicid = o.bibliographicid;

ANALYZE moml.works, moml.editions;
