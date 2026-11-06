-- migrate:up
SET ROLE = law_admin;

-- Volume translations for two reporters whose nominative volumes CAP numbers
-- in a longer series (issues #399, #400, split out of the pin-cite evaluation
-- in #393). Both were found by looking up, for each citation in
-- opinion_citations.citations_unlinked with a directly detected case name,
-- the CAP case whose first page is the cited page, and asking whether its
-- name agrees with the detected one.
--
-- Hun (#399). CAP's N.Y. Sup. Ct. (reporter 110) counts Thompson & Cook as
-- volumes 1-7, which it does not hold, so Hun volume n is N.Y. Sup. Ct.
-- volume n + 7: McCleary v. McCleary, 30 Hun 154, is 37 N.Y. Sup. Ct. 154.
-- 'Hun' had no diffvols rows and probed volume n, where 160 of 13,881 hits
-- agreed; at n + 7, 49,046 of 51,375 did. CAP's volumes run 8-99, Hun 1-92.
--
-- Vroom (#400). Vroom is one continuously numbered series, P. D. Vroom's
-- volumes 1-6 and G. D. W. Vroom's 7-56, and volume n is N.J.L. n + 29
-- throughout: Horner v. Stillwell, 6 Vroom 307, is 35 N.J.L. 307. The
-- 'Vroom' rows mapped 1-50 to 36-85, as if G. D. W. Vroom restarted at 1,
-- which sent every cite six volumes late. At n + 35, 93 of 3,293 hits
-- agreed; at n + 29, 12,587 of 12,947, and n + 29 wins in every stretch of
-- six volumes from 1 to 56. 'Vroom (o.s.)' (1-6 -> 30-35) already agrees
-- with this and is left as it is; no whitelist spelling reaches it, since
-- every Vroom spelling maps to 'Vroom'.
--
-- No stub or stub metadata is keyed to either reporter. The data step is a
-- relink of both corpora: ./scripts/pipeline.sh --from truncate-links, then
-- ./scripts/pipeline.sh --corpus cap --from truncate-links.

INSERT INTO legalhist.reporters_diffvols (reporter_title, vol, cap_vol, cap_reporter, reporter_standard)
SELECT 'Hun''s New York Supreme Court Reports', v, v + 7, 'N.Y. Sup. Ct.', 'Hun'
FROM generate_series(1, 92) AS v
WHERE NOT EXISTS (
    SELECT 1 FROM legalhist.reporters_diffvols d
    WHERE d.reporter_standard = 'Hun' AND d.vol = v
);

DELETE FROM legalhist.reporters_diffvols
 WHERE reporter_standard = 'Vroom'
   AND cap_vol IS DISTINCT FROM vol + 29;

INSERT INTO legalhist.reporters_diffvols (reporter_title, vol, cap_vol, cap_reporter, reporter_standard)
SELECT 'Vroom G.D.W.''s Reports', v, v + 29, 'N.J.L.', 'Vroom'
FROM generate_series(1, 56) AS v
WHERE NOT EXISTS (
    SELECT 1 FROM legalhist.reporters_diffvols d
    WHERE d.reporter_standard = 'Vroom' AND d.vol = v
);

-- migrate:down
SET ROLE = law_admin;

DELETE FROM legalhist.reporters_diffvols WHERE reporter_standard = 'Hun';

DELETE FROM legalhist.reporters_diffvols WHERE reporter_standard = 'Vroom';

INSERT INTO legalhist.reporters_diffvols (reporter_title, vol, cap_vol, cap_reporter, reporter_standard)
SELECT 'Vroom G.D.W.''s Reports', v, v + 35, 'N.J.L.', 'Vroom'
FROM generate_series(1, 50) AS v;
