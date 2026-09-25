-- migrate:up
SET ROLE = law_admin;

-- Group MOML's editions into works (issue #335).
--
-- A MOML volume belongs to an edition (issue #142), and this adds the level
-- above it: the work, a treatise across all its editions. Kent's Commentaries is
-- one work in 22 editions; Blackstone's is one work of 119. The work is taken
-- broadly: it includes later editions revised by other hands, retitled, or
-- catalogued under their editor, translations, and what was made from the
-- treatise -- abridgments, analyses, question-and-answer books, students'
-- guides and hints, supplements, indexes, and casebooks arranged to accompany
-- it. Every edition belongs to exactly one work, so an edition with no relative
-- in MOML is a work of one edition. moml.editions.derivative marks the editions
-- that are made from their work rather than editions of it, so the one can be
-- told from the other.
--
-- MOML has no field for the work, so it was built in two steps.
--
-- First, three rules, stated here in SQL and in db/queries/work-candidates.sql:
--
--   1. Editions with the same first author and the same main title are one
--      work. The first author is the first name in editions.author (the
--      original author; editors follow), compared by surname and first initial,
--      which absorbs heading variants such as "Cooley, Roger W. (Roger
--      William)". The main title is the title before its subtitle, lower-cased,
--      without a leading article or the author's own name ("Roscoe's"),
--      letters and digits only.
--   2. So are editions by the same author whose main titles have the same
--      content words, once the words that do not tell one treatise from another
--      are dropped ("a treatise on the law of", "Kerr on"): Mayne's Treatise on
--      the law of damages and his Treatise on damages.
--   3. A title that names another author ("Archbold's Summary ...", "Woodfall's
--      Law of landlord and tenant", "Kerr on fraud") joins that author's work
--      with the same content words, if there is exactly one. These are later
--      editions catalogued under their editor.
--
--   An edition with no author (422 of them) stays a work of its own: their
--   repeated titles are different pamphlets, six of them In memoriam.
--
-- Second, a review. The rules left 3,447 groups in 1,230 families of
-- candidates: titles by one author that share most of their words, titles that
-- name an author with a work of three or more editions, and six volume sets
-- that Gale catalogued as two editions. Each family was read, and 1,041 links
-- join an edition to the work of another: 849 because it is the same text under
-- a changed title (the Court of King's Bench became the Queen's Bench in 1837;
-- Pratt's Law of highways became Pratt and Mackenzie's), 192 because it was made
-- from that work. The rule in doubt was to leave apart:
--
--   * a different subject by the same author (Story on agency and on
--     partnership), or a casebook beside a treatise, unless the casebook says it
--     accompanies it;
--   * a book merely "founded on" its sources: Stephen's New commentaries,
--     "partly founded on Blackstone", is a work of its own, and J. W. Smith's
--     Manual of equity, "founded on Story's Commentaries and Spence's", is too;
--   * a book made from two works, since an edition can belong to one work only
--     (an analysis of both Leake and Benjamin on contracts);
--   * a work split in two, such as Oke's Game and fishery laws, which became
--     two books; and a volume set of unrelated tracts bound together.
--
-- A rule group can hold both an edition and an abridgment whose titles agree
-- ("Blackstone's Commentaries"), so four such editions are marked derivative by
-- hand.
--
-- Measured on 2026-09-25: 16,058 works, of which 2,374 have more than one
-- edition, together 8,085 of the 21,769 editions; 248 editions are derivative.
-- The largest works are Blackstone's Commentaries (119 editions, 46
-- derivative), Archbold's Criminal pleading (32), Williams on real property
-- (31), Woodfall on landlord and tenant (30), Snell's Principles of equity (28,
-- with Blyth's analyses and Gibson's Aids), Anson on contract (28), and the
-- Federalist (26, with its translations).
--
-- A work is named after its principal edition, its earliest edition that is not
-- derivative: moml.works.author is that edition's first author and title its
-- main title. Work ids are assigned in order of the work's earliest year, so
-- they are the same wherever the migration runs; a work added later (by
-- scripts/sidecorpus-import, for a new edition) takes the next id.
-- db/queries/work-candidates.sql lists the pairs that may still belong together.

