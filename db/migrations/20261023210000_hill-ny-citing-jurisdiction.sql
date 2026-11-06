-- migrate:up
SET ROLE = law_admin;

-- Hill's New York Reports (issue #401, split out of the pin-cite evaluation
-- in #393).
--
-- Bare 'Hill' is Hill's South Carolina Law Reports (three volumes, 19-21
-- S.C.L.), and #358 gave it the fallback 'Hill (N.Y.)'. The fallback never
-- linked, for two reasons.
--
-- 1. CAP spells Hill's New York Reports (its reporter 116, seven volumes,
--    1,033 cases) 'Hill & Den.': People ex rel. Whiting v. Carrique, 2 Hill
--    93, is '2 Hill & Den. 93'. FreeLaw spells them the same way. 'Hill
--    (N.Y.)' had reporter_cap 'Hill', which matches nothing. Lalor's one-volume
--    supplement (CAP reporter 115) is also '1 Hill & Den.', and 19 of its cites
--    are the same strings as a cite in Hill's volume 1; the linker's
--    shared-cite rule (cap_shared_longest, #359) takes those.
--
-- 2. Volumes 1-3 never reach the fallback, because Hill's S.C. holds them:
--    a New York cite to '2 Hill 93' was probed as South Carolina's. In the
--    CAP ledger, New York opinions made 1,113 exact and 3,479 pin-cite links
--    into S.C.L. under 'Hill' at volumes 1-3. The linker now reads the citing
--    jurisdiction: when an opinion's state is the fallback reporter's and not
--    the spelling's own, the fallback is read first and the spelling's own
--    reporter becomes the fallback. That applies to every fallback pair, so
--    'Porter' in an Alabama opinion is read as Port. (Ala.) first, 'Den.' in
--    a New York one as Denio, and so on. The MOML ledger records no citing
--    state and is unaffected.
--
-- cap_jurisdiction_codes maps CAP's jurisdictions onto the codes
-- legalhist.reporters.jurisdiction uses ('us:' and the postal code), which
-- is how the linker compares them. Jurisdictions with no state of their own
-- (Dakota Territory, the tribal courts, the Navajo Nation, 'uk') have no row
-- and never reorder a reading.
--
-- The data step is a relink of both corpora: ./scripts/pipeline.sh --from
-- truncate-links, then ./scripts/pipeline.sh --corpus cap --from
-- truncate-links.

UPDATE legalhist.reporters
   SET reporter_cap = 'Hill & Den.'
 WHERE reporter_standard = 'Hill (N.Y.)';

CREATE TABLE IF NOT EXISTS legalhist.cap_jurisdiction_codes (
    cap_jurisdiction bigint PRIMARY KEY REFERENCES cap.jurisdictions(id),
    jurisdiction text NOT NULL
);

COMMENT ON TABLE legalhist.cap_jurisdiction_codes IS 'CAP jurisdictions mapped to the codes legalhist.reporters.jurisdiction uses (us: and the postal code), so the linker can compare a citing opinion''s state with a reporter''s (issue #401).';

INSERT INTO legalhist.cap_jurisdiction_codes (cap_jurisdiction, jurisdiction)
SELECT j.id, m.code
FROM (VALUES
    ('ala', 'us:al'), ('alaska', 'us:ak'), ('am-samoa', 'us:as'), ('ariz', 'us:az'),
    ('ark', 'us:ar'), ('cal', 'us:ca'), ('colo', 'us:co'), ('conn', 'us:ct'),
    ('dc', 'us:dc'), ('del', 'us:de'), ('fla', 'us:fl'), ('ga', 'us:ga'),
    ('guam', 'us:gu'), ('haw', 'us:hi'), ('idaho', 'us:id'), ('ill', 'us:il'),
    ('ind', 'us:in'), ('iowa', 'us:ia'), ('kan', 'us:ks'), ('ky', 'us:ky'),
    ('la', 'us:la'), ('mass', 'us:ma'), ('md', 'us:md'), ('me', 'us:me'),
    ('mich', 'us:mi'), ('minn', 'us:mn'), ('miss', 'us:ms'), ('mo', 'us:mo'),
    ('mont', 'us:mt'), ('nc', 'us:nc'), ('nd', 'us:nd'), ('neb', 'us:ne'),
    ('nev', 'us:nv'), ('nh', 'us:nh'), ('nj', 'us:nj'), ('nm', 'us:nm'),
    ('n-mar-i', 'us:mp'), ('ny', 'us:ny'), ('ohio', 'us:oh'), ('okla', 'us:ok'),
    ('or', 'us:or'), ('pa', 'us:pa'), ('pr', 'us:pr'), ('ri', 'us:ri'),
    ('sc', 'us:sc'), ('sd', 'us:sd'), ('tenn', 'us:tn'), ('tex', 'us:tx'),
    ('us', 'us:us'), ('utah', 'us:ut'), ('va', 'us:va'), ('vi', 'us:vi'),
    ('vt', 'us:vt'), ('wash', 'us:wa'), ('wis', 'us:wi'), ('w-va', 'us:wv'),
    ('wyo', 'us:wy')
) AS m(slug, code)
JOIN cap.jurisdictions j ON j.slug = m.slug
ON CONFLICT (cap_jurisdiction) DO NOTHING;

-- migrate:down
SET ROLE = law_admin;

DROP TABLE IF EXISTS legalhist.cap_jurisdiction_codes;

UPDATE legalhist.reporters
   SET reporter_cap = 'Hill'
 WHERE reporter_standard = 'Hill (N.Y.)';