-- 1. Keys for the rules. The same keys are in db/queries/work-candidates.sql.
CREATE TEMPORARY TABLE work_keys ON COMMIT DROP AS
WITH e AS (
  SELECT e.bibliographicid, e.author,
         min(v.year) AS year,
         min(v.display_title) AS title,
         btrim(regexp_replace(split_part(coalesce(e.author, ''), ';', 1), '\s+', ' ', 'g')) AS first_author
  FROM moml.editions e
  JOIN moml.volumes v USING (bibliographicid)
  GROUP BY e.bibliographicid, e.author
), s AS (
  SELECT e.*,
         regexp_replace(lower(split_part(first_author, ',', 1)), '[^a-z]', '', 'g') AS surname,
         left(regexp_replace(lower(split_part(first_author, ',', 2)), '[^a-z]', '', 'g'), 1) AS initial,
         regexp_replace(lower(split_part(split_part(split_part(title, ' : ', 1), ' / ', 1), ' ; ', 1)),
                        '^(a|an|the) ', '') AS main_title
  FROM e
), t AS (
  SELECT s.*,
         btrim(regexp_replace(
           CASE WHEN surname <> '' THEN regexp_replace(main_title, '^' || surname || '''?s ', '')
                ELSE main_title END,
           '[^a-z0-9]+', ' ', 'g')) AS title_key
  FROM s
)
SELECT t.bibliographicid, t.author, t.year, t.title, t.first_author, t.surname, t.title_key,
       t.surname || ' ' || t.initial AS author_key,
       coalesce(
         (SELECT string_agg(w, ' ' ORDER BY w COLLATE "C")
            FROM (SELECT DISTINCT w
                    FROM regexp_split_to_table(
                           CASE WHEN t.surname <> ''
                                THEN regexp_replace(t.title_key, '^' || t.surname || ' (s )?on ', '')
                                ELSE t.title_key END, ' ') AS w
                   WHERE w <> ''
                     AND w <> ALL (ARRAY['a','an','the','of','on','and','in','to','for','with','by','or',
                                         'law','laws','treatise','practical','concise','relating','relative',
                                         'respecting','concerning','upon','its'])) d),
         t.title_key) AS content_key
FROM t;

-- Rules 1 and 2: an edition with an author belongs with every edition of the
-- same author key and content key. An edition without an author is alone.
ALTER TABLE work_keys ADD COLUMN group_key text;
UPDATE work_keys
SET group_key = CASE WHEN author IS NOT NULL THEN author_key || '|' || content_key
                     ELSE 'edition|' || bibliographicid END;

-- Rule 3: a title "<name>'s ..." or "<name> on ..." that names someone other
-- than the first author belongs with <name>'s group of the same content key,
-- when exactly one such group exists.
CREATE TEMPORARY TABLE work_rule3 ON COMMIT DROP AS
WITH ref AS (
  SELECT k.bibliographicid, k.group_key, k.surname,
         (regexp_match(k.title_key, '^([a-z]+) (s|on) (.+)$'))[1] AS name,
         (regexp_match(k.title_key, '^([a-z]+) (s|on) (.+)$'))[3] AS rest
  FROM work_keys k
  WHERE k.author IS NOT NULL
    AND k.title_key ~ '^([a-z]+) (s|on) (.+)$'
), refk AS (
  SELECT ref.*,
         coalesce((SELECT string_agg(w, ' ' ORDER BY w COLLATE "C")
                     FROM (SELECT DISTINCT w FROM regexp_split_to_table(rest, ' ') AS w
                            WHERE w <> ''
                              AND w <> ALL (ARRAY['a','an','the','of','on','and','in','to','for','with','by','or',
                                                  'law','laws','treatise','practical','concise','relating','relative',
                                                  'respecting','concerning','upon','its'])) d),
                  rest) AS rest_key
  FROM ref
  WHERE ref.name <> ref.surname
)
SELECT r.group_key AS from_group, min(t.group_key) AS to_group
FROM refk r
JOIN work_keys t
  ON t.author IS NOT NULL AND t.surname = r.name AND t.content_key = r.rest_key
 AND t.group_key <> r.group_key
GROUP BY r.bibliographicid, r.group_key
HAVING count(DISTINCT t.group_key) = 1;

-- Components of the rule groups joined by rule 3.
CREATE TEMPORARY TABLE work_rule_component ON COMMIT DROP AS
SELECT DISTINCT group_key, group_key AS component FROM work_keys;

DO $$
BEGIN
  LOOP
    UPDATE work_rule_component c
    SET component = m.component
    FROM (SELECT e.a, min(rc.component) AS component
          FROM (SELECT from_group AS a, to_group AS b FROM work_rule3
                UNION SELECT to_group, from_group FROM work_rule3) e
          JOIN work_rule_component rc ON rc.group_key = e.b
          GROUP BY e.a) m
    WHERE c.group_key = m.a AND m.component < c.component;
    EXIT WHEN NOT FOUND;
  END LOOP;
END $$;

-- 2. The reviewed links. Each says that the edition on the left belongs to the
-- work of the edition on the right; derivative marks an abridgment, analysis,
-- question book, supplement, adaptation or translation of that work.
CREATE TEMPORARY TABLE work_links (
    source_bibliographicid text NOT NULL,
    target_bibliographicid text NOT NULL,
    derivative             boolean NOT NULL,
    note                   text NOT NULL
) ON COMMIT DROP;

INSERT INTO work_links (source_bibliographicid, target_bibliographicid, derivative, note)
VALUES
  ('ocm23963572', 'ocm23963565', true,  'Supplement to Barnes''s Observations on the tithe modus bill (volume set)'),
  ('ocm13813872', 'ocm13813875', false, 'Ellis, second volume of the General introduction to Domesday (volume set)'),
  ('ocm12213640', 'ocm12213618', false, 'Second report of the same New York bar committee (volume set)'),
  ('ocm23737432', 'ocm26000918', false, 'Shaws'' Parish law, later editions of Archbold''s Parish officer'),
  ('ocm14150532', 'ocm14150479', false, 'Ballard''s Law of real property, retitled Ballard annual'),
  ('ocm18889236', 'CTRG96-B3767', true, 'Casebook arranged with reference to Clark''s Handbook of criminal law'),
  ('ocm13832346', 'ocm13831928', false, 'The works of Edmund Burke, 3rd ed. of the collected works'),
  ('ocm32026764', 'CTRG96-B3124', false, 'Pratt''s digest of national bank laws, earlier edition'),
  ('CTRG96-B3122', 'CTRG96-B3124', false, 'Pratt''s digest of national bank laws, 1907 edition'),
  ('CTRG96-B3120', 'CTRG96-B3124', false, 'Pratt''s digest of national banking laws, 1917 edition'),
  ('CTRG00-B1572', 'CTRG96-B3124', false, 'Pratt''s digest retitled federal banking laws, 1920'),
  ('ocm32028789', 'CTRG96-B3124', true, 'Supplement to Pratt''s digest of the National Bank Act'),
  ('ocm32046978', 'ocm32884338', false, 'Pratt''s Law of highways, 13th-14th eds by Mackenzie'),
  ('CTRG96-B1412', 'ocm32884338', false, 'Pratt & Mackenzie''s highways, 15th-16th eds'),
  ('CTRG98-B2769', 'ocm32884338', false, 'Pratt & Mackenzie''s highways, 17th ed'),
  ('CTRG97-B1215', 'ocm31538801', false, 'Roscoe''s criminal evidence, 13th ed by Cohen'),
  ('CTRG97-B2856', 'ocm31538801', false, 'Roscoe''s criminal evidence, 14th ed'),
  ('ocm32773002', 'ocm31733762', false, 'Roscoe''s nisi prius evidence, 11th ed'),
  ('CTRG98-B2831', 'ocm31733762', false, 'Roscoe''s nisi prius evidence retitled civil actions, 19th ed'),
  ('ocm17372757', 'ocm17234499', false, 'Collier on bankruptcy, 1st-2nd eds'),
  ('CTRG96-B434', 'ocm17234499', false, 'Collier on bankruptcy, 4th ed'),
  ('CTRG98-B727', 'ocm17234499', false, 'Collier on bankruptcy 4th ed catalogued under editor'),
  ('ocm32415096', 'ocm31991715', true, 'Precedents expressly supplementary to Story''s Equity pleadings'),
  ('ocm31372359', 'ocm31533131', false, 'Powell on evidence, first edition under earlier title'),
  ('CTRG96-B3381', 'ocm30808531', true, 'Analytical tables for use with Stephen''s Digest of evidence'),
  ('CTRG98-B1094', 'ocm30808531', true, 'American adaptation of Stephen''s Digest of evidence'),
  ('ocm29451288', 'ocm30802146', false, 'Smith''s Action at law continued by its editor Foulkes'),
  ('ocm32023663', 'ocm32153928', true, 'Key to Story''s Equity jurisprudence'),
  ('ocm32023702', 'ocm32153928', true, 'Introduction on the basis of Story''s Equity commentaries'),
  ('ocm20350132', 'ocm20369515', false, 'Indermaur''s epitome of conveyancing and equity cases, 1st ed'),
  ('ocm12733119', 'ocm14341726', true, 'Epitome intended as guide to Smith''s Leading cases'),
  ('ocm30434530', 'ocm27566396', false, 'Indermaur''s manual of Judicature practice, 6th ed'),
  ('CTRG96-B1561', 'ocm27566396', false, 'Indermaur''s manual of practice, King''s Bench eds'),
  ('CTRG96-B1600', 'ocm31373021', true, 'Epitome of White and Tudor''s Leading cases in equity'),
  ('ocm32007891', 'ocm22619909', true, 'Supplement to Buswell and Walcott''s Massachusetts practice'),
  ('CTRG95-B4212', 'ocm17395967', false, 'Roscoe''s digest of the law of light, 4th ed'),
  ('CTRG98-B3172', 'CTRG98-B3171', true, 'Supplement to Smith''s annotated Pennsylvania Practice Act'),
  ('CTRG97-B2485', 'ocm31741569', true, 'Analysis of Smith''s Principles of equity'),
  ('ocm30802163', 'ocm31775631', false, 'J.W. Smith''s Manual of equity, early eds'),
  ('ocm31864754', 'ocm31775631', false, 'J.W. Smith''s Manual of equity, 13th-14th eds'),
  ('ocm14239480', 'ocm12733727', false, 'J.W. Smith''s Manual of common law, 1st ed'),
  ('ocm14349711', 'ocm12733727', false, 'J.W. Smith''s Manual of common law, 10th-12th eds'),
  ('ocm24452143', 'ocm24451503', false, 'W.L. Smith''s probate practice, Massachusetts 5th-6th eds'),
  ('ocm12149377', 'ocm13708038', false, 'Stephen''s New commentaries, early eds'),
  ('CTRG95-B3661', 'ocm13708038', false, 'Stephen''s New commentaries, 15th ed'),
  ('ocm14957488', 'ocm13708038', true, 'Questions on Stephen''s New commentaries'),
  ('ocm14957494', 'ocm13708038', true, 'Questions on Stephen''s New commentaries'),
  ('CTRG98-B2757', 'ocm13708038', true, 'Examination guide on Stephen''s Commentaries'),
  ('ocm12738643', 'ocm13708038', true, 'Hints on Stephen''s Commentaries for students'),
  ('CTRG96-B1640', 'ocm27438677', false, 'Stephen''s Digest of the criminal law, 6th ed'),
  ('ocm22470705', 'ocm31640583', false, 'Story''s Selection of pleadings, 2nd ed'),
  ('ocm32663793', 'ocm17317394', false, 'Story on agency, 8th-9th eds'),
  ('ocm31640513', 'ocm17320286', false, 'Story on partnership, 2nd ed'),
  ('ocm30386867', 'ocm30386848', false, 'Stephen on pleading, 6th ed catalogued under editor'),
  ('ocm15555986', 'ocm15642094', false, 'Parsons'' Laws of business, 1857 ed'),
  ('ocm15601067', 'ocm15642094', false, 'Parsons'' Laws of business, rev. eds'),
  ('ocm15229374', 'ocm15642094', false, 'Parsons'' Laws of business, 1875 ed'),
  ('ocm15642066', 'ocm15642094', false, 'Parsons'' Laws of business, enlarged with Canada'),
  ('ocm32915846', 'ocm23535452', false, 'Rogers on elections, 7th ed'),
  ('ocm23535376', 'ocm23535452', false, 'Rogers on elections, 8th ed'),
  ('ocm32915865', 'ocm23535452', false, 'Rogers on elections, 9th ed'),
  ('CTRG95-B3859', 'CTRG95-B2213', false, 'Schouler''s combined wills and administration, 4th ed'),
  ('CTRG95-B2548', 'ocm33083112', false, 'Hurrell on directors and officers, 4th ed'),
  ('CTRG99-B57', 'ocm17372869', false, 'Mechem''s cases on agency, 2nd ed by Seavey'),
  ('CTRG96-B1586', 'ocm22830438', false, 'Kerly on trade marks, 3rd-4th eds'),
  ('ocm11997180', 'ocm11985504', false, 'Clerk and magistrate''s assistant, 1st ed'),
  ('ocm17065541', 'ocm15495081', true, 'Supplement to Gould and Tucker''s Notes on revised statutes'),
  ('ocm31430085', 'ocm25754827', false, 'The Federalist, other editions'),
  ('ocm25754898', 'ocm25754827', false, 'The Federalist, Scott edition with other papers'),
  ('ocm18038713', 'ocm25754827', false, 'The Federalist, Scott edition with other papers'),
  ('ocm23469757', 'ocm32002920', false, 'Thayer''s cases on evidence, 1st ed'),
  ('ocm31633556', 'ocm32181580', false, 'Thayer''s Preliminary treatise, Part I issued separately'),
  ('ocm28146178', 'ocm28146168', false, 'Gray''s Country attorney''s practice, 7th ed'),
  ('ocm28146194', 'ocm28146168', false, 'Gray''s Country attorney''s practice, 8th-9th eds'),
  ('ocm31871004', 'ocm32539205', true, 'Supplement to Burn''s Justice of the peace'),
  ('ocm32519495', 'ocm32539205', true, 'Supplement to Burn''s Justice of the peace'),
  ('CTRG99-B900', 'CTRG95-B1575', false, 'Bays on property, 2nd ed'),
  ('ocm23019973', 'ocm21326956', false, 'May on fraudulent conveyances, 2nd ed'),
  ('CTRG95-B2630', 'ocm21326956', false, 'May on fraudulent conveyances, 3rd ed retitled'),
  ('CTRG96-B2053', 'CTRG95-B3207', true, 'Supplement to Drinker''s Interstate Commerce Act treatise'),
  ('CTRG97-B649', 'CTRG95-B3471', true, 'Analysis of Salmond''s Jurisprudence'),
  ('CTRG95-B3462', 'CTRG95-B3471', false, 'Salmond''s Jurisprudence, 4th-7th eds'),
  ('CTRG99-B1354', 'CTRG96-B1301', false, 'Zoline on federal appellate jurisdiction, 2nd ed'),
  ('ocm17317274', 'CTRG96-B165', false, 'Dyer''s Maine corporation statutes, 2nd ed'),
  ('ocm17317279', 'CTRG96-B165', false, 'Dyer''s Maine corporation statutes, 4th ed'),
  ('CTRG98-B1031', 'CTRG96-B3064', true, 'Supplement to Holmes'' Federal income tax'),
  ('CTRG98-B1000', 'CTRG96-B3064', true, '1921 supplement to Holmes'' Federal income tax'),
  ('ocm23753248', 'ocm23753464', false, 'Hanson''s death duties acts, 4th-5th eds'),
  ('CTRG96-B718', 'ocm23753464', false, 'Hanson''s death duties acts, 6th ed'),
  ('CTRG98-B972', 'CTRG97-B417', false, 'Wigmore''s cases on evidence, 2nd ed'),
  ('ocm32663500', 'ocm33888609', false, 'Snyder''s mechanics'' lien law, 1st ed'),
  ('CTRG98-B1150', 'ocm33888609', false, 'Snyder''s lien law, 6th ed'),
  ('ocm12044039', 'ocm12046947', false, 'Wells'' Every man his own lawyer, new ed'),
  ('ocm16724592', 'ocm16701493', false, 'Saunders on affiliation and bastardy, 10th ed'),
  ('ocm32372559', 'ocm20345329', false, 'Ellis on fire and life insurance, 1834 ed'),
  ('ocm17017736', 'ocm20345329', false, 'Ellis on fire and life insurance, American ed'),
  ('ocm32189039', 'ocm18197056', false, 'Blackburn on contract of sale, 1847 ed'),
  ('ocm18197080', 'ocm18197056', false, 'Blackburn on contract of sale, 1887 American ed'),
  ('ocm20549409', 'ocm28935846', false, 'Baldwin on bankruptcy, earlier eds before bills of sale'),
  ('ocm21327170', 'ocm32850129', false, 'Manson on trading companies, 2nd ed'),
  ('ocm31674920', 'ocm31723928', false, 'Brooke''s Notary, 2nd ed'),
  ('ocm31912204', 'ocm26018967', false, 'Connell on Scottish tithes, 2nd ed'),
  ('ocm30484305', 'ocm30484294', false, 'Atkinson''s Sheriff law, 3rd-4th eds'),
  ('ocm31716121', 'ocm30484294', false, 'Atkinson''s Sheriff law, 6th ed'),
  ('ocm32140154', 'ocm31741607', false, 'Starkie on evidence, English 2nd-3rd eds'),
  ('CTRG95-B2027', 'CTRG95-B1822', false, 'Noble on Massachusetts charity trusts, 2nd ed'),
  ('CTRG95-B2434', 'CTRG95-B4209', true, 'Continuation of Shearwood''s selection of bar questions'),
  ('ocm22620960', 'CTRG95-B2533', false, 'Macnamara on carriers, 1st ed'),
  ('CTRG95-B2855', 'ocm14186931', true, 'Notes for use with Chaplin on wills'),
  ('CTRG98-B883', 'ocm14186931', true, 'Notes for use with Chaplin on wills'),
  ('CTRG95-B2909', 'ocm22657752', false, 'Gardiner''s constitutional documents, 3rd ed'),
  ('CTRG95-B2978', 'CTRG97-B1487', false, 'Duckworth on charter-parties, 2nd ed'),
  ('CTRG95-B3663', 'ocm14394904', false, 'Odgers'' Common law of England, 10th ed of Broom'),
  ('CTRG98-B746', 'CTRG95-B3940', true, 'Supplement to Eastman on Pennsylvania private corporations'),
  ('CTRG95-B4056', 'ocm14186936', false, 'Chaplin on suspension of power of alienation, 2nd ed'),
  ('CTRG95-B4250', 'ocm21834191', false, 'Lawrance''s deeds of arrangement, 7th-8th eds'),
  ('ocm32875157', 'CTRG95-B4610', false, 'Emden on winding-up, early ed'),
  ('CTRG96-B1411', 'ocm31849396', false, 'Mather''s sheriff law, 2nd ed'),
  ('CTRG96-B1566', 'ocm30374689', false, 'Pritchard''s quarter sessions, 2nd ed'),
  ('CTRG96-B1708', 'CTRG97-B1196', false, 'Willis on housing and town planning, 2nd ed'),
  ('CTRG98-B3120', 'CTRG96-B3', false, 'White''s Making of the English constitution, 2nd ed'),
  ('CTRG99-B879', 'CTRG96-B3363', false, 'Sundheim on building and loan associations, 2nd ed'),
  ('CTRG98-B3231', 'CTRG97-B1027', false, 'Branson on instructions to juries, 2nd ed'),
  ('CTRG97-B1238', 'ocm32938838', false, 'Armour-Hannay on valuation for rating, 2nd ed'),
  ('CTRG97-B1461', 'ocm17395863', false, 'Hall''s Law relating to children, 3rd ed'),
  ('CTRG97-B2877', 'CTRG97-B2878', true, 'Supplement to Nichols'' NY pleading and practice'),
  ('CTRG98-B2895', 'CTRG97-B665', false, 'Bell''s Sale of Food and Drugs Acts, 7th ed'),
  ('CTRG98-B1058', 'ocm25471044', true, 'Equitable remedies expressly supplementary to Pomeroy''s Equity jurisprudence'),
  ('CTRG98-B1322', 'ocm19337943', false, 'Wiltsie on foreclosing mortgages, revised ed'),
  ('CTRG99-B870', 'CTRG99-B39', false, 'Volume IV of Eastman''s Courts and lawyers of Pennsylvania'),
  ('ocm12734275', 'ocm14399021', false, 'Petersdorff''s Abridgment, later issue'),
  ('ocm12991054', 'ocm12982577', false, 'Law Academy of Philadelphia constitution, earlier issues'),
  ('ocm13486871', 'ocm13491164', false, 'Sewall''s legal condition of women in Massachusetts, 1886 update'),
  ('ocm14073255', 'ocm17632941', false, 'Cruise''s Digest of real property, revised American ed'),
  ('ocm14094192', 'ocm14094216', false, 'Taylor''s Landlord and tenant, 1st ed'),
  ('ocm31801321', 'ocm14880711', false, 'Irving''s study of the civil law, 3rd ed under earlier title'),
  ('ocm20438189', 'ocm15374287', false, 'Newell on libel and slander, 1st ed'),
  ('ocm16724699', 'ocm16724694', true, 'Supplement to Falconer''s essay On surnames'),
  ('ocm17153590', 'ocm17153561', false, 'Jones on railroad securities, 1st ed of corporate bonds'),
  ('ocm17313144', 'ocm20611086', false, 'Morawetz on private corporations, 1st ed'),
  ('ocm20399619', 'ocm17563001', false, 'Hudson''s guide to making and proving wills, early ed'),
  ('ocm17738859', 'ocm31911590', false, 'Fawcett on landlord and tenant, 1st ed'),
  ('ocm20387472', 'ocm18278075', false, 'Chalmers'' Sale of goods, 1st ed'),
  ('ocm18435766', 'ocm23020023', false, 'Mayne on damages, American ed by Wood'),
  ('ocm23077096', 'ocm19451709', false, 'Comyn on contracts, 3rd American ed'),
  ('ocm20278087', 'ocm20256996', true, 'Appendix to Parker''s Notes on the diligence of adjudication'),
  ('ocm20345427', 'ocm32055753', false, 'Fell on guaranties, 1st American ed'),
  ('ocm20447793', 'ocm20409915', false, 'Baylis on domestic servants, 5th ed'),
  ('ocm31868208', 'ocm20570171', true, 'Appendix to Gould and Tucker''s Federal income tax'),
  ('ocm23279701', 'ocm20847524', false, 'Barbour''s NY criminal law, 3rd ed'),
  ('ocm31820748', 'ocm21122285', false, 'Dix''s Remarks on prisons, 2nd ed'),
  ('ocm21380703', 'ocm21380731', false, 'Chadwyck-Healey on companies, 3rd enlarged ed'),
  ('ocm21947198', 'ocm21575630', false, 'Browne on carriers, American ed by Wood'),
  ('ocm22445246', 'ocm29817808', false, 'Harris''s Before trial, American ed'),
  ('ocm22602942', 'ocm22602891', false, 'Dicey''s Law of the constitution, 1st-2nd eds'),
  ('ocm32587088', 'ocm22914011', false, 'Jeremy on carriers, 1816 ed'),
  ('ocm23534893', 'ocm23834837', false, 'Pulling on City of London, 1st ed'),
  ('ocm29516707', 'ocm23678763', false, 'Arnould''s life of Lord Denman, retitled ed'),
  ('ocm23805092', 'ocm23805113', false, 'Alpe on stamp duties, 1st ed'),
  ('ocm23862951', 'ocm23862953', false, 'Castle on rating, 3rd-4th eds'),
  ('ocm24397397', 'ocm24397157', false, 'Dayton on surrogates, 3rd ed'),
  ('ocm25421205', 'ocm29884200', false, 'Phelps''s history of Newgate of Connecticut, 1860 ed'),
  ('ocm26019622', 'ocm27270157', false, 'Callis upon the statute of sewers, later ed'),
  ('ocm31943344', 'ocm26821946', false, 'Browne on divorce court practice, 3rd ed'),
  ('ocm27692488', 'ocm27692439', true, 'Supplement to Webster''s Law of letters patent'),
  ('ocm31926945', 'ocm28159322', false, 'Jepson''s Lands Clauses Acts, 2nd ed'),
  ('ocm31774918', 'ocm28394786', false, 'Slater on arbitration, 1st ed as epitome'),
  ('ocm29044187', 'ocm32408706', false, 'Chitty''s Practice of the law, 3rd ed'),
  ('ocm32539358', 'ocm30136654', false, 'Bythewood''s Selection of precedents, 2nd ed'),
  ('ocm30434450', 'ocm32965925', false, 'Hume''s Commentaries on crimes, 1st ed vol. on trial'),
  ('ocm31926834', 'ocm31390048', false, 'Jarman''s Chancery practice, 3rd ed'),
  ('ocm31864357', 'ocm31864403', false, 'Whittaker''s Practice and pleading under the code, 2nd ed'),
  ('ocm32010235', 'ocm32172275', false, 'Wharton and Stille''s Medical jurisprudence, 3rd-4th eds'),
  ('ocm32324014', 'ocm32754244', false, 'Bingham on judgments and executions, 1836 ed'),
  ('ocm32799947', 'ocm32532526', false, 'De Lolme''s Constitution of England, other eds'),
  ('ocm28539076', 'ocm26726848', false, 'Archbold''s King''s/Queen''s Bench practice, later editions'),
  ('ocm32830364', 'ocm26726848', false, 'Chitty''s Archbold''s Practice, 14th ed.'),
  ('ocm28861458', 'ocm26726848', false, 'Archbold''s practice, 13th ed. by Prentice'),
  ('ocm27420482', 'ocm28861394', false, 'Archbold''s digest of pleading and evidence, retitled 2nd ed.'),
  ('ocm29516638', 'ocm28861394', false, 'American edition of Archbold''s civil pleading digest'),
  ('ocm27421829', 'ocm26220808', false, 'Archbold''s practice of attornies, later retitled editions'),
  ('ocm28935315', 'ocm27420528', false, '2nd ed. of Archbold''s New practice'),
  ('ocm29951174', 'ocm30484280', false, 'Archbold''s criminal pleading, later editions by Jervis'),
  ('ocm31906150', 'ocm30484280', false, 'Archbold''s criminal pleading, 22nd ed.'),
  ('CTRG97-B1220', 'ocm30484280', false, 'Archbold''s criminal pleading, 23rd ed. by Craies'),
  ('CTRG97-B1235', 'ocm30484280', false, 'Archbold''s criminal pleading, 24th-26th eds.'),
  ('ocm31395738', 'ocm30484280', false, 'American (Waterman/Pomeroy) editions of Archbold''s criminal pleading'),
  ('CTRG96-B934', 'ocm30484280', true, 'Compiled from Archbold''s Pleading, evidence & practice'),
  ('ocm26773508', 'ocm32915516', false, 'Archbold''s edition of Robinson''s Magistrate''s pocket-book'),
  ('ocm32915195', 'ocm32915516', false, 'Archbold''s 4th ed. of Robinson''s Magistrate''s pocket-book'),
  ('ocm30484289', 'ocm30105309', false, 'Archbold''s Quarter sessions, 1st ed.'),
  ('CTRG96-B672', 'ocm30105309', false, 'Archbold''s Quarter sessions, 6th ed.'),
  ('ocm23737361', 'ocm26000918', false, 'Archbold''s Parish officer, 4th ed.'),
  ('ocm23737386', 'ocm26000918', false, 'Archbold''s Parish officer, 5th ed.'),
  ('ocm31808864', 'ocm31407022', false, 'Chitty on pleading'),
  ('ocm30148904', 'ocm31407022', false, 'Chitty on pleading, 1809'),
  ('ocm29617651', 'ocm31407022', false, 'Chitty on pleading, 5th eds.'),
  ('ocm31415754', 'ocm31407022', false, 'Chitty on pleading, 8th American ed.'),
  ('ocm32766614', 'ocm31407022', true, 'Supplement (precedents) to Chitty''s Treatise on pleading'),
  ('ocm31749943', 'ocm31407233', false, 'Chitty on contracts, 1st ed.'),
  ('ocm33006602', 'ocm31407233', false, 'Chitty on contracts, retitled later editions'),
  ('ocm32146996', 'ocm31407233', false, 'Chitty on contracts, 9th American ed.'),
  ('ocm33006333', 'ocm31407233', false, 'Chitty on contracts, 6th English ed.'),
  ('ocm32041834', 'ocm31407233', false, 'Chitty on contracts, 11th-18th eds.'),
  ('ocm21204659', 'ocm21204689', false, 'Palmer''s Company precedents, 2nd ed.'),
  ('ocm21204676', 'ocm21204689', false, 'Palmer''s Company precedents, 4th ed.'),
  ('CTRG95-B2867', 'ocm21204689', false, 'Palmer''s Company precedents, 8th and 11th eds.'),
  ('CTRG98-B233', 'ocm21204689', false, 'Palmer''s Company precedents, 9th ed.'),
  ('ocm29873878', 'ocm28228561', false, 'Odgers'' Principles of pleading, 1st ed.'),
  ('ocm32532426', 'ocm28228561', false, 'Odgers'' Principles of pleading, 2nd ed.'),
  ('CTRG97-B1473', 'ocm28228561', false, 'Odgers'' Principles of pleading, 6th-8th eds.'),
  ('CTRG96-B817', 'ocm28228561', false, 'Odgers'' Principles of pleading, 7th ed.'),
  ('CTRG97-B977', 'ocm31533131', false, 'Powell''s Principles and practice of evidence, 9th ed. by Odgers'),
  ('CTRG97-B491', 'CTRG97-B492', false, 'Bullen and Leake''s Precedents of pleadings, 7th ed.'),
  ('ocm17312779', 'ocm31418157', false, 'Dill''s New Jersey private companies, other editions'),
  ('CTRG99-B1065', 'CTRG95-B3234', true, 'Supplement to Frankfurter''s Interstate Commerce Act cases'),
  ('ocm14442745', 'ocm13866819', true, 'Thomas''s systematic arrangement of Coke upon Littleton'),
  ('ocm14443047', 'ocm13866819', true, 'Abridgment of Coke upon Littleton'),
  ('CTRG96-B2112', 'CTRG96-B3767', false, 'Clark''s Handbook of criminal law, 3rd ed.'),
  ('ocm19030265', 'CTRG96-B3767', true, 'Casebook arranged with references to Clark''s Handbook of criminal law'),
  ('CTRG97-B339', 'ocm23367655', false, 'Clark''s Handbook of criminal procedure, 2nd ed.'),
  ('ocm31813826', 'ocm32184646', false, 'Chitty on bills, American new ed.'),
  ('ocm18456473', 'ocm32184646', false, 'Chitty on bills, 10th ed.'),
  ('ocm18396849', 'ocm32184646', false, 'Chitty on bills, 11th ed.'),
  ('CTRG95-B2550', 'CTRG95-B2558', false, 'States it is 4th ed. of Elliott''s workmen''s compensation'),
  ('CTRG95-B3292', 'ocm14103609', false, 'Tiedeman''s American law of real property, 3rd-4th eds.'),
  ('ocm13901142', 'ocm14103609', true, 'Casebook for use with Tiedeman''s treatise on real property'),
  ('CTRG99-B116', 'ocm14103609', true, '2nd ed. of casebook for use with Tiedeman''s treatise'),
  ('CTRG96-B1633', 'ocm32777686', false, 'Thwaites'' Guide to criminal law, 7th-9th eds.'),
  ('CTRG96-B535', 'ocm32902561', false, 'Indermaur and Thwaites'' constitutional law guide, 4th-5th eds.'),
  ('ocm23019714', 'ocm23019706', false, 'Stubbs'' Constitutional history, library ed.'),
  ('CTRG96-B748', 'ocm23019706', false, 'Stubbs'' Constitutional history, 6th ed.'),
  ('CTRG98-B1186', 'CTRG97-B2316', true, 'Supplement to Sutherland''s Code pleading and practice'),
  ('ocm32438772', 'ocm23113977', true, 'Precedents issued as appendix to Van Santvoord''s Pleading'),
  ('ocm12756096', 'ocm12747488', false, 'Gifford''s English lawyer, later editions'),
  ('ocm13593341', 'ocm12747488', false, 'Gifford''s English lawyer, Irish 14th ed.'),
  ('ocm13593324', 'ocm12747488', true, 'Supplement to Gifford''s English and Irish lawyer'),
  ('ocm13813349', 'ocm14930858', false, 'Thomas May''s History of the Parliament, new ed.'),
  ('ocm33005157', 'ocm23529535', false, 'Erskine May''s Constitutional history of England'),
  ('ocm15616778', 'ocm15616773', true, 'Casebook arranged to accompany Burdick''s Law of sales'),
  ('ocm18278440', 'ocm32023530', false, 'Odgers on libel and slander, 5th ed.'),
  ('ocm20278111', 'ocm32023530', true, 'Supplement to Odgers'' Digest of the law of libel'),
  ('ocm32455470', 'ocm23659478', false, 'Another translation of Gneist''s English Parliament'),
  ('ocm23608987', 'ocm23659478', false, 'Gneist''s English Parliament translation, 4th ed.'),
  ('ocm31879163', 'ocm30484312', false, 'Ayckbourn''s Chancery practice, earlier editions'),
  ('ocm31717159', 'ocm30484312', false, 'Forms volume, second volume of Ayckbourn''s Chancery practice'),
  ('ocm31977840', 'ocm30484312', false, 'Revised 10th ed. of Ayckbourn''s Chancery practice'),
  ('ocm23113661', 'ocm25471044', false, 'Pomeroy''s Equity jurisprudence, 2nd ed.'),
  ('CTRG97-B3075', 'CTRG97-B3074', true, 'Casebook, companion book to Eaton on equity'),
  ('ocm22967697', 'ocm22885425', false, 'Roscoe''s Admiralty practice, 2nd ed.'),
  ('CTRG95-B2760', 'ocm22885425', false, 'Roscoe''s Admiralty practice, 3rd-4th eds.'),
  ('CTRG99-B931', 'CTRG95-B2927', false, 'Herold''s Interpretations of the Civil code, 3rd ed.'),
  ('CTRG98-B763', 'CTRG95-B2927', true, 'Annotations supplementing Herold''s Interpretations of the civil code'),
  ('ocm15977008', 'ocm15976923', false, 'Outram''s legal lyrics, enlarged collection'),
  ('CTRG95-B3776', 'ocm15976923', false, 'Outram''s legal lyrics, later enlarged collection'),
  ('CTRG96-B1424', 'ocm21575457', false, 'Kain''s solicitors'' book-keeping by double entry, 12th ed.'),
  ('ocm32993316', 'CTRG96-B1652', false, 'Lewis''s Sheriff court practice, 1st ed.'),
  ('ocm33086971', 'CTRG96-B1652', false, 'Lewis''s Sheriff court practice, 2nd ed.'),
  ('CTRG97-B568', 'CTRG96-B2404', false, 'Bradbury''s workmen''s compensation, 2nd ed.'),
  ('CTRG96-B2461', 'CTRG96-B2404', false, 'Bradbury''s workmen''s compensation, 3rd ed.'),
  ('ocm23746199', 'CTRG96-B751', false, 'Freeth''s death duties, 1st ed.'),
  ('ocm23746177', 'CTRG96-B751', false, 'Freeth''s death duties, 2nd ed.'),
  ('CTRG99-B113', 'CTRG97-B1140', false, 'Pond''s Public utilities, 3rd ed.'),
  ('CTRG97-B1850', 'CTRG97-B1848', false, 'Rush''s equity pleading and practice, retitled 2nd ed.'),
  ('ocm22946587', 'ocm22865412', false, 'Beven''s Employers'' liability, 1st ed.'),
  ('CTRG97-B642', 'ocm22865412', false, 'Beven''s Employers'' liability, 3rd ed.'),
  ('ocm31373115', 'ocm30844966', false, 'Wigram on extrinsic evidence, American ed.'),
  ('CTRG98-B901', 'ocm30844966', false, 'Wigram on extrinsic evidence, 5th ed.'),
  ('ocm14127117', 'ocm14150479', true, 'Index to Ballard''s Law of real property'),
  ('ocm14127015', 'ocm14150479', true, 'Index-digest to Ballard''s Law of real property'),
  ('ocm14150498', 'ocm14150479', false, 'Ballard''s Annual law of real property, retitled continuation'),
  ('ocm17391146', 'ocm17391145', false, 'Griffith''s Married Women''s Property Act, 3rd ed.'),
  ('ocm16770398', 'ocm17391145', false, 'Griffith''s Married Women''s Property Acts, 4th ed.'),
  ('ocm17558590', 'ocm17558584', false, 'Crabb''s precedents in conveyancing, 1st-2nd eds.'),
  ('ocm18241364', 'ocm18241336', false, 'Lyon on bills of sale, 3rd ed.'),
  ('ocm18241403', 'ocm18241336', false, 'Lyon & Redman on bills of sale, 4th ed.'),
  ('ocm31820119', 'ocm18770125', true, 'Supplement to Burroughs'' Treatise on taxation'),
  ('ocm21732067', 'ocm20549557', false, 'Calvert''s Observations on joint-stock company suits, 2nd ed.'),
  ('ocm23535417', 'ocm23535452', false, 'Rogers on elections, 10th ed.'),
  ('ocm23535397', 'ocm23535452', false, 'Rogers on elections, 11th ed.'),
  ('ocm23535431', 'ocm23535452', false, 'Rogers on elections, 12th ed.'),
  ('ocm27176834', 'ocm23535452', false, 'Rogers on elections, 13th ed.'),
  ('ocm30379951', 'ocm23535452', false, 'Rogers on elections, 14th ed.'),
  ('ocm29948486', 'ocm26176723', false, 'Companion second volume of Alison''s criminal law of Scotland'),
  ('ocm31253192', 'ocm32905526', false, 'Mitford (Redesdale) on Chancery pleadings, 3rd ed.'),
  ('ocm31945692', 'ocm32905526', true, 'Pleadings illustrative of Redesdale''s Treatise on pleadings'),
  ('CTRG99-B339', 'CTRG00-B1468', false, 'Brown''s essays on unearned incomes, enlarged 2nd ed.'),
  ('CTRG95-B2474', 'ocm17593064', false, 'Urlin''s Handy book of trustees, revised ed.'),
  ('CTRG95-B2627', 'ocm22967719', false, 'Scrutton on charterparties, 6th-9th eds.'),
  ('CTRG95-B2946', 'CTRG95-B2936', false, 'Koren''s summaries of insanity laws, revised'),
  ('CTRG95-B3096', 'CTRG95-B3115', false, 'Gross on real estate brokers with 1917 supplement'),
  ('CTRG95-B3954', 'ocm20558747', false, 'Selover''s Negotiable instruments law, 2nd ed.'),
  ('CTRG99-B115', 'CTRG95-B4116', false, 'Tiffany''s Principal and agent, 2nd ed.'),
  ('CTRG99-B889', 'CTRG95-B4205', false, 'Griffin''s Chattel mortgages, 4th ed.'),
  ('CTRG95-B4280', 'ocm18395759', false, 'Cooper''s Defamation, 2nd ed.'),
  ('ocm21501938', 'CTRG95-B4664', false, 'Underhill''s Partnership, 1st ed.'),
  ('CTRG96-B1425', 'ocm32913368', false, 'Pridmore''s bills of costs, 11th ed.'),
  ('CTRG97-B698', 'CTRG96-B1632', false, 'Crew''s Secret commissions, 2nd ed.'),
  ('CTRG96-B200', 'CTRG96-B195', false, 'Watkins'' Shippers and carriers, later eds.'),
  ('CTRG96-B2297', 'ocm24451336', false, 'Smith''s Sheriffs, coroners and constables, later ed.'),
  ('CTRG96-B2630', 'ocm20880375', false, 'Cooke''s Combinations, retitled 2nd ed.'),
  ('CTRG97-B748', 'CTRG96-B722', false, 'Bolton''s Courts (Emergency Powers) Acts, 4th ed.'),
  ('CTRG97-B1137', 'ocm23094520', false, 'Nash''s Pleading and practice, 5th ed.'),
  ('CTRG97-B1333', 'CTRG97-B1307', false, 'Trotter''s Contract during war, 3rd ed.'),
  ('CTRG98-B2875', 'CTRG97-B1476', false, 'Payne''s Carriage of goods by sea, 2nd-3rd eds.'),
  ('CTRG98-B842', 'CTRG97-B1703', false, 'Harper''s Workmen''s compensation, 2nd ed.'),
  ('CTRG97-B1923', 'ocm23469698', false, 'Brickwood''s Sackett on instructions to juries, 3rd ed.'),
  ('CTRG97-B1922', 'ocm23469698', true, 'Illinois supplement to Brickwood''s Sackett on instructions'),
  ('CTRG97-B3041', 'CTRG98-B1146', false, 'Newman''s Kentucky pleading, 3rd ed.'),
  ('CTRG97-B697', 'ocm17391063', false, 'Gover''s Hints on title, 4th ed.'),
  ('CTRG98-B1075', 'CTRG97-B2537', true, 'Cases supplementary to Ames'' Cases in equity jurisdiction'),
  ('CTRG98-B3018', 'CTRG98-B3012', false, 'Cole''s Hague rules explained, updated for 1924 Act'),
  ('CTRG98-B3243', 'CTRG98-B755', false, 'Bradbury''s Rules of pleading, 2nd ed.'),
  ('CTRG98-B523', 'ocm23229031', false, 'Bispham''s Principles of equity, 6th-7th eds.'),
  ('CTRG99-B1588', 'ocm24358116', false, 'Giauque''s Manual for notaries, 5th ed.'),
  ('ocm13336694', 'ocm13336738', false, 'Angell on limitations, 1st ed.'),
  ('ocm13493663', 'ocm15670995', false, 'Woolsey on divorce, 2nd ed.'),
  ('ocm14919244', 'ocm14812990', false, 'Stirling''s Philosophy of law lectures, reissued with essays'),
  ('ocm14920226', 'ocm14920267', true, 'French translation of Spencer''s Justice'),
  ('ocm15600881', 'ocm31819903', false, 'Sawyer''s Merchant''s and shipmaster''s guide, 5th ed.'),
  ('ocm16701471', 'ocm16621269', false, 'Shelford on marriage and divorce, same 1841 work'),
  ('ocm16788367', 'ocm16788370', false, 'Adams on ejectment, 2nd ed.'),
  ('ocm17224518', 'ocm33894413', false, 'Thompson on building associations, 2nd ed.'),
  ('ocm20910551', 'ocm17372900', false, 'Taylor on private corporations, 1st ed.'),
  ('ocm17434083', 'ocm32601433', false, 'Woolrych on window lights, later ed.'),
  ('ocm17563388', 'ocm32042039', false, 'Theobald on wills, 1st ed.'),
  ('ocm20345487', 'ocm17738909', false, 'Fisher on mortgage, 1st ed.'),
  ('ocm17984880', 'ocm19042568', false, 'Read''s opinion on 1863 conscription act, reprint'),
  ('ocm18121354', 'ocm18121079', false, 'Van Santvoord''s Chief justices, 2nd ed.'),
  ('ocm18220906', 'ocm18220886', false, 'Hampson on trustees, 2nd ed.'),
  ('ocm32333130', 'ocm18395907', false, 'De Colyar on guarantees, American ed.'),
  ('ocm18893891', 'ocm32002973', false, 'Frieze''s suffrage history, same 1842 work'),
  ('ocm26771901', 'ocm20278287', false, 'Pollock''s Principles of contract, later editions'),
  ('ocm20345628', 'ocm32443823', false, 'Wordsworth on joint stock companies, 10th ed.'),
  ('ocm20537388', 'ocm20537379', false, 'Heyl''s United States duties on imports, 31st ed.'),
  ('ocm20624432', 'ocm20624511', false, 'Blackwell on tax titles, 1st ed.'),
  ('ocm23040767', 'ocm21488952', false, 'Theobald on principal and surety, enlarged with agency'),
  ('ocm21732146', 'ocm21732178', true, 'Appendix to Baily''s Doctrine of life-annuities'),
  ('ocm33921475', 'ocm22478844', false, 'Wait''s General principles of the law, same work'),
  ('ocm22657705', 'ocm23073725', false, 'Frend''s railway conveyancing precedents, 2nd ed.'),
  ('ocm32023449', 'ocm22738910', false, 'Hildyard on marine insurance, later ed.'),
  ('ocm22967713', 'ocm22967715', false, 'Scratchley on friendly societies, 13th ed.'),
  ('ocm23064200', 'ocm27639321', false, 'Tomlins'' Familiar explanation of wills, new ed.'),
  ('ocm23660001', 'ocm23659985', false, 'Hardcastle on statutes, 1st ed.'),
  ('ocm23831831', 'ocm23831869', false, 'Hudson on building contracts, 1st ed.'),
  ('ocm25782579', 'ocm32010086', false, 'Erichsen''s railway injuries, revised as Concussion of the spine'),
  ('ocm27766238', 'ocm27766168', false, 'Mackcoull''s Abuses of justice, 2nd ed.'),
  ('ocm28384958', 'ocm30600074', true, 'Epitome of Brandon''s Notes of practice of Mayor''s Court'),
  ('ocm28861317', 'ocm28861251', true, 'Both supplements to Alexander''s Abridgement; original not in collection'),
  ('ocm30427323', 'ocm29817782', false, 'Hare on discovery, 2nd ed.'),
  ('ocm31538780', 'ocm30347967', true, 'American notes intended to accompany Peake''s Compendium of evidence'),
  ('ocm31165728', 'ocm31793403', false, 'Eden on injunctions, 3rd ed.'),
  ('ocm32771109', 'ocm31429000', false, 'Espinasse on actions on statutes, enlarged ed.'),
  ('ocm31879274', 'ocm32775816', false, 'Prideaux on judgments, 2nd ed.'),
  ('ocm31962054', 'ocm31962090', true, 'Supplement to Dunlop''s Parochial law'),
  ('ocm32053383', 'ocm32162637', false, 'Broom''s Legal maxims, 1st ed.'),
  ('ocm32367760', 'ocm32367917', false, 'Cowen''s Justice of the peace, 1st ed.'),
  ('ocm32727954', 'ocm32727792', false, 'Stewart''s Blackstone-based property principles, 2nd ed.'),
  ('ocm32872764', 'ocm33018613', false, 'Smith''s Irish probate practice, 3rd ed.'),
  ('ocm33060576', 'ocm32944234', false, 'Cay''s Scottish Reform Act analysis, later ed.'),
  ('ocm14526545', 'ocm12139316', false, 'Tucker''s American edition of Blackstone''s Commentaries'),
  ('ocm12199318', 'ocm12139316', true, 'Analysis of Anthon''s abridgment of Blackstone''s Commentaries'),
  ('CTRG95-B3266', 'ocm12088512', true, 'Questions on Kent''s Commentaries'),
  ('ocm31401951', 'ocm12139316', true, 'Analytical abridgment of Blackstone''s Commentaries'),
  ('ocm12118791', 'ocm12139316', true, 'Abridgment of Blackstone''s Commentaries'),
  ('ocm12139445', 'ocm12139316', true, 'Blackstone abridged for students'),
  ('ocm14526681', 'ocm12139316', true, 'German translation of Gifford''s Blackstone abridgment'),
  ('ocm12118637', 'ocm12139316', true, 'Abridgment of Blackstone in letters'),
  ('ocm12822368', 'ocm12139316', true, 'Epitome of Blackstone''s Commentaries for schools'),
  ('ocm13866965', 'ocm12139316', true, 'Select extracts from Blackstone''s Commentaries'),
  ('ocm12130953', 'ocm12139316', true, 'Student''s Blackstone, selections from the Commentaries'),
  ('ocm16991015', 'ocm12139316', true, 'Blackstone adapted to Upper Canada law'),
  ('ocm11948748', 'ocm12139316', true, 'Compendium abridging Blackstone''s Commentaries'),
  ('ocm12131079', 'ocm12139316', false, 'Kerr''s edition of Blackstone''s Commentaries'),
  ('ocm32345410', 'ocm12139316', true, 'American students'' Blackstone, abridged'),
  ('ocm12116831', 'ocm12139316', true, 'Analytical charts of Blackstone'),
  ('ocm12801636', 'ocm12139316', true, 'Abridgment of Blackstone''s Commentaries'),
  ('ocm11949123', 'ocm12139316', true, 'Blackstone''s Commentaries reduced to questions and answers'),
  ('ocm12151990', 'ocm12088512', true, 'Kent''s Commentaries reduced to questions and answers'),
  ('ocm14371624', 'ocm12139316', true, 'Blackstone''s Commentaries reduced to questions and answers'),
  ('ocm32419651', 'ocm12088512', true, 'Analysis of Kent''s Commentaries'),
  ('ocm32770530', 'ocm12139316', true, 'Analysis of Blackstone''s Commentaries'),
  ('ocm14371646', 'ocm12139316', true, 'Analysis of Blackstone in questions'),
  ('ocm12618115', 'ocm12139316', true, 'Translation of quotations in Blackstone''s Commentaries'),
  ('ocm12822449', 'ocm12139316', true, 'Blackstone adapted to Ontario'),
  ('ocm13812717', 'ocm12139316', true, 'Kinne''s Blackstone questions and answers'),
  ('ocm32376258', 'ocm12139316', true, 'Kinne''s Blackstone questions and answers'),
  ('ocm12090401', 'ocm12088512', true, 'Kinne''s Kent questions and answers'),
  ('ocm13744794', 'ocm12139316', true, 'Companion and supplement to Blackstone''s Commentaries'),
  ('ocm23378623', 'ocm25295527', true, 'Supplement to Abbotts'' forms'),
  ('ocm23378465', 'ocm25295527', true, 'New supplement to Abbotts'' forms'),
  ('ocm31958598', 'ocm25295527', false, 'Same Abbotts'' forms of practice and pleading'),
  ('CTRG99-B395', 'ocm23259332', false, '4th ed. of Brief on modes of proving facts'),
  ('ocm17348453', 'ocm17348419', true, 'Continuation digest of corporation cases from July 1868'),
  ('CTRG98-B3218', 'CTRG98-B1915', true, 'Supplement to 545 United States tax cases'),
  ('CTRG98-B3219', 'CTRG98-B1915', true, 'Second supplement to 545 United States tax cases'),
  ('ocm21010936', 'ocm21010963', false, 'Later editions of Wharton''s criminal law treatise'),
  ('CTRG96-B3744', 'ocm22487106', false, '3rd ed. of Wharton on homicide'),
  ('ocm17011550', 'ocm32419070', true, 'Casebook arranged to accompany Elliott''s Outline of insurance'),
  ('ocm17332360', 'ocm17320829', false, 'Retitled 3rd+ ed. of Principles of private corporations'),
  ('ocm21131364', 'ocm17320829', true, 'Cases to accompany Principles of private corporations'),
  ('CTRG99-B1027', 'ocm18863717', false, '3rd ed. of Principles of public corporations retitled'),
  ('momlextra006', 'momlextra007', true, 'Shorter selection of Beale''s conflict of laws cases'),
  ('CTRG97-B335', 'ocm23280183', true, 'Notes to Beale''s Criminal pleading and practice'),
  ('ocm15258506', 'ocm20278275', true, 'Casebook for use with Pollock on torts'),
  ('CTRG98-B2129', 'ocm20278275', true, 'Analysis of Pollock''s Law of torts'),
  ('ocm20278282', 'ocm20278275', false, 'American ed. of Pollock''s Law of torts'),
  ('ocm31997685', 'ocm26146761', false, 'American ed. of Taylor''s Manual of medical jurisprudence'),
  ('ocm23045023', 'ocm23045004', true, 'Supplement to Woolrych''s law of waters and sewers'),
  ('ocm21508227', 'ocm23045004', false, '2nd ed. of Woolrych''s law of waters'),
  ('ocm21732219', 'ocm21732230', false, 'Earlier ed. of Dixon''s Law of the farm'),
  ('CTRG98-B2422', 'CTRG98-B2621', false, '4th/5th ed. of Spencer''s Agricultural Holdings Acts'),
  ('CTRG98-B2730', 'CTRG98-B2621', false, '6th ed. of Spencer''s Agricultural Holdings Acts'),
  ('CTRG98-B2733', 'CTRG98-B2625', false, '2nd ed. of Spencer''s Small Holdings and Allotments Acts'),
  ('ocm20369664', 'ocm20369504', false, '3rd ed. of Fraser on master and servant'),
  ('ocm30426650', 'ocm32807746', false, 'Chalmers''s 3rd ed. of Wilson''s Judicature Acts'),
  ('ocm30426694', 'ocm32807746', false, 'Burney''s 6th ed. of Wilson''s Judicature Acts'),
  ('ocm33031458', 'ocm28034672', false, '1st ed. of Dove Wilson''s sheriff court practice'),
  ('CTRG98-B1045', 'CTRG95-B2273', false, '2nd ed. of Kales on future interests in Illinois'),
  ('ocm19522945', 'ocm19522955', false, 'Earlier eds. of Chalmers''s Digest of bills of exchange'),
  ('ocm19522952', 'ocm19522955', true, 'American adaptation of Chalmers''s Digest of bills of exchange'),
  ('CTRG95-B4270', 'ocm32764683', false, '6th/7th eds. of Bunyon''s Law of fire insurance'),
  ('ocm19678964', 'ocm20446929', true, 'Supplement to Bunyon''s Law of life assurance'),
  ('CTRG96-B2050', 'ocm17320613', false, '2nd ed. of Tompkins''s corporations casebook'),
  ('ocm21321336', 'ocm21321380', false, 'Earlier eds. of Kerr on receivers'),
  ('ocm29763729', 'ocm21321380', false, 'American ed. of Kerr on receivers'),
  ('CTRG96-B879', 'ocm21321380', false, '5th-7th eds. of Kerr on receivers'),
  ('ocm23113944', 'ocm23113953', false, '1st ed. of Troubat''s Pennsylvania practice'),
  ('ocm22478603', 'ocm23113953', false, '5th ed. of Troubat''s Pennsylvania practice'),
  ('CTRG97-B251', 'ocm23113953', false, '6th ed. of Troubat''s Pennsylvania practice, retitled'),
  ('ocm19280974', 'ocm14234970', false, '4th/5th eds. of Gerard''s Titles to real estate'),
  ('ocm17348436', 'ocm17176502', true, 'Supplement to Bishop''s treatise on insolvent debtors'),
  ('ocm14442576', 'ocm14442518', true, 'Supplement to Petersdorff''s Concise practical abridgment'),
  ('ocm28146293', 'ocm29739844', false, '1st ed. of Harris''s Hints on advocacy'),
  ('ocm32463758', 'ocm29739844', false, 'American ed. of Harris''s Hints on advocacy'),
  ('ocm21320083', 'ocm21541325', false, '3rd ed. of Henley''s bankrupt law treatise, retitled digest'),
  ('ocm23832138', 'ocm23831901', false, '7th ed. of Hudson''s guide to legacy duties'),
  ('ocm33083005', 'ocm23831901', false, '8th ed. of Hudson''s guide to legacy duties'),
  ('ocm23832171', 'ocm23831901', false, '10th ed. of Hudson''s guide to legacy duties'),
  ('CTRG96-B3640', 'CTRG96-B3660', false, 'Wyman''s 2nd/3rd eds. of Beale and Wyman''s cases'),
  ('ocm20312885', 'ocm20312897', false, '1st ed. of Smith''s Handy book on joint stock companies'),
  ('CTRG95-B2802', 'ocm20312897', false, 'Later ed. of Smith''s joint stock companies book'),
  ('CTRG95-B3294', 'CTRG95-B3879', false, 'Enlarged ed. of Tiffany''s Law of real property'),
  ('CTRG98-B1032', 'CTRG96-B574', false, '2nd ed. of Nellis on street railroads'),
  ('CTRG96-B247', 'ocm15578121', false, '2nd ed. of Jones on pledges'),
  ('CTRG96-B158', 'ocm15578121', false, '3rd ed. of Jones on pledges, retitled'),
  ('CTRG96-B907', 'ocm26002594', false, '3rd ed. of Black''s parochial ecclesiastical law of Scotland'),
  ('CTRG97-B2437', 'ocm17535630', false, '20th ed. of Prideaux''s Precedents in conveyancing'),
  ('ocm32900639', 'ocm32871867', false, '2nd ed. of White''s Merchant shipping acts'),
  ('CTRG97-B677', 'ocm32871867', false, '3rd ed. of White''s Merchant shipping acts'),
  ('CTRG98-B1300', 'CTRG98-B1171', true, 'Students'' abridgment of Willoughby''s Constitutional law'),
  ('CTRG98-B3180', 'ocm22461813', false, '3rd ed. of Shipman''s Common-law pleading'),
  ('ocm32357361', 'ocm15250811', false, '2nd ed. of Cooley on torts'),
  ('ocm18432660', 'ocm18423369', false, '1st ed. of Hayes''s Introduction to conveyancing'),
  ('ocm19520153', 'ocm18423369', false, '3rd/5th eds. of Hayes''s Introduction to conveyancing'),
  ('ocm30802003', 'ocm30800785', true, 'Supplement to Simmons on courts martial'),
  ('ocm19534900', 'ocm30800785', false, '7th ed. of Simmons on courts martial'),
  ('ocm23746743', 'ocm23746759', false, '2nd ed. of Glen''s Law relating to highways'),
  ('ocm31373090', 'ocm31373021', false, 'American ed. of White and Tudor''s Leading cases in equity'),
  ('ocm31898292', 'ocm31898426', true, 'Supplement to Gabbett''s Digested abridgment'),
  ('CTRG95-B1335', 'CTRG95-B2950', false, 'Same constitution and by-laws, later printing'),
  ('CTRG95-B2546', 'CTRG97-B2360', true, 'Abridgment of Salmond''s Law of torts'),
  ('CTRG95-B4107', 'CTRG95-B2862', false, 'Same monograph on valuation of tenant right'),
  ('CTRG95-B3308', 'CTRG95-B3302', false, 'Enlarged ed. of Borland on wills'),
  ('CTRG95-B3450', 'ocm17423441', false, '6th ed. of Hunt''s law of boundaries'),
  ('CTRG95-B3705', 'ocm13879773', false, 'Later eds. of Carter''s legal history, retitled'),
  ('CTRG95-B3947', 'ocm15542425', false, '2nd ed. of Huffcut''s Negotiable instruments'),
  ('CTRG95-B4176', 'ocm23073728', false, '7th ed. of Fletcher''s Dilapidations, retitled'),
  ('CTRG95-B4279', 'ocm18218279', false, '3rd ed. of Currie''s Confirmation of executors'),
  ('ocm31932044', 'CTRG96-B1413', false, '2nd ed. of Summerhays''s Precedents of bills of costs'),
  ('CTRG96-B1577', 'CTRG96-B926', false, 'Later issue of Alexander''s Administration of justice'),
  ('CTRG96-B172', 'ocm15589886', false, '4th ed. of Norton''s Bills and notes'),
  ('CTRG96-B1923', 'CTRG98-B433', false, 'Same work, retitled Invalid legislation'),
  ('CTRG96-B303', 'CTRG96-B304', false, '2nd ed. of Nims on unfair competition'),
  ('CTRG96-B781', 'ocm27589682', false, '3rd ed. of Jelf''s Corrupt Practices Acts'),
  ('CTRG97-B1249', 'CTRG97-B438', false, '3rd ed. of Fyfe''s Employers & workmen'),
  ('CTRG98-B2205', 'CTRG97-B1470', false, '2nd ed. of Firminger''s Workmen''s Compensation Act'),
  ('CTRG97-B1689', 'ocm17800371', false, '2nd ed. of Reno on employers'' liability acts'),
  ('CTRG97-B2573', 'ocm21588526', true, 'Analysis of Carver''s Carriage of goods by sea'),
  ('CTRG97-B2881', 'ocm32989032', false, '2nd ed. of Osborne''s Irish county courts practice'),
  ('CTRG97-B668', 'ocm31855385', false, '2nd ed. of Short''s Crown side practice'),
  ('CTRG98-B1335', 'ocm32902623', false, '2nd ed. of Conner''s Fisheries (Ireland) Acts'),
  ('CTRG98-B3000', 'ocm27176769', false, '2nd ed. of Baty''s First elements'),
  ('CTRG98-B3240', 'ocm31991928', false, '9th ed. of Bryant''s justices of the peace'),
  ('ocm12126097', 'ocm15727594', false, 'Same Comic Blackstone'),
  ('ocm12486400', 'ocm12437876', false, 'New ed. of Iredell''s digested manual'),
  ('ocm13491530', 'ocm13491548', false, '2nd ed. of Stow''s Probate confiscation'),
  ('ocm14074974', 'ocm15556581', false, '4th ed. of Reeve''s Domestic relations'),
  ('ocm14104216', 'ocm14290256', false, '2nd ed. of Sedgwick on trial of title to land'),
  ('ocm14654982', 'ocm14646444', false, 'Later eds. of Hallilay''s digest of examination questions'),
  ('ocm15229441', 'ocm15189488', false, '4th/5th eds. of W.W. Story on contracts'),
  ('ocm32373120', 'ocm15495059', false, 'Same Freedley''s Legal adviser'),
  ('ocm15732052', 'ocm15728286', false, 'Roscoe''s Lives of eminent British lawyers reissued'),
  ('ocm16875455', 'ocm16875462', false, '1st ed. of Kerr on fraud and mistake'),
  ('ocm17154002', 'ocm32607708', false, '2nd ed. of Niblack on voluntary societies'),
  ('ocm18832575', 'ocm17355900', true, 'Supplement to Cutler''s Insolvent laws of Massachusetts'),
  ('ocm17427704', 'ocm32775723', false, '2nd ed. of Prater on husband and wife'),
  ('ocm22659068', 'ocm17563229', true, 'Supplement to Roberts''s Treatise on wills and codicils'),
  ('ocm17594093', 'ocm31765103', false, '3rd ed. of Innes''s Digest of easements'),
  ('ocm17738906', 'ocm18208077', false, '2nd ed. of Field''s Landholding'),
  ('ocm18712518', 'ocm17971495', false, 'Parsons''s rights of a citizen, retitled'),
  ('ocm18038737', 'ocm25754827', true, 'Spanish translation of The Federalist'),
  ('ocm18038749', 'ocm25754827', true, 'Portuguese translation of The Federalist'),
  ('ocm18692803', 'ocm18102316', false, '2nd ed. of Duer''s Outlines, retitled Course of lectures'),
  ('ocm18237209', 'ocm18197307', false, 'Earlier eds. of Brown''s Law of fixtures'),
  ('ocm32845300', 'ocm18278157', false, '4th ed. of Clerke''s Conveyancing acts'),
  ('ocm18620322', 'ocm18606458', false, '15th ed. of Kingsbury''s Maine townsman'),
  ('ocm18706938', 'ocm19102670', false, 'Later ed. of Morrison''s New Hampshire town officer'),
  ('ocm19505506', 'ocm19505490', false, '2nd ed. of Saunders on negligence'),
  ('ocm20278202', 'ocm32174805', false, '1st ed. of Newland on contracts in equity'),
  ('ocm20345527', 'ocm32151272', false, '3rd ed. of Forsyth on composition with creditors'),
  ('ocm20425319', 'ocm20425416', false, '1st ed. of Melsheimer''s Stock Exchange'),
  ('ocm26050163', 'ocm20534799', false, 'Same Cory''s Practical treatise on accounts'),
  ('ocm22461756', 'ocm22461761', false, '3rd ed. of Riddle on supplementary proceedings'),
  ('ocm22656770', 'ocm32772234', false, '4th ed. of Fowler on collieries'),
  ('ocm30766935', 'ocm22738758', false, 'Same Hallam''s Constitutional history'),
  ('ocm31912405', 'ocm22946522', false, '2nd ed. of Broom''s Constitutional law'),
  ('ocm32950949', 'ocm23044865', false, '5th ed. of Williams''s law of auctions, retitled'),
  ('ocm23608983', 'ocm32772378', false, '2nd ed. of Glover on municipal corporations'),
  ('ocm23713216', 'ocm23713219', false, '2nd ed. of Mattinson on corrupt practices'),
  ('ocm31997508', 'ocm24484086', false, 'Later eds. of Swan''s Ohio justices treatise'),
  ('ocm24876669', 'ocm25420667', false, '1st ed. of Olcott''s Louisiana magistrate'),
  ('ocm31995726', 'ocm25638603', false, '3rd ed. of Crary''s special proceedings'),
  ('ocm27176859', 'ocm25699832', false, '3rd ed. of Romilly''s Observations'),
  ('ocm26124179', 'ocm26124159', true, 'Supplement to Shelford on lunatics'),
  ('ocm31771360', 'ocm30326693', false, '2nd ed. of Morgan and Davey''s Costs in Chancery'),
  ('ocm31898831', 'ocm31165515', false, 'Later ed. of Harrison''s Chancery practice'),
  ('ocm31765463', 'ocm31765480', false, '1st ed. of Hallilay''s Articled clerks'' hand-book'),
  ('ocm31878884', 'ocm31945647', false, '2nd ed. of Watson on sheriffs'),
  ('ocm31942913', 'ocm31942871', false, '2nd ed. of Mullins on the magistracy'),
  ('ocm32048018', 'ocm32148254', false, 'Earlier eds. of Redman''s Landlord and tenant'),
  ('ocm32325148', 'ocm32325091', false, 'American eds. of Russell on crimes'),
  ('ocm32967896', 'ocm32533206', true, 'Supplement to Hutcheson''s treatise on justice of peace'),
  ('ocm32869637', 'ocm32989278', false, '5th ed. of Nolan''s Irish landlord and tenant statutes'),
  ('ocm33008827', 'ocm33084999', false, '3rd ed. of Guthrie Smith''s poor law digest'),
  ('ocm22737145', 'ocm22461761', true, 'Supplement to Riddle''s supplementary proceedings treatise'),
  ('CTRG99-B123', 'CTRG99-B3', true, 'Supplement to Carmody''s New York practice'),
  ('ocm31904568', 'ocm31727074', true, 'Supplement to Chisholm-Batten''s county courts equity treatise'),
  ('ocm26716691', 'ocm26716673', true, 'Supplement to Palmer''s Practice on appeals from colonies'),
  ('CTRG97-B2526', 'ocm23234435', false, 'Wait''s justices'' courts practice, 8th ed. retitled'),
  ('CTRG96-B2456', 'ocm23210669', false, 'Ames''s cases on pleading, 1905 edition'),
  ('CTRG95-B2291', 'ocm15314297', false, 'Ames''s cases on suretyship, later issue'),
  ('ocm32041650', 'ocm18423363', false, 'Hayes & Jarman''s concise forms of wills, later eds'),
  ('ocm32048651', 'ocm18423363', false, 'Hayes & Jarman''s concise forms of wills, 5th ed.'),
  ('ocm32536484', 'ocm18423363', false, 'Hayes & Jarman''s concise forms of wills, 11th ed.'),
  ('CTRG95-B4015', 'ocm18423363', false, 'Hayes & Jarman''s concise forms of wills, 12th-13th eds'),
  ('ocm32056336', 'ocm18423363', false, 'Hayes & Jarman''s concise forms of wills, 10th ed.'),
  ('CTRG95-B3977', 'ocm18423363', false, 'Hayes & Jarman''s concise forms of wills, 14th ed.'),
  ('ocm25044113', 'ocm18240872', true, 'General index to Jarman on wills'),
  ('ocm17352866', 'ocm17367593', false, 'Cook on stock and stockholders, 1st ed.'),
  ('ocm17773356', 'ocm17367593', false, 'Cook on stock and stockholders, 2nd ed.'),
  ('ocm25127575', 'ocm17367593', false, 'Cook on stock and stockholders, 3rd ed.'),
  ('CTRG96-B3115', 'CTRG96-B3053', false, 'Montgomery''s annual Income tax procedure, 1918'),
  ('CTRG97-B951', 'CTRG96-B3053', false, 'Montgomery''s annual Income tax procedure, 1919'),
  ('CTRG97-B1127', 'CTRG96-B3053', false, 'Montgomery''s annual Income tax procedure, 1920'),
  ('CTRG98-B1188', 'CTRG96-B3053', false, 'Montgomery''s annual Income tax procedure, 1921'),
  ('CTRG99-B352', 'CTRG99-B1204', false, 'Montgomery''s Excess profits tax procedure, 1921 ed.'),
  ('ocm13592721', 'ocm23073733', false, 'Ridgway''s Erskine speeches, American reprint'),
  ('ocm13592652', 'ocm23073733', false, 'Companion fifth volume of Ridgway''s Erskine speeches'),
  ('ocm13592634', 'ocm23073733', false, 'Ridgway''s Erskine speeches, enlarged 1847 edition'),
  ('ocm13744760', 'ocm14341658', false, 'Leading cases made easy is Shirley''s 1st ed.'),
  ('CTRG96-B1623', 'ocm26156649', false, 'Warburton''s criminal law leading cases, later eds'),
  ('ocm13901712', 'ocm14207268', false, '1st ed. of Hilliard''s American law of real property'),
  ('CTRG95-B2894', 'ocm20312837', false, 'Sebastian''s Law of trade marks, 5th ed.'),
  ('CTRG98-B2872', 'CTRG98-B1961', false, 'Sebastian''s Law of trade mark registration, 2nd ed.'),
  ('ocm31813876', 'ocm25127975', false, 'Caruthers'' History of a lawsuit'),
  ('ocm22426784', 'ocm25127975', false, 'Caruthers'' History of a lawsuit'),
  ('CTRG96-B2282', 'ocm25127975', false, 'Caruthers'' History of a lawsuit, 4th ed.'),
  ('CTRG98-B873', 'ocm25127975', false, 'Caruthers'' History of a lawsuit, 5th ed.'),
  ('ocm31141110', 'ocm18925648', false, 'Goodwin''s Town officer, 4th ed. by Thomas'),
  ('ocm19228578', 'ocm18698307', false, 'Thomas'' town officer, new ed.'),
  ('ocm23087323', 'ocm25098937', false, 'Fiero''s special proceedings, 2nd ed.'),
  ('ocm20334863', 'ocm20334910', false, 'Underhill''s Summary of torts, 2nd ed.'),
  ('ocm18413279', 'ocm20334910', false, 'American ed. of Underhill''s torts'),
  ('CTRG95-B4281', 'ocm20334910', false, 'Canadian ed. of Underhill''s torts retitled'),
  ('ocm33888711', 'ocm30808531', true, 'US adaptation of Stephen''s Digest of evidence'),
  ('ocm25044335', 'ocm23475652', false, 'Reynolds'' Theory of evidence, 3rd ed.'),
  ('CTRG96-B2102', 'ocm23475652', false, 'Reynolds'' Theory of evidence, 4th ed.'),
  ('CTRG97-B615', 'ocm32777884', false, 'Carson''s retitled continuation of Shelford''s Real property statutes'),
  ('ocm13275407', 'ocm13509560', false, 'Bishop on marriage and divorce, 4th ed.'),
  ('ocm13275595', 'ocm13509560', false, 'Bishop on marriage and divorce, 5th-6th eds'),
  ('ocm23849480', 'ocm23849485', false, 'Shelford''s tithe commutation acts, 3rd ed.'),
  ('ocm22620108', 'ocm22620123', false, 'Brewster''s Pennsylvania practice, 2nd ed.'),
  ('ocm23899446', 'ocm23899413', false, 'Lloyd on compensation, 4th ed.'),
  ('ocm32139224', 'ocm23899413', false, 'Lloyd on compensation, 5th ed.'),
  ('ocm28161643', 'ocm23899413', false, 'Lloyd on compensation, 6th ed.'),
  ('ocm31629372', 'ocm33267046', false, 'Paine''s New York banking laws, later ed.'),
  ('ocm32608050', 'ocm33267046', false, 'Paine''s New York banking laws, later ed.'),
  ('CTRG98-B974', 'CTRG96-B3722', true, 'Annotations supplementing Lust''s Loss and damage claims'),
  ('CTRG95-B2810', 'CTRG98-B2804', false, 'Connell''s Agricultural Holdings (Scotland) Acts, 2nd ed.'),
  ('ocm17023719', 'ocm17023727', false, 'Bacon on benefit societies, 1st ed.'),
  ('CTRG95-B3104', 'ocm17023727', false, 'Bacon on benefit societies, 4th ed. retitled'),
  ('CTRG95-B3296', 'ocm14062805', false, 'Boone''s real property, 2nd ed.'),
  ('CTRG95-B4592', 'CTRG95-B4227', false, 'Gore-Browne''s Concise precedents, 3rd ed.'),
  ('CTRG97-B713', 'CTRG95-B4227', false, 'Gore-Browne''s Concise precedents, 4th ed.'),
  ('CTRG98-B2237', 'ocm32876365', false, 'Fulton on patents, 3rd ed.'),
  ('CTRG96-B1584', 'ocm32876365', false, 'Fulton on patents, 4th ed.'),
  ('CTRG96-B1988', 'CTRG96-B1986', false, 'Clark & Marshall on private corporations, further volumes'),
  ('CTRG96-B2455', 'ocm17854406', false, 'Bailey''s personal injuries, 2nd ed.'),
  ('CTRG96-B946', 'ocm32157005', true, 'Abridgment of Phipson''s Law of evidence'),
  ('CTRG98-B1428', 'ocm32157005', true, 'Abridgment of Phipson''s Law of evidence, 3rd ed.'),
  ('CTRG98-B1418', 'CTRG98-B2416', false, 'Jackson''s Agricultural Holdings Acts, 2nd ed.'),
  ('CTRG97-B1477', 'CTRG98-B2416', false, 'Jackson''s Agricultural Holdings Acts, 3rd-4th eds'),
  ('CTRG98-B3202', 'CTRG98-B830', false, 'McMichael''s leaseholds, 2nd ed. retitled'),
  ('CTRG98-B3203', 'CTRG98-B830', false, 'McMichael''s leaseholds, 3rd ed.'),
  ('ocm23274574', 'ocm23283997', false, 'Hilliard''s Remedies for torts, 2nd ed.'),
  ('ocm16901571', 'ocm16895643', false, 'Appendix issued with The Court of Session garland'),
  ('ocm16895641', 'ocm16895643', true, 'Supplement to The Court of Session garland'),
  ('ocm18435177', 'ocm17732856', false, 'Cameron''s intestate succession in Scotland, 2nd ed.'),
  ('ocm20312918', 'ocm19535453', false, 'Sugden''s Vendors and purchasers, 5th ed.'),
  ('ocm22669166', 'ocm19535453', true, 'Concise view abridging Sugden''s Vendors and purchasers'),
  ('ocm23234782', 'ocm23113985', false, 'Van Santvoord''s equity practice, 2nd ed.'),
  ('ocm23234732', 'ocm23113985', false, 'Van Santvoord''s equity practice, 3rd ed.'),
  ('ocm24452192', 'ocm32028465', false, 'W.R. Smith''s justices of the peace, 3rd ed.'),
  ('ocm26716777', 'ocm26716757', false, 'Parker''s Notes on arbitration, 2nd ed.'),
  ('ocm32990042', 'ocm26716757', false, 'Parker''s Notes on arbitration, enlarged 2nd ed.'),
  ('ocm32048752', 'ocm32138140', false, 'Thring''s joint-stock companies, 2nd-3rd eds'),
  ('ocm32138636', 'ocm32138140', false, 'Thring''s joint-stock companies, 4th-5th eds'),
  ('CTRG95-B3208', 'CTRG95-B2327', true, 'Teacher''s handbook to accompany Gano''s Commercial law'),
  ('CTRG95-B2584', 'ocm33059987', false, 'Brown on sale of goods, 2nd ed. retitled'),
  ('CTRG97-B1530', 'CTRG95-B2761', false, 'Nicolas on formation of companies, 3rd ed.'),
  ('CTRG95-B2956', 'CTRG95-B2959', true, 'Supplement to Greenwood''s Law relating to trade unions'),
  ('CTRG99-B1357', 'CTRG95-B3127', false, 'Williston on sales, 2nd ed.'),
  ('CTRG99-B1070', 'CTRG95-B3209', false, 'Goddard''s cases on principal and agent, 2nd ed.'),
  ('CTRG95-B3609', 'ocm16875445', false, 'Lush on husband and wife, 3rd ed.'),
  ('CTRG95-B3730', 'CTRG97-B1448', false, 'Farrer''s Precedents of conditions of sale, reissue'),
  ('CTRG95-B4118', 'ocm31856225', false, 'Tucker''s Massachusetts corporations manual, 2nd ed.'),
  ('CTRG95-B4223', 'ocm18403383', false, 'Fuller on friendly societies, 3rd ed.'),
  ('CTRG97-B1440', 'CTRG95-B4698', false, 'Copnall on highways, 2nd ed. retitled'),
  ('CTRG96-B1455', 'ocm31730390', false, 'Dixon on probate and administration, 3rd ed.'),
  ('CTRG96-B1650', 'CTRG96-B1639', false, 'Wilshere''s Outlines of procedure, 2nd ed.'),
  ('CTRG99-B66', 'CTRG96-B181', false, 'Wrightington on unincorporated associations, 2nd ed.'),
  ('CTRG96-B2557', 'ocm20579741', false, 'Talbot''s Degeneracy, later issue'),
  ('CTRG98-B3191', 'CTRG96-B378', false, 'McKay on community property, 2nd ed.'),
  ('CTRG98-B2069', 'CTRG96-B727', false, 'Highmore''s Customs laws, 2nd ed.'),
  ('CTRG96-B937', 'ocm31758581', false, 'Everest and Strode''s Law of estoppel, later eds'),
  ('CTRG97-B1144', 'CTRG97-B948', false, 'Langdell''s Brief survey of equity jurisdiction, 2nd ed.'),
  ('CTRG97-B1338', 'ocm32900502', false, 'Temperley''s Merchant Shipping Acts, 2nd-3rd eds'),
  ('CTRG97-B346', 'CTRG97-B235', false, 'Part of Loyd''s Cases on civil procedure'),
  ('CTRG97-B493', 'CTRG97-B2797', false, 'Browning on registration of title in Ireland, 2nd ed.'),
  ('ocm22165937', 'CTRG97-B3084', false, 'Foster''s Federal practice, 2nd ed.'),
  ('CTRG97-B707', 'CTRG98-B1202', false, 'Chandler''s Trust accounts, 2nd ed.'),
  ('CTRG99-B1474', 'CTRG98-B1228', false, 'Model city charter, final ed.'),
  ('CTRG98-B1487', 'CTRG98-B1486', false, 'Montgomery''s licensing laws, 7th ed. retitled'),
  ('CTRG98-B1916', 'CTRG98-B1909', true, 'Supplement to Chandler''s Express trusts'),
  ('CTRG98-B583', 'ocm31538879', false, 'Benedict''s American admiralty, 4th-5th eds'),
  ('CTRG99-B1020', 'ocm25127705', false, 'Abbott''s Principles and forms of practice, 3rd ed.'),
  ('ocm13045061', 'ocm13276479', false, 'The attorney (Quod correspondence), 3rd ed.'),
  ('ocm13150003', 'ocm31495933', false, 'Webster''s Plymouth discourse, 2nd ed.'),
  ('ocm13721481', 'ocm14405212', false, 'Reeves'' History of the English law, 3rd ed.'),
  ('ocm14207298', 'ocm14092940', false, 'McAdam on landlord and tenant, 1st ed.'),
  ('ocm14152846', 'ocm14290177', false, 'Willard on real estate, later ed.'),
  ('ocm14813059', 'ocm14813060', false, 'Warren''s Duties of attorneys, spelling variant'),
  ('ocm14981296', 'ocm14964274', false, 'Broom''s Philosophy of law, 3rd ed. retitled'),
  ('ocm15235615', 'ocm15235621', false, 'Beach on contributory negligence, 3rd ed.'),
  ('ocm15621363', 'ocm15616812', false, 'Edwards on bills and notes, 3rd ed.'),
  ('ocm17372949', 'ocm17253709', false, 'Gluck on receivers of corporations, 2nd ed.'),
  ('ocm17530219', 'ocm28035311', false, 'Godefroi on trusts, 1st ed. as Digest'),
  ('ocm17568064', 'ocm32838927', true, 'Abridgment of Preston''s Abstracts of title'),
  ('ocm17627997', 'ocm17733579', false, 'Cornish''s Purchase deeds, new ed.'),
  ('ocm17865264', 'ocm17854788', false, 'Jameson''s Constitutional convention, 4th ed. retitled'),
  ('ocm18080936', 'ocm18034273', true, 'Supplement to Browne on trade-marks'),
  ('ocm18038777', 'ocm31690350', false, 'Stevens'' Sources of the Constitution, 2nd ed.'),
  ('ocm18122109', 'ocm18122068', false, 'Webster text book compilation retitled for 1861'),
  ('ocm32534087', 'ocm18237799', false, 'Dart''s Vendors and purchasers, later eds retitled'),
  ('ocm32913022', 'ocm18396492', false, 'Plumptre''s Summary of simple contracts, later ed.'),
  ('ocm18755771', 'ocm19030231', false, 'Dillon on municipal corporations, 1st-2nd eds'),
  ('ocm21847511', 'ocm19528752', true, 'Supplement to Lindley on partnership'),
  ('ocm20369690', 'ocm32880820', false, 'Gibbons on contracts for works, 2nd ed.'),
  ('ocm20425730', 'ocm20425620', false, 'Moore''s Abstracts of titles, 4th ed.'),
  ('ocm32534043', 'ocm20549502', true, 'Addenda to Cooke''s Bankrupt laws, 4th ed.'),
  ('ocm24730340', 'ocm20718436', false, 'Hammond''s justice of the peace, same book'),
  ('ocm20900079', 'ocm20900061', false, 'Lewis and Bombaugh''s Stratagems, 2nd ed.'),
  ('ocm21320279', 'ocm21575260', false, 'Hunt on fraudulent conveyances, 2nd ed.'),
  ('ocm21540722', 'ocm32041676', false, 'Grant on corporations, later ed.'),
  ('ocm32473029', 'ocm21847538', false, 'Locke on foreign attachment, later issue'),
  ('ocm22613051', 'ocm31962112', false, 'Dunlop on Scottish poor law, new ed.'),
  ('ocm22659031', 'ocm22659043', false, 'Redman on railway carriers, 2nd ed.'),
  ('ocm22884431', 'ocm32056171', false, 'Hayes''s Concise conveyancer, 2nd-3rd eds'),
  ('ocm23864126', 'ocm23019844', false, 'Taswell-Langmead''s English constitutional history'),
  ('ocm23662088', 'ocm32777918', false, 'Shepherd on parliamentary elections, 3rd ed.'),
  ('ocm23834600', 'ocm23834354', false, 'Pope''s Abridgment of custom and excise laws, 3rd ed.'),
  ('ocm32767742', 'ocm23963494', false, 'Cox''s Registration and elections, 12th ed.'),
  ('ocm24768326', 'ocm31165662', false, 'Hening''s Virginia justice, 4th ed.'),
  ('ocm25647742', 'ocm25647753', false, 'Clifford''s guide for administrators, 2nd ed.'),
  ('ocm27270182', 'ocm28384871', false, 'Christian''s dissertation on Lords'' evidence, retitled 2nd ed.'),
  ('ocm30152924', 'ocm29819482', false, 'Kerr on injunctions, 1st ed.'),
  ('ocm29963619', 'ocm29963578', false, 'Another English version of Cottu''s same work'),
  ('ocm30369445', 'ocm31784108', false, 'Philips''s Letters on special pleading, 2nd ed.'),
  ('ocm31727436', 'ocm30600482', false, 'Cunningham''s Precedents of pleading, 2nd ed.'),
  ('ocm31674531', 'ocm31674555', false, 'Beames on ne exeat, 1st ed.'),
  ('ocm33240575', 'ocm32435474', true, 'Halleck''s abridgment of his International law'),
  ('ocm32753002', 'ocm32753027', false, 'Addison on torts, 7th-8th eds'),
  ('ocm32875267', 'ocm32875496', false, 'Forsyth''s Hortensius, 3rd ed.'),
  ('ocm33064467', 'ocm32944373', false, 'Craigie''s Scottish conveyancing: heritable rights, 1st ed.'),
  ('ocm18432106', 'CTRG95-B4454', false, 'Leake on contracts, 1st ed. as Elements'),
  ('ocm18432087', 'CTRG95-B4454', false, 'Leake on contracts, 2nd ed. as Elementary digest'),
  ('ocm18423383', 'CTRG95-B4454', false, 'Leake on contracts, 3rd ed. as Digest of principles'),
  ('CTRG95-B4703', 'CTRG95-B4454', false, 'Leake on contracts, 6th ed. by Randall'),
  ('CTRG95-B1571', 'CTRG95-B2042', false, 'Benjamin''s principles of contract, combined edition'),
  ('ocm31756896', 'ocm31727263', false, 'Coote''s common form probate practice, 1st ed.'),
  ('ocm31942993', 'ocm31727263', false, 'Coote''s probate practice, 6th ed.'),
  ('ocm31756916', 'ocm31727263', false, 'Coote''s common form practice, 7th-9th eds.'),
  ('ocm31756928', 'ocm31727263', false, 'Coote and Tristram, 11th ed.'),
  ('ocm31758192', 'ocm31727263', false, 'Coote and Tristram, 12th and 15th eds.'),
  ('CTRG97-B1276', 'ocm31727263', false, 'Coote and Tristram, 13th ed.'),
  ('CTRG96-B971', 'ocm31727263', false, 'Coote and Tristram, 14th ed.'),
  ('ocm21588207', 'ocm22946582', false, 'Coote''s Admiralty practice, 2nd ed.'),
  ('ocm18237686', 'ocm20372185', false, 'Coote on mortgages, later editions'),
  ('ocm28759712', 'ocm28759718', false, 'Tidd''s Practice, early King''s Bench editions'),
  ('ocm28759715', 'ocm28759718', false, 'Tidd''s Practice, 5th-7th eds.'),
  ('ocm30820569', 'ocm28759718', true, 'Supplement to Tidd''s Practice'),
  ('ocm28759722', 'ocm28759718', true, 'Tidd''s second supplement on changes by late statutes'),
  ('ocm28759723', 'ocm28759718', true, 'Tidd''s New practice, continuation of the Practice'),
  ('ocm31395428', 'ocm28759718', true, 'Appendix of NY notes to Tidd''s Practice'),
  ('ocm30820555', 'ocm28759717', false, 'Tidd''s Appendix of forms, later Forms'),
  ('ocm28759724', 'ocm28759717', false, 'Tidd''s Forms, 8th ed.'),
  ('ocm31813978', 'ocm28759717', true, 'Forms taken from Tidd''s Appendix, adapted to NY'),
  ('ocm13682533', 'ocm13682458', false, 'Bell''s Commentaries, early edition'),
  ('ocm12822074', 'ocm13682458', false, 'Bell''s Commentaries, 4th and 7th eds.'),
  ('ocm13682485', 'ocm13831895', true, 'Bell''s case illustrations of his Principles'),
  ('CTRG97-B431', 'ocm13831895', true, 'Synopsis of Bell''s Principles'),
  ('ocm23706497', 'ocm26726517', false, 'Campbell''s Lives of the Chancellors, American ed.'),
  ('ocm20345592', 'ocm21246599', false, 'Williams on bankruptcy, 1st ed.'),
  ('CTRG97-B699', 'ocm18247383', true, 'Wilshere''s analysis of Williams on real property'),
  ('CTRG97-B2479', 'ocm18247383', true, 'Wilshere''s analysis of Williams, 3rd ed.'),
  ('ocm16706126', 'ocm16706117', false, 'Same pamphlet reissued, shew/show spelling'),
  ('CTRG98-B1133', 'CTRG97-B3050', true, 'Supplement to Wigmore on evidence'),
  ('CTRG99-B68', 'CTRG97-B3050', false, 'Wigmore on evidence, 2nd ed.'),
  ('CTRG97-B1826', 'CTRG97-B1827', true, 'Supplement to Honnold''s workmen''s compensation treatise'),
  ('ocm28860156', 'CTRG95-B2559', false, 'Willis''s Workmen''s Compensation Acts, 5th-6th eds.'),
  ('CTRG95-B2695', 'CTRG95-B2559', false, 'Willis''s Workmen''s Compensation Acts, 7th-8th eds.'),
  ('ocm21980530', 'ocm31371953', false, 'Morrison''s Mining rights, 8th-11th eds.'),
  ('CTRG96-B565', 'ocm31371953', false, 'Morrison''s Mining rights, 12th-15th eds.'),
  ('ocm13510241', 'ocm13510274', false, 'Browne on the Statute of frauds, 1st ed.'),
  ('ocm31808402', 'ocm20624341', false, 'Bishop''s criminal law, 8th ed. as New commentaries'),
  ('ocm19281808', 'ocm20624341', false, 'Suppressed preface of Bishop''s criminal law 2nd ed.'),
  ('ocm23228935', 'ocm23228767', false, 'Bishop''s criminal procedure, later as New criminal procedure'),
  ('CTRG97-B1550', 'CTRG97-B1682', false, 'Simonson''s Companies Acts commentaries, updated for 1907'),
  ('ocm26117304', 'ocm26117290', false, 'Ruegg on Employers'' Liability Act, early ed.'),
  ('CTRG97-B1456', 'ocm26117290', false, 'Ruegg on Employers'' Liability, 5th-6th eds.'),
  ('CTRG95-B2686', 'ocm26117290', false, 'Ruegg on Employers'' Liability, 8th ed.'),
  ('ocm21477951', 'CTRG95-B4662', false, 'Simonson on debentures, 2nd ed.'),
  ('CTRG95-B4672', 'CTRG95-B4662', false, 'Simonson on debentures, 4th ed.'),
  ('ocm32869899', 'ocm32904539', false, 'Cherry''s Irish land acts, 2nd ed.'),
  ('CTRG97-B634', 'ocm32904539', false, 'Cherry''s Irish land acts, 3rd ed.'),
  ('ocm13373965', 'ocm13357552', false, 'Memoir of Josiah Quincy Jun., 2nd ed.'),
  ('ocm13374283', 'ocm13357552', false, 'Memoir of Josiah Quincy Jun., 3rd ed.'),
  ('ocm29536933', 'ocm23850937', false, 'Other translation of Dumont''s Bentham legislation treatise'),
  ('ocm29451195', 'ocm23850937', true, 'Analysis of Bentham''s Theory of legislation'),
  ('ocm17669021', 'ocm17627954', false, 'Comyns'' exercises on abstracts, 3rd ed.'),
  ('ocm17627990', 'ocm17627954', false, 'Comyns'' exercises on abstracts, 4th ed.'),
  ('ocm17669042', 'ocm17627954', false, 'Comyns'' exercises on abstracts, 5th ed.'),
  ('ocm23234360', 'ocm31991978', false, 'Baylies'' New trials and appeals, 1st ed.'),
  ('ocm23227256', 'ocm31538840', false, 'Baylies'' Trial practice, 2nd ed.'),
  ('ocm26155819', 'ocm26155783', false, 'Wade''s Black book, 1831 ed.'),
  ('ocm26155876', 'ocm26155783', false, 'Wade''s Black book, new enlarged ed.'),
  ('ocm26156268', 'ocm26155783', true, 'Appendix to Wade''s Black book'),
  ('CTRG95-B3307', 'ocm22339492', false, 'McClain''s carriers casebook, 3rd ed. enlarged'),
  ('CTRG97-B1005', 'ocm14910767', true, 'Analysis of Austin''s Lectures on jurisprudence'),
  ('CTRG97-B1216', 'ocm23826039', false, 'Austen-Cartmell on Finance Acts, 3rd ed.'),
  ('CTRG96-B16', 'ocm23826039', false, 'Austen-Cartmell on Finance Acts, 4th ed.'),
  ('CTRG96-B225', 'ocm17343734', false, 'Huffcut on agency, 2nd ed.'),
  ('CTRG98-B695', 'CTRG96-B250', false, 'Frost on incorporation of corporations, 2nd ed.'),
  ('CTRG99-B1352', 'CTRG97-B755', false, 'Willis''s cases on bailments, 2nd ed.'),
  ('CTRG96-B951', 'CTRG96-B978', false, 'Cockle''s cases on evidence, 1st ed.'),
  ('CTRG96-B948', 'CTRG96-B978', false, 'Cockle''s cases on evidence, 2nd ed.'),
  ('CTRG97-B1547', 'ocm13590728', true, 'Synopsis of Erskine''s Principles'),
  ('CTRG97-B2719', 'ocm23087509', false, 'Green''s Michigan practice, 3rd ed.'),
  ('ocm29043750', 'ocm27270036', false, 'Cababé on interpleader, 2nd-3rd eds.'),
  ('CTRG98-B1301', 'CTRG98-B1313', false, 'Manual for courts-martial, 1908 rev. ed.'),
  ('CTRG98-B1315', 'CTRG98-B1313', false, 'Manual for courts-martial, 1920 ed.'),
  ('CTRG99-B938', 'CTRG99-B937', true, 'Supplement to Aron''s Digest of NY real property'),
  ('ocm12674677', 'ocm12674896', false, 'Wade''s Cabinet lawyer, early editions'),
  ('ocm14957604', 'ocm14957593', true, 'Answers to Shearwood''s digest of examination questions'),
  ('ocm31820159', 'ocm15794884', false, 'Ballantine on limitations, American ed.'),
  ('ocm22357941', 'ocm15794884', false, 'Ballantine on limitations, 1829 ed.'),
  ('ocm32333319', 'ocm18230130', false, 'Fearne on contingent remainders, devises spelling'),
  ('ocm16920585', 'ocm18230130', true, 'Coote''s analysis and index of Fearne''s Essay'),
  ('ocm17935121', 'ocm17945502', false, 'Angell on watercourses, 1st ed.'),
  ('ocm17935127', 'ocm17945502', false, 'Angell on watercourses, 2nd ed.'),
  ('ocm18451862', 'ocm18451788', false, 'Byles on bills, later editions'),
  ('ocm19500373', 'ocm18451788', false, 'Byles on bills, cheques editions'),
  ('ocm24484507', 'ocm24484541', false, 'Tiffany''s Michigan justices of the peace, 1st ed.'),
  ('ocm23234331', 'ocm23367645', false, 'Burrill''s NY practice, 2nd ed.'),
  ('ocm25148525', 'ocm23367645', false, 'Appendix of forms volume of Burrill''s practice'),
  ('ocm24865744', 'ocm24865707', false, 'McClellan''s surrogates'' practice, 2nd ed.'),
  ('ocm24866153', 'ocm24865707', false, 'McClellan''s surrogates'' practice, 3rd ed.'),
  ('ocm32336553', 'ocm32349940', false, 'Vattel''s Law of nations, 1805 ed.'),
  ('ocm32351310', 'ocm32349940', false, 'Vattel''s Law of nations, 1852 ed.'),
  ('CTRG95-B1487', 'ocm15119393', false, 'Harriman on contracts, 2nd ed.'),
  ('CTRG99-B873', 'CTRG95-B1684', false, 'Street''s Texas personal injuries, revised and retitled'),
  ('CTRG95-B2044', 'CTRG95-B2046', false, 'Thornton on federal employers'' liability, 2nd ed.'),
  ('CTRG95-B2530', 'ocm20446848', false, 'Bellot on money-lenders, 2nd ed.'),
  ('CTRG98-B2894', 'CTRG95-B2906', false, 'Pease''s students'' summary of contract, 3rd ed.'),
  ('CTRG95-B2974', 'ocm32077744', false, 'Williamson on licensing, 3rd ed.'),
  ('CTRG98-B660', 'CTRG95-B3241', false, 'Bays on corporations, 2nd ed.'),
  ('CTRG95-B3612', 'ocm16929500', false, 'Edwards'' compendium of property in land, 4th-5th eds.'),
  ('CTRG95-B3734', 'ocm16849281', false, 'Banning on limitation of actions, 3rd ed.'),
  ('CTRG95-B4132', 'CTRG95-B4123', false, 'Hall on Massachusetts business corporations, 3rd ed.'),
  ('CTRG95-B4464', 'CTRG95-B4465', false, 'Fraser on parliamentary elections, 2nd ed.'),
  ('CTRG96-B1685', 'ocm25782551', false, 'Emden on building contracts, 4th ed.'),
  ('CTRG96-B183', 'ocm22601421', false, 'Bagehot''s English constitution with added essays'),
  ('CTRG99-B33', 'CTRG96-B2046', false, 'Rose on federal courts, 2nd ed.'),
  ('CTRG98-B3209', 'CTRG96-B3684', false, 'Lile on equity pleading, 2nd ed.'),
  ('CTRG96-B395', 'ocm17372831', false, 'Freeman on void judicial sales, 4th ed.'),
  ('CTRG96-B740', 'CTRG97-B1538', false, 'Montgomery on excess profits duty, 2nd ed.'),
  ('ocm25906619', 'CTRG96-B991', false, 'Vincent''s Police code, 8th ed.'),
  ('CTRG98-B3139', 'CTRG97-B1164', true, 'Supplement to Heaton''s Surrogates'' Courts'),
  ('ocm32583838', 'CTRG97-B1339', false, 'Heywood and Massey''s lunacy practice, 1st ed.'),
  ('CTRG98-B2947', 'CTRG97-B2483', false, 'Safford on Rent Restrictions Acts, 3rd ed.'),
  ('CTRG97-B2812', 'CTRG97-B619', false, 'O''Sullivan''s key to Labourers Acts, 4th ed.'),
  ('CTRG97-B608', 'CTRG97-B580', true, 'Supplement of notes on Summary Jurisdiction Act 1908'),
  ('CTRG97-B750', 'ocm23964297', false, 'Coldridge on gambling, 2nd ed.'),
  ('CTRG98-B1093', 'CTRG95-B3941', true, 'Case facts for Smith and Moore''s bills casebook'),
  ('CTRG98-B2405', 'CTRG98-B1139', false, 'Beverley''s digest of compensation cases, 2nd ed.'),
  ('CTRG98-B1497', 'ocm30386827', false, 'Soward on estate duty, 5th ed.'),
  ('CTRG98-B2935', 'ocm21398853', false, 'Chalmers on bankruptcy acts, 8th ed.'),
  ('ocm12099579', 'ocm12099584', false, 'Hill''s Liberty and law, 2nd ed.'),
  ('ocm13640943', 'ocm13593235', true, 'Principles extracted from Stair''s Institutions'),
  ('ocm14975320', 'ocm32403565', false, 'Raithby''s Study and practice of the law, 2nd ed.'),
  ('ocm18925495', 'ocm17023750', false, 'Gazzam''s bankrupt law, 4th ed.'),
  ('ocm21131860', 'ocm17334398', false, 'Freeman on executions, 1882 ed.'),
  ('ocm31907318', 'ocm17395977', false, 'Sampson''s Criminal jurisprudence, 2nd ed.'),
  ('ocm17535428', 'ocm17535406', false, 'Atkinson on conveyancing, 2nd ed.'),
  ('ocm17657554', 'ocm18447142', false, 'M''Laren on wills and succession, 3rd ed.'),
  ('ocm17799605', 'ocm18755697', false, 'Curtis on patents, 3rd-4th eds.'),
  ('ocm20578506', 'ocm17865235', false, 'Angell on carriers, 3rd ed.'),
  ('ocm32070853', 'ocm18240133', false, 'Watkins on descents, 4th ed.'),
  ('ocm19490265', 'ocm18403312', false, 'Fry on specific performance, American eds.'),
  ('ocm31819962', 'ocm18670595', false, 'Volume I of the same Suffolk County history'),
  ('ocm18979926', 'ocm31180571', false, 'Jefferson''s Notes on Virginia, new ed.'),
  ('ocm32057685', 'ocm19528827', false, 'Oliphant on horses, 1st ed.'),
  ('ocm22885375', 'ocm20289047', true, 'Supplement to Robson on bankruptcy'),
  ('ocm20446898', 'ocm20387421', false, 'Davis on building societies, 4th ed.'),
  ('ocm20495227', 'ocm20495238', false, 'Dos Passos on inheritance taxes, 2nd ed.'),
  ('ocm20558707', 'ocm32139410', false, 'Sedgwick on statutory construction, 2nd ed.'),
  ('ocm21327186', 'ocm21327080', false, 'Montagu and Ayrton on bankruptcy, 1845 issue'),
  ('ocm21575113', 'ocm22769275', false, 'Hindmarch on patents, 1847 ed.'),
  ('ocm21989556', 'ocm32026878', false, 'Reed''s Conduct of lawsuits, 1st ed.'),
  ('ocm22357965', 'ocm27176750', false, 'Best on right to begin, American ed.'),
  ('ocm22885208', 'ocm22885197', false, 'Roberts on employers'' liability, 2nd ed.'),
  ('ocm32333802', 'ocm23087488', false, 'Graham on new trials, 2nd ed.'),
  ('ocm23475976', 'ocm31814212', false, 'Blake''s NY chancery practice, 2nd ed.'),
  ('ocm23714587', 'ocm23714980', false, 'Ellis on proceedings in Parliament, 2nd ed.'),
  ('ocm25044127', 'ocm25044133', false, 'Redfield on Surrogates'' courts, 1st ed.'),
  ('ocm27270293', 'ocm32407734', false, 'Beames on costs in equity, 1st ed.'),
  ('ocm29015553', 'ocm29015602', false, 'Best on evidence, early editions'),
  ('ocm29835811', 'ocm30326220', false, 'Mathews on presumptive evidence, American ed.'),
  ('ocm29974566', 'ocm29974498', false, 'Farries on bills of costs, 3rd ed.'),
  ('ocm32208623', 'ocm30427355', false, 'Hawles'' Englishman''s right, 1810 ed.'),
  ('ocm30766929', 'ocm31757127', false, 'Pemberton''s judgments and orders, 4th ed.'),
  ('ocm32408085', 'ocm31722680', false, 'Bennet on masters'' office, 1842 ed.'),
  ('ocm31890902', 'ocm33060175', false, 'Brunton and Haig''s senators, 1849 issue'),
  ('ocm32153613', 'ocm32153639', false, 'Wills on circumstantial evidence, early eds.'),
  ('ocm32905200', 'ocm31801347', false, 'Jervis on coroners, 5th ed. by Melsheimer'),
  ('ocm32905228', 'ocm31801347', false, 'Jervis on coroners, 6th ed. by Melsheimer'),
  ('ocm33065847', 'ocm32944673', false, 'De Moleyns'' landowner''s guide, 1st ed.'),
  ('ocm31674787', 'ocm31775448', true, 'Blyth''s analysis of Snell''s Principles of equity'),
  ('ocm31674812', 'ocm31775448', true, 'Blyth''s analysis of Snell''s Principles of equity'),
  ('ocm30599935', 'ocm31775448', true, 'Blyth''s analysis of Snell''s Principles of equity'),
  ('CTRG96-B1590', 'ocm31775448', true, 'Blyth''s analysis of Snell''s Principles of equity'),
  ('CTRG98-B2262', 'ocm31775448', true, 'Blyth''s analysis of Snell''s Principles of equity'),
  ('CTRG98-B2263', 'ocm31775448', true, 'Blyth''s analysis of Snell''s Principles of equity'),
  ('CTRG96-B1591', 'ocm31775448', true, 'Blyth''s analysis of Snell''s Principles of equity'),
  ('ocm14560266', 'ocm31775448', true, 'Gibson''s Aids: student guide to Snell''s Principles of equity'),
  ('CTRG96-B1593', 'ocm31775448', true, 'Gibson & Weldon''s Aids: guide to Snell''s Principles of equity'),
  ('ocm20372384', 'CTRG95-B2755', false, 'Gibson & Weldon''s Student''s bankruptcy, earlier edition'),
  ('ocm20372367', 'CTRG95-B2755', false, 'Gibson & Weldon''s Student''s bankruptcy, 2nd edition'),
  ('ocm14560300', 'CTRG98-B2220', false, 'Student''s criminal and magisterial law, earlier editions'),
  ('CTRG98-B2813', 'ocm32772339', false, 'Gibson''s Conveyancing is the Student''s conveyancing, 12th ed.'),
  ('ocm30699087', 'ocm29628720', false, 'Daniell''s Chancery practice, English editions'),
  ('CTRG97-B650', 'ocm29628720', false, '7th ed. of Daniell''s Chancery practice'),
  ('CTRG97-B691', 'ocm29628720', false, '8th ed. of Daniell''s Chancery practice'),
  ('ocm31961467', 'ocm29628720', true, 'Headlam''s supplement to Daniell''s Chancery practice'),
  ('ocm30107663', 'ocm30107641', false, 'Grant''s Chancery practice, 2nd ed. under new orders'),
  ('ocm30766924', 'ocm30107641', true, 'second supplement to Grant''s Chancery practice'),
  ('ocm30766923', 'ocm30107641', true, 'supplement to Grant''s Chancery practice'),
  ('ocm21290099', 'ocm19500450', false, 'Grant on bankers and banking, earlier editions'),
  ('ocm32058497', 'ocm23019761', false, 'Sugden''s Letters to a man of property, 3rd ed.'),
  ('ocm32062466', 'ocm23019761', false, 'Letters to a man of property, 4th ed. retitled'),
  ('ocm32484621', 'ocm23019761', false, 'Sugden''s Letters to a man of property, later edition'),
  ('ocm32058515', 'ocm17422852', false, 'Letter to James Humphreys, 3rd ed.'),
  ('ocm25847178', 'ocm17418624', false, 'Handy book on property law, other editions'),
  ('ocm31931708', 'ocm17418627', false, '2nd ed. of Sugden''s Essay on the new statutes, retitled'),
  ('ocm15611323', 'ocm32164528', false, 'Bigelow''s Elements of the law of torts'),
  ('CTRG98-B725', 'ocm32164528', false, 'Law of torts: 7th and 8th eds. of Elements'),
  ('ocm20625186', 'ocm15601027', false, '2nd ed. of Elements of bills, notes, and cheques'),
  ('ocm15601021', 'ocm15601027', true, 'casebook to accompany Bigelow''s bills, notes, and cheques'),
  ('ocm18430360', 'ocm19522895', false, 'Anson''s Principles of the English law of contract'),
  ('ocm21617433', 'ocm19522895', false, 'American edition of Anson on contract'),
  ('ocm15235632', 'ocm19522895', true, 'notes supplementary to Anson on contracts'),
  ('ocm20409878', 'ocm19522895', true, 'questions and answers to Anson on contracts'),
  ('ocm22272281', 'ocm19522895', true, 'casebook arranged on Anson''s analysis of contract'),
  ('CTRG95-B2138', 'ocm19522895', true, 'later editions of Huffcut''s Anson casebook'),
  ('ocm22620250', 'ocm16671967', true, 'supplement to Elton on copyholds, 2nd ed.'),
  ('ocm17411382', 'ocm17406455', false, 'Scriven on copyholds, 1st ed.'),
  ('ocm31907334', 'ocm17406455', false, 'Scriven on copyholds, 2nd ed.'),
  ('ocm17411387', 'ocm17406455', true, 'supplement to Scriven on copyholds, 3rd ed.'),
  ('ocm31912858', 'ocm17406455', false, 'Scriven on copyholds, 6th ed.'),
  ('ocm17406463', 'ocm17406455', false, 'Scriven on copyholds, 7th ed.'),
  ('ocm24293927', 'ocm24293869', false, 'Conkling on United States courts, 2nd ed.'),
  ('ocm24293907', 'ocm24293869', false, 'Conkling on United States courts, 5th ed.'),
  ('ocm31995673', 'ocm31614421', false, 'Conkling''s admiralty jurisdiction, same 1848 work'),
  ('ocm31614364', 'ocm31614421', false, 'Conkling''s admiralty jurisdiction, 2nd ed.'),
  ('ocm32835173', 'ocm32834892', false, 'Buckley on the Companies acts, 3rd ed.'),
  ('ocm32835235', 'ocm32834892', false, 'Buckley on the Companies acts, 4th ed.'),
  ('ocm32835272', 'ocm32834892', false, 'Buckley on the Companies acts, 7th ed.'),
  ('CTRG97-B622', 'ocm32834892', false, 'Buckley on the Companies acts, 8th ed.'),
  ('CTRG97-B612', 'ocm32834892', false, 'Buckley on the Companies acts, 9th ed.'),
  ('ocm31991468', 'ocm32165299', false, 'American edition of Smith''s Compendium of mercantile law'),
  ('ocm32872946', 'ocm16634265', false, 'Josiah Smith''s Compendium of real and personal property'),
  ('ocm16806608', 'ocm16634265', false, 'Josiah Smith''s Compendium of real and personal property'),
  ('CTRG95-B1553', 'CTRG98-B1052', true, 'selection from Williston''s Cases on the law of contracts'),
  ('CTRG96-B962', 'CTRG97-B799', false, 'American revision of Kenny''s Outlines of criminal law'),
  ('ocm30347943', 'ocm30347921', false, 'Paley on summary convictions, 4th-5th eds.'),
  ('ocm30347952', 'ocm30347921', false, 'Paley on summary convictions, 6th ed.'),
  ('ocm30347955', 'ocm30347921', false, 'Paley on summary convictions, 7th ed.'),
  ('CTRG97-B1246', 'ocm30347921', false, 'Paley on summary convictions, 8th ed.'),
  ('CTRG95-B2023', 'ocm31535535', false, 'Richards on insurance, 3rd ed.'),
  ('CTRG95-B3120', 'CTRG98-B1196', false, 'Richards'' cases on insurance, 2nd ed.'),
  ('ocm18236546', 'ocm32059091', true, 'supplement to Underhill on trusts and trustees'),
  ('ocm17592980', 'ocm32059091', false, 'Underhill on trusts and trustees, 4th ed.'),
  ('CTRG95-B2477', 'ocm32059091', false, 'Underhill on trusts and trustees, 7th ed.'),
  ('CTRG95-B2716', 'ocm22787726', false, 'Arnould on marine insurance, 7th ed.'),
  ('ocm22787769', 'ocm22787726', false, 'Arnould on marine insurance, Maclachlan''s editions'),
  ('ocm32503992', 'ocm32438002', false, 'Langbein on New York City district courts'),
  ('ocm29763868', 'ocm32438002', false, 'district courts became Municipal Court; 4th ed.'),
  ('CTRG96-B1326', 'ocm32438002', false, 'Langbein on Municipal Court, 5th ed.'),
  ('CTRG96-B2390', 'CTRG96-B2393', false, 'Powell on taxation of New York corporations, 1st ed.'),
  ('CTRG96-B2281', 'CTRG96-B2393', false, 'Powell on taxation of New York corporations, 2nd ed.'),
  ('ocm31504686', 'ocm31504684', true, 'supplement to Matthews'' Digest of indictable offences'),
  ('ocm15728269', 'ocm15728264', false, 'Life of Sir Samuel Romilly, 3rd ed.'),
  ('ocm32415238', 'ocm31628198', false, 'Sedgwick on the measure of damages, early eds.'),
  ('ocm32048420', 'ocm32140249', false, 'Starkie on slander and libel, 1st ed.'),
  ('ocm17783531', 'ocm32140249', false, 'Folkard''s Starkie on slander, American ed.'),
  ('ocm32145276', 'ocm32140249', false, 'Folkard''s Starkie; edition numbering continues Starkie''s'),
  ('ocm25044058', 'ocm25044041', false, 'Puterbaugh''s Illinois common law pleading and practice'),
  ('ocm31896799', 'ocm31896783', false, 'Tait on constables in Scotland, 4th ed.'),
  ('ocm21989473', 'ocm15578137', false, 'Putzel''s Commercial precedents, other edition'),
  ('CTRG95-B1460', 'ocm15578137', false, 'Putzel''s Commercial precedents, 1913 edition'),
  ('CTRG95-B2628', 'ocm20449074', false, 'Copinger on copyright, 4th ed.'),
  ('CTRG95-B2899', 'ocm20449074', false, 'Copinger on copyright, 5th ed.'),
  ('ocm13485590', 'ocm17239396', false, 'Bigelow on estoppel, 5th ed.'),
  ('CTRG95-B2852', 'ocm17239396', false, 'Bigelow on estoppel, 6th ed.'),
  ('CTRG95-B3453', 'ocm14925910', false, 'Markby''s Elements of law, 6th ed.'),
  ('Ocm15000718', 'ocm14925910', true, 'supplement to Markby''s Elements of law'),
  ('CTRG96-B1622', 'ocm27902927', false, 'Oswald on contempt of court, 3rd ed.'),
  ('CTRG97-B671', 'ocm27902927', false, 'Oswald on contempt of court, 3rd American ed.'),
  ('ocm32996006', 'CTRG96-B652', false, 'Muirhead on burgh police government, 1st ed.'),
  ('CTRG97-B1252', 'CTRG96-B652', true, 'supplement to Muirhead on burgh government'),
  ('CTRG97-B1085', 'ocm25295659', false, 'Mitchell''s Motions and rules, 2nd ed. enlarged'),
  ('CTRG98-B2632', 'CTRG98-B2425', false, 'Webster-Brown on Finance Acts, 2nd ed.'),
  ('CTRG97-B2849', 'CTRG98-B2425', false, 'Webster-Brown on Finance Acts, 4th ed.'),
  ('CTRG98-B2748', 'ocm32913716', false, 'Redgrave''s Factory acts, 11th ed.'),
  ('CTRG98-B1590', 'ocm32913716', false, 'Redgrave''s Factory acts, 12th ed.'),
  ('ocm11957643', 'ocm12181713', false, 'Butts'' Business man''s law library, earlier edition'),
  ('ocm32520105', 'ocm32519922', true, 'supplement to Carpmael''s Patent laws of the world'),
  ('ocm31233776', 'ocm21326915', false, 'Marshall on insurance, American editions'),
  ('ocm31941840', 'ocm21326915', false, 'Marshall on insurance, 5th ed. retitled'),
  ('ocm32407444', 'ocm32444296', false, 'continuation volume of Barbour''s analytical equity digest'),
  ('ocm25124952', 'ocm31794283', true, 'tabular analysis adapted to Greenleaf on evidence'),
  ('ocm32486862', 'ocm31683705', false, 'Sheppard''s Touchstone, 8th ed.'),
  ('ocm32663347', 'ocm31683705', false, 'Sheppard''s Touchstone, 1840 edition'),
  ('CTRG95-B2205', 'CTRG95-B1528', false, 'Frost on guaranty insurance, 2nd ed.'),
  ('ocm20289043', 'CTRG95-B2383', false, 'Robbins on devolution of real estate, 2nd ed.'),
  ('CTRG95-B2611', 'ocm21439891', false, 'Pixley on auditors, 11th ed.'),
  ('CTRG97-B1432', 'CTRG95-B2976', false, 'Duckworth on general average, 1st ed.'),
  ('CTRG99-B906', 'CTRG95-B3243', false, 'Bays on bankruptcy, retitled'),
  ('ocm13277755', 'CTRG95-B3373', false, 'same Rhode Island legislative history, second record'),
  ('CTRG95-B3763', 'ocm14186910', false, 'Devlin on deeds, 3rd ed. retitled'),
  ('CTRG95-B4012', 'CTRG97-B859', false, 'Daniels'' Law of distress, 5th ed.'),
  ('CTRG99-B1578', 'CTRG95-B4125', false, 'Hamilton on Michigan corporations, student ed. of 3rd ed.'),
  ('CTRG95-B4246', 'CTRG95-B4245', false, 'Cohen''s Trade union law, 1st ed.'),
  ('CTRG95-B4591', 'ocm19517466', false, 'Hart on auctioneers, 2nd ed.'),
  ('CTRG96-B1460', 'ocm23747156', false, 'Norman''s Digest of the death duties, 3rd ed.'),
  ('CTRG97-B1195', 'CTRG96-B1700', false, 'Allan on Housing of the Working Classes Acts, 2nd ed.'),
  ('CTRG96-B1830', 'ocm23094499', false, 'Hayne on new trial and appeal, revised ed.'),
  ('CTRG96-B344', 'CTRG96-B295', true, 'supplement to Savidge on Pennsylvania corporations'),
  ('CTRG99-B1068', 'CTRG96-B3117', false, 'Gleason on inheritance taxation, 4th ed.'),
  ('CTRG96-B3703', 'ocm23113763', false, 'Rumsey''s New York practice, 2nd ed.'),
  ('CTRG96-B549', 'ocm31871216', false, 'Thring''s Practical legislation, later ed.'),
  ('CTRG98-B1494', 'CTRG96-B742', false, 'Langdon on excess profits duty, 4th ed.'),
  ('CTRG97-B1186', 'ocm20590634', false, 'Kinney on irrigation, 2nd ed.'),
  ('CTRG97-B1434', 'ocm21439766', false, 'Palmer''s Private companies, 30th ed.'),
  ('CTRG97-B798', 'CTRG97-B610', true, 'supplement to Roberts'' Federal liabilities of carriers'),
  ('CTRG97-B869', 'CTRG98-B1583', false, 'Rawlinson''s Municipal corporations acts, 10th ed.'),
  ('CTRG98-B1055', 'ocm13490898', false, 'Schouler on domestic relations, 6th ed. retitled'),
  ('CTRG98-B1172', 'ocm32683637', false, 'Winthrop''s Military law, retitled later edition'),
  ('CTRG98-B2114', 'CTRG98-B2623', false, 'Robertson''s Manual of medical jurisprudence, 4th ed.'),
  ('CTRG98-B471', 'ocm23227239', false, 'Baylies'' Rules of pleading, 2nd ed.'),
  ('CTRG99-B377', 'CTRG99-B376', false, 'part 2 of the same publication'),
  ('ocm12283845', 'ocm31477884', false, 'Works of James Wilson, 1896 edition'),
  ('ocm12672684', 'ocm12672615', false, 'Bacon''s Abridgment, 7th ed.'),
  ('ocm13170717', 'ocm13167207', false, 'Ernst on married women in Massachusetts, 2nd ed.'),
  ('ocm14093400', 'ocm14094186', false, 'Thomas on New York mortgages, 1st ed.'),
  ('ocm14207322', 'ocm14207325', false, 'later issue of the society''s manual of child laws'),
  ('ocm14813091', 'ocm15712983', false, 'Reddie''s Inquiries in the science of law, 2nd ed.'),
  ('ocm17108949', 'ocm15126008', true, 'supplement to Hinkley''s Testamentary law of Maryland'),
  ('ocm15276248', 'ocm22610816', false, 'Chipman''s Essay on contracts, 1852 ed.'),
  ('ocm15978314', 'ocm15689492', false, 'Pearce''s Inns of Court, retitled revised ed.'),
  ('ocm15987062', 'ocm16015298', true, 'afterword to Shakespeare vor dem Forum der Jurisprudenz'),
  ('ocm17395466', 'ocm17348398', false, 'Beach on receivers, 2nd ed.'),
  ('ocm17418411', 'ocm17418394', false, 'Powell''s Essay on devises, 3rd ed.'),
  ('ocm17558546', 'ocm17558541', false, 'Atkinson on marketable titles, 2nd ed.'),
  ('ocm32069604', 'ocm17593524', true, 'Woodfall''s epitome of his Landlord and tenant'),
  ('ocm32407973', 'ocm17732676', false, 'Bell on husband and wife property, 1850 ed.'),
  ('ocm17950899', 'ocm20683369', false, 'Bump on patents, trade-marks, copyrights, 2nd ed.'),
  ('ocm25143217', 'ocm18081076', false, 'Hickey''s Constitution, 1st ed.'),
  ('ocm32039708', 'ocm18241232', false, 'Lewin on trusts, early editions'),
  ('ocm18434836', 'ocm33006920', false, 'Beven''s Principles of negligence, 1st ed.'),
  ('ocm19108229', 'ocm18670747', false, 'Ringgold''s Legal Sunday, 2nd ed.'),
  ('ocm19030346', 'ocm19030341', true, 'extract from Carey''s Cursory views'),
  ('ocm19451609', 'ocm32150857', false, 'Wood''s American edition of Collyer on partnership'),
  ('ocm32055028', 'ocm20323291', false, 'Thomson on bills of exchange, new ed.'),
  ('ocm20504167', 'ocm20504157', false, 'Pomeroy on water rights, rev. ed. of riparian rights'),
  ('ocm32598128', 'ocm20913753', true, 'abridged translation of Beaumont and Tocqueville''s report'),
  ('ocm21575142', 'ocm22769332', false, 'Hodges on railways, 1st ed.'),
  ('ocm33814519', 'ocm22000631', false, 'Freedley on Pennsylvania corporation law, 2nd ed.'),
  ('ocm22601776', 'ocm22999764', false, 'Lowndes on general average, 2nd-4th eds.'),
  ('ocm22634378', 'ocm22634400', false, 'Oke''s game laws, 3rd ed.'),
  ('ocm22669616', 'ocm22830633', false, 'Tennant on factories and workshops, 2nd ed.'),
  ('ocm28387088', 'ocm22885364', false, 'appendix volume to Lex parochialis'),
  ('ocm23108559', 'ocm23108646', false, 'Bentham''s Fragment on government, 1891 ed.'),
  ('ocm23529526', 'ocm23713229', false, 'May''s Parliamentary practice, 10th ed.'),
  ('ocm23678622', 'ocm23678659', false, 'Arnold on municipal corporations, 1st ed.'),
  ('ocm23862792', 'ocm23862731', false, 'Kerr''s Inebriety, 3rd ed.'),
  ('ocm24006113', 'ocm24005925', true, 'second supplement to De Gex''s Arrangements'),
  ('ocm25161625', 'ocm24772551', false, 'Jones on Illinois county courts, 1st ed.'),
  ('ocm25687512', 'ocm25687503', false, 'Pritchard''s Reform of ecclesiastical courts, revised'),
  ('ocm26018796', 'ocm26018780', false, 'Carrington''s Supplement to criminal law treatises, 3rd ed.'),
  ('ocm28394732', 'ocm28394723', false, 'Shelford on probate and succession duties, 2nd ed.'),
  ('ocm32323883', 'ocm29015692', false, 'Billing on awards and arbitrations, 1846 issue'),
  ('ocm30106202', 'ocm30106167', true, 'supplement to Barclay''s Notes on meditatione fugae'),
  ('ocm30427393', 'ocm30427397', false, 'Heywood on county courts, 1st ed.'),
  ('ocm31165434', 'ocm30766934', false, 'Hale''s Pleas of the crown, 1st American ed.'),
  ('ocm32041475', 'ocm31372721', false, 'Selwyn''s Abridgment of nisi prius'),
  ('ocm31538653', 'ocm32291709', false, 'Roberts'' Principles of Chancery, 2nd ed.'),
  ('ocm31884538', 'ocm31730369', false, 'appendix volume to Dickinson''s justice of the peace'),
  ('ocm32794425', 'ocm32794435', false, 'Westlake on private international law, 1st ed.'),
  ('ocm32992680', 'ocm32990158', false, 'Kisbey on Irish bankruptcy, 4th ed.');

DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n FROM work_links l
  WHERE NOT EXISTS (SELECT 1 FROM moml.editions e WHERE e.bibliographicid = l.source_bibliographicid)
     OR NOT EXISTS (SELECT 1 FROM moml.editions e WHERE e.bibliographicid = l.target_bibliographicid);
  IF n <> 0 THEN
    RAISE EXCEPTION '% work links name an edition that does not exist', n;
  END IF;
  IF EXISTS (SELECT 1 FROM work_links GROUP BY source_bibliographicid HAVING count(*) > 1) THEN
    RAISE EXCEPTION 'an edition is linked to more than one work';
  END IF;
  IF EXISTS (SELECT 1 FROM work_links WHERE source_bibliographicid = target_bibliographicid) THEN
    RAISE EXCEPTION 'an edition is linked to itself';
  END IF;
END $$;

-- 3. Works: the rule components joined by the reviewed links.
CREATE TEMPORARY TABLE work_edition ON COMMIT DROP AS
SELECT k.bibliographicid, rc.component AS rule_component, rc.component AS work_component
FROM work_keys k
JOIN work_rule_component rc USING (group_key);

DO $$
BEGIN
  LOOP
    UPDATE work_edition w
    SET work_component = m.work_component
    FROM (SELECT e.a, min(we.work_component) AS work_component
          FROM (SELECT s.rule_component AS a, t.rule_component AS b
                  FROM work_links l
                  JOIN work_edition s ON s.bibliographicid = l.source_bibliographicid
                  JOIN work_edition t ON t.bibliographicid = l.target_bibliographicid
                UNION
                SELECT t.rule_component, s.rule_component
                  FROM work_links l
                  JOIN work_edition s ON s.bibliographicid = l.source_bibliographicid
                  JOIN work_edition t ON t.bibliographicid = l.target_bibliographicid) e
          JOIN work_edition we ON we.rule_component = e.b
          GROUP BY e.a) m
    WHERE w.rule_component = m.a AND m.work_component < w.work_component;
    EXIT WHEN NOT FOUND;
  END LOOP;
END $$;

-- An edition is derivative when its rule group was linked in as a derivative.
ALTER TABLE work_edition ADD COLUMN derivative boolean NOT NULL DEFAULT false;
UPDATE work_edition w
SET derivative = true
WHERE w.rule_component IN (
  SELECT s.rule_component FROM work_links l
  JOIN work_edition s ON s.bibliographicid = l.source_bibliographicid
  WHERE l.derivative);

-- A rule group can hold both an edition and an abridgment when their titles
-- agree ("Blackstone's Commentaries"), so these editions are marked one by one.
UPDATE work_edition
SET derivative = true
WHERE bibliographicid IN (VALUES
  ('ocm12102232'),   -- Blackstone's Commentaries systematically abridged by Samuel Warren, 1855
  ('ocm12822404'),   -- the same, 1856
  ('ocm12117052'),   -- Blackstone's Commentaries for students, obsolete matter eliminated, 1882
  ('ocm14526584'));  -- Blackstone's Commentaries in questions and answers, 1887

-- A work is named after its principal edition: its earliest edition that is not
-- derivative.
CREATE TEMPORARY TABLE work_principal ON COMMIT DROP AS
SELECT DISTINCT ON (w.work_component)
       w.work_component, k.bibliographicid, k.first_author, k.title
FROM work_edition w
JOIN work_keys k USING (bibliographicid)
ORDER BY w.work_component, w.derivative, k.year NULLS LAST, k.bibliographicid;

CREATE TABLE IF NOT EXISTS moml.works (
    work_id integer GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    author  text,
    title   text NOT NULL
);

COMMENT ON TABLE moml.works IS
  'A treatise across all its editions, including later editors'' revisions, translations, and works derived from it (abridgments, analyses, question books, supplements); every edition belongs to one. See db/queries/work-candidates.sql.';
COMMENT ON COLUMN moml.works.author IS
  'First author of the work''s principal edition (its earliest edition that is not derivative); NULL for an anonymous work.';
COMMENT ON COLUMN moml.works.title IS
  'Main title of the work''s principal edition.';

GRANT SELECT ON moml.works TO law_service;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON moml.works TO law_dev;

CREATE TEMPORARY TABLE work_ids ON COMMIT DROP AS
SELECT p.work_component,
       row_number() OVER (ORDER BY y.first_year NULLS LAST, p.first_author, p.title, p.bibliographicid)::integer AS work_id,
       nullif(p.first_author, '') AS author,
       btrim(split_part(split_part(p.title, ' : ', 1), ' / ', 1)) AS title
FROM work_principal p
JOIN (SELECT w.work_component, min(k.year) AS first_year
      FROM work_edition w JOIN work_keys k USING (bibliographicid)
      GROUP BY w.work_component) y USING (work_component);

INSERT INTO moml.works (work_id, author, title)
SELECT work_id, author, title FROM work_ids;

SELECT setval(pg_get_serial_sequence('moml.works', 'work_id'), (SELECT max(work_id) FROM moml.works));

ALTER TABLE moml.editions
  ADD COLUMN IF NOT EXISTS work_id integer,
  ADD COLUMN IF NOT EXISTS derivative boolean NOT NULL DEFAULT false;

UPDATE moml.editions e
SET work_id = wi.work_id,
    derivative = w.derivative
FROM work_edition w
JOIN work_ids wi USING (work_component)
WHERE e.bibliographicid = w.bibliographicid;

ALTER TABLE moml.editions ALTER COLUMN work_id SET NOT NULL;
ALTER TABLE moml.editions DROP CONSTRAINT IF EXISTS editions_work_id_fkey;
ALTER TABLE moml.editions
  ADD CONSTRAINT editions_work_id_fkey FOREIGN KEY (work_id) REFERENCES moml.works (work_id);
CREATE INDEX IF NOT EXISTS editions_work_id_idx ON moml.editions (work_id);

COMMENT ON COLUMN moml.editions.work_id IS
  'The work this edition belongs to.';
COMMENT ON COLUMN moml.editions.derivative IS
  'True when the edition is an abridgment, analysis, question book, supplement, adaptation or translation of its work rather than an edition of it.';

-- 4. Checks.
DO $$
DECLARE
  n bigint;
BEGIN
  SELECT count(*) INTO n FROM moml.editions WHERE work_id IS NULL;
  IF n <> 0 THEN
    RAISE EXCEPTION '% editions have no work', n;
  END IF;
  SELECT count(*) INTO n FROM moml.works w
  WHERE NOT EXISTS (SELECT 1 FROM moml.editions e WHERE e.work_id = w.work_id);
  IF n <> 0 THEN
    RAISE EXCEPTION '% works have no edition', n;
  END IF;
  -- Known works: Blackstone's Commentaries with its editions and derivatives,
  -- Kent's Commentaries, Greenleaf on evidence.
  SELECT count(*) INTO n FROM moml.editions
  WHERE work_id = (SELECT work_id FROM moml.editions WHERE bibliographicid = 'ocm12139316');
  IF n <> 119 THEN
    RAISE EXCEPTION 'Blackstone''s Commentaries has % editions, expected 119', n;
  END IF;
  SELECT count(*) INTO n FROM moml.editions
  WHERE work_id = (SELECT work_id FROM moml.editions WHERE bibliographicid = 'ocm12088512');
  IF n <> 22 THEN
    RAISE EXCEPTION 'Kent''s Commentaries has % editions, expected 22', n;
  END IF;
  SELECT count(*) INTO n FROM moml.editions
  WHERE work_id = (SELECT work_id FROM moml.editions WHERE bibliographicid = 'ocm31794283');
  IF n <> 18 THEN
    RAISE EXCEPTION 'Greenleaf on evidence has % editions, expected 18', n;
  END IF;
  -- Blackstone's own Analysis of the laws of England is a separate work, and so
  -- are Story's commentaries on different subjects.
  IF (SELECT work_id FROM moml.editions WHERE bibliographicid = 'ocm14526508')
     = (SELECT work_id FROM moml.editions WHERE bibliographicid = 'ocm12139316') THEN
    RAISE EXCEPTION 'Blackstone''s Analysis joined the Commentaries';
  END IF;
  SELECT count(DISTINCT work_id) INTO n FROM moml.editions
  WHERE bibliographicid IN ('ocm32153928', 'ocm31991715', 'ocm17317394', 'ocm17320286', 'ocm15621393');
  IF n <> 5 THEN
    RAISE EXCEPTION 'Story''s five commentaries fell into % works', n;
  END IF;
END $$;

ANALYZE moml.works, moml.editions;

-- migrate:down
SET ROLE = law_admin;

DROP INDEX IF EXISTS moml.editions_work_id_idx;
ALTER TABLE moml.editions DROP CONSTRAINT IF EXISTS editions_work_id_fkey;
ALTER TABLE moml.editions
  DROP COLUMN IF EXISTS work_id,
  DROP COLUMN IF EXISTS derivative;
DROP TABLE IF EXISTS moml.works;
