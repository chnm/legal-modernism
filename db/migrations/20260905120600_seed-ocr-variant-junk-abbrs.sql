-- migrate:up
SET ROLE = law_admin;

-- Seed legalhist.whitelist with the OCR-corrupted spellings whose every
-- reading is a spelling the whitelist already rejects as junk ("Cye." for
-- "Cyc.", "Dee." for "Dec.", "Wol." for "Vol.", a lone "ii" for "n"),
-- so that they stop counting as skipped_not_whitelisted, which reads as
-- work still to do, and are recorded as the noise they are (issue #247,
-- part of #165).
--
-- Candidates are the TOJUNK section of db/whitelist-candidates-ocr-variants.tsv,
-- compiled by TestOCRVariantWhitelistSuggestions from
-- moml_citations.citations_unlinked, spellings not in the whitelist with at least 20 rows, 2026-09-05.
-- Readings are the same as for the reporter proposals: a letter-flanked
-- digit stripped or read as its look-alike letter, or one substitution
-- from the OCR confusion table in go/citations/ocrvariant.go. A spelling
-- is here only when no reading reaches a reporter, so there is nothing to
-- weigh it against; it inherits the whitelist's verdict on what it was
-- read as. Built by scripts/build-whitelist-junk-migration.py.
--
-- 613 spellings, 71,358 rows. 199 of them (11,970 rows) read as a regnal-year
-- statute spelling that 20260905120000_seed-statute-reporters.sql reroutes
-- from junk to a statute row, and are seeded to that row instead, so the
-- two migrations agree; the other 414 (59,388 rows) are seeded as junk.
-- Counts are rows in moml_citations.citations_unlinked.

INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk) VALUES
    ('Gco. I. c.', 'Stat. Geo.', false),  -- 485, read as Geo. I. c., e.g. 0 Gco. I. c. 13
    ('Hen. VIll. e.', 'Stat. Hen.', false),  -- 348, read as Hen. VIll. c., e.g. 14 Hen. VIll. e. 31
    ('Gco', 'Stat. Geo.', false),  -- 326, read as Geo, e.g. 06 Gco 2
    ('Gco. Ill., c.', 'Stat. Geo.', false),  -- 305, read as Geo. Ill., c., e.g. 10 Gco. Ill., c. 10
    ('Geo. ll. c.', 'Stat. Geo.', false),  -- 303, read as Geo. Il. c., e.g. 0 Geo. ll. c. 67
    ('Gco. III, c.', 'Stat. Geo.', false),  -- 237, read as Geo. III, c., e.g. 10 Gco. III, c. 18
    ('Gco. IV., c.', 'Stat. Geo.', false),  -- 234, read as Geo. IV., c., e.g. 0 Gco. IV., c. 10
    ('Ceo. Ill. c.', 'Stat. Geo.', false),  -- 223, read as Geo. Ill. c., e.g. 12 Ceo. Ill. c. 20
    ('Gco. II, c.', 'Stat. Geo.', false),  -- 188, read as Geo. II, c., e.g. 10 Gco. II, c. 28
    ('Ceo. IV. c.', 'Stat. Geo.', false),  -- 180, read as Geo. IV. c., e.g. 06 Ceo. IV. c. 5
    ('Gco. II., c.', 'Stat. Geo.', false),  -- 159, read as Geo. II., c., e.g. 0 Gco. II., c. 36
    ('Gco. IV, c.', 'Stat. Geo.', false),  -- 148, read as Geo. IV, c., e.g. 0 Gco. IV, c. 120
    ('Gco. V. c.', 'Stat. Geo.', false),  -- 144, read as Geo. V. c., e.g. 0 Gco. V. c. 10
    ('Ceo. II. c.', 'Stat. Geo.', false),  -- 139, read as Geo. II. c., e.g. 10 Ceo. II. c. 28
    ('Geo. It. c.', 'Stat. Geo.', false),  -- 137, read as Geo. Il. c., e.g. 0 Geo. It. c. 28
    ('Gco. IV. &', 'Stat. Geo.', false),  -- 135, read as Geo. IV. &, e.g. 11 Gco. IV.    & 1
    ('Jac. I. e.', 'Stat. Jac.', false),  -- 134, read as Jac. I. c., e.g. 1 Jac. I. e. 1
    ('Geo. n. c.', 'Stat. Geo.', false),  -- 133, read as Geo. ii. c., e.g. 11 Geo. n. c. 19
    ('Jae. I. c.', 'Stat. Jac.', false),  -- 133, read as Jac. I. c., e.g. 1 Jae. I. c. 1
    ('Gco. Ill.', 'Stat. Geo.', false),  -- 130, read as Geo. Ill., e.g. 10 Gco. Ill., 1688
    ('Viet. c', 'Stat. Vict.', false),  -- 130, read as Vict. c, e.g. 10 Viet. c 1
    ('W. & M. e.', 'Stat. W. & M.', false),  -- 126, read as W. & M. c., e.g. 1 W. & M. e. 16
    ('Geo. Ill., e.', 'Stat. Geo.', false),  -- 96, read as Geo. Ill., c., e.g. 10 Geo. Ill., e. 51
    ('Gco. iv. c.', 'Stat. Geo.', false),  -- 93, read as Geo. iv. c., e.g. 0 Gco. iv. c. 120
    ('Viet., e.', 'Stat. Vict.', false),  -- 93, read as Vict., e., e.g. 0 Viet., e. 00
    ('Gco. Ill. cap.', 'Stat. Geo.', false),  -- 90, read as Geo. Ill. cap., e.g. 10 Gco. Ill. cap. 51
    ('Geo. lI. c.', 'Stat. Geo.', false),  -- 90, read as Geo. II. c., e.g. 10 Geo. lI. c. 32
    ('Anue, c.', 'Stat. Anne', false),  -- 89, read as Anne, c., e.g. 10 Anue, c. 1
    ('Will. IV., e.', 'Stat. Will.', false),  -- 88, read as Will. IV., c., e.g. 1 Will. IV., e. 20
    ('Geo. I1I. c.', 'Stat. Geo.', false),  -- 87, read as Geo. II. c., e.g. 12 Geo. I1I. c. 19
    ('Gco. Ill, c.', 'Stat. Geo.', false),  -- 84, read as Geo. Ill, c., e.g. 11 Gco. Ill, c. 24
    ('Viet. r.', 'Stat. Vict.', false),  -- 83, read as Vict. r., e.g. 11 Viet. r. 102
    ('Gco. S. c.', 'Stat. Geo.', false),  -- 82, read as Geo. S. c., e.g. 10 Gco. S. c. 48
    ('Will. Ill. e.', 'Stat. Will.', false),  -- 82, read as Will. Ill. c., e.g. 0 Will. Ill. e. 11
    ('Gco. . c.', 'Stat. Geo.', false),  -- 81, read as Geo. . c., e.g. 0 Gco. . c. 30
    ('Geo. IlI, c.', 'Stat. Geo.', false),  -- 81, read as Geo. III, c., e.g. 10 Geo. IlI, c. 50
    ('Edw. VI. e.', 'Stat. Edw.', false),  -- 79, read as Edw. VI. c., e.g. 0 Edw. VI. e. 10
    ('Geo. IlL. c.', 'Stat. Geo.', false),  -- 79, read as Geo. IIL. c., e.g. 11 Geo. IlL. c. 19
    ('Geo. l. c.', 'Stat. Geo.', false),  -- 79, read as Geo. I. c., e.g. 0 Geo. l. c. 31
    ('Geo. IIl, c.', 'Stat. Geo.', false),  -- 77, read as Geo. III, c., e.g. 10 Geo. IIl, c. 50
    ('Geo. IV., e.', 'Stat. Geo.', false),  -- 73, read as Geo. IV., c., e.g. 10 Geo. IV., e. 34
    ('Car. Il. c.', 'Stat. Car.', false),  -- 72, read as Car. II. c., e.g. 12 Car. Il. c. 13
    ('Edw. Ill. e.', 'Stat. Edw.', false),  -- 72, read as Edw. Ill. c., e.g. 0 Edw. Ill. e. 5
    ('Gco. I, c.', 'Stat. Geo.', false),  -- 72, read as Geo. I, c., e.g. 0 Gco. I, c. 7
    ('Geo. V. e.', 'Stat. Geo.', false),  -- 72, read as Geo. V. c., e.g. 10 Geo. V. e. 92
    ('Gco. Ill. .', 'Stat. Geo.', false),  -- 69, read as Geo. Ill. ., e.g. 12 Gco. Ill. . 24
    ('Geo. Il, c.', 'Stat. Geo.', false),  -- 69, read as Geo. II, c., e.g. 11 Geo. Il, c. 19
    ('Vict.. e.', 'Stat. Vict.', false),  -- 69, read as Vict.. c., e.g. 04 Vict.. e. 5
    ('Wm. IV. e.', 'Stat. Will.', false),  -- 69, read as Wm. IV. c., e.g. 1 Wm. IV. e. 36
    ('Aune, c.', 'Stat. Anne', false),  -- 67, read as Anne, c., e.g. 10 Aune, c. 0
    ('Geo. III, e.', 'Stat. Geo.', false),  -- 67, read as Geo. III, c., e.g. 11 Geo. III, e. 62
    ('Geo. IV, e.', 'Stat. Geo.', false),  -- 65, read as Geo. IV, c., e.g. 10 Geo. IV, e. 38
    ('Geo. iI. c.', 'Stat. Geo.', false),  -- 65, read as Geo. il. c., e.g. 11 Geo. iI. c. 19
    ('Rie.', 'Stat. Ric.', false),  -- 65, read as Ric., e.g. 10 Rie. 1
    ('Hcn.', 'Stat. Hen.', false),  -- 64, read as Hen., e.g. 10 Hcn. 7
    ('Vict. eh.', 'Stat. Vict.', false),  -- 64, read as Vict. ch., e.g. 11 Vict. eh. 11
    ('Gco. II.', 'Stat. Geo.', false),  -- 62, read as Geo. II., e.g. 10 Gco. II. 246
    ('Gco. IV. and', 'Stat. Geo.', false),  -- 62, read as Geo. IV. and, e.g. 11 Gco. IV. and 1
    ('Hen. VIIl. c.', 'Stat. Hen.', false),  -- 62, read as Hen. VIll. c., e.g. 14 Hen. VIIl. c. 2
    ('Car. I. e.', 'Stat. Car.', false),  -- 59, read as Car. I. c., e.g. 11 Car. I. e. 2
    ('Gco. III c.', 'Stat. Geo.', false),  -- 57, read as Geo. III c., e.g. 12 Gco. III c. 11
    ('Vict., cli.', 'Stat. Vict.', false),  -- 57, read as Vict., ch., e.g. 0 Vict., cli. 55
    ('Viet.c.', 'Stat. Vict.', false),  -- 56, read as Vict.c., e.g. 11 Viet.c. 17
    ('Aun. c.', 'Stat. Anne', false),  -- 54, read as Ann. c., e.g. 10 Aun. c. 15
    ('Heu.', 'Stat. Hen.', false),  -- 54, read as Hen., e.g. 10 Heu. 6
    ('Viet . .', 'Stat. Vict.', false),  -- 54, read as Vict . ., e.g. 0 Viet . . 48
    ('Geo. II, e.', 'Stat. Geo.', false),  -- 52, read as Geo. II, c., e.g. 11 Geo. II, e. 10
    ('Geo. Il c.', 'Stat. Geo.', false),  -- 52, read as Geo. II c., e.g. 0 Geo. Il c. 48
    ('Viet .', 'Stat. Vict.', false),  -- 51, read as Vict ., e.g. 13 Viet . 103
    ('Ceo', 'Stat. Geo.', false),  -- 50, read as Geo, e.g. 11 Ceo 1
    ('Hen. VI. e.', 'Stat. Hen.', false),  -- 50, read as Hen. VI. c., e.g. 10 Hen. VI. e. 2
    ('Vicl. .', 'Stat. Vict.', false),  -- 50, read as Vict. ., e.g. 10 Vicl. . 33
    ('Vict. e .', 'Stat. Vict.', false),  -- 50, read as Vict. c ., e.g. 11 Vict. e . 17
    ('Viet. ch.', 'Stat. Vict.', false),  -- 50, read as Vict. ch., e.g. 12 Viet. ch. 42
    ('Geo. IIl., c.', 'Stat. Geo.', false),  -- 49, read as Geo. Ill., c., e.g. 10 Geo. IIl., c. 50
    ('Hen. VII. e.', 'Stat. Hen.', false),  -- 49, read as Hen. VII. c., e.g. 10 Hen. VII. e. 21
    ('Geo. I.e.', 'Stat. Geo.', false),  -- 48, read as Geo. I.c., e.g. 11 Geo. I.e. 18
    ('Ceo. I. c.', 'Stat. Geo.', false),  -- 47, read as Geo. I. c., e.g. 0 Ceo. I. c. 5
    ('Vict. n.', 'Stat. Vict.', false),  -- 47, read as Vict. u., e.g. 10 Vict. n. 95
    ('Viet., Ch.', 'Stat. Vict.', false),  -- 47, read as Vict., Ch., e.g. 12 Viet., Ch. 29
    ('Viet., cap.', 'Stat. Vict.', false),  -- 47, read as Vict., cap., e.g. 10 Viet., cap. 31
    ('Viet.. c.', 'Stat. Vict.', false),  -- 47, read as Vict.. c., e.g. 10 Viet.. c. 101
    ('Edw. IIl. c.', 'Stat. Edw.', false),  -- 46, read as Edw. Ill. c., e.g. 10 Edw. IIl. c. 2
    ('Gco. IV.', 'Stat. Geo.', false),  -- 46, read as Geo. IV., e.g. 0 Gco. IV., 2
    ('Vict.e.', 'Stat. Vict.', false),  -- 46, read as Vict.c., e.g. 10 Vict.e. 95
    ('Geo. Il., c.', 'Stat. Geo.', false),  -- 45, read as Geo. II., c., e.g. 0 Geo. Il., c. 20
    ('Viet. (.', 'Stat. Vict.', false),  -- 45, read as Vict. (., e.g. 0 Viet. (. 772
    ('Viet. cc.', 'Stat. Vict.', false),  -- 45, read as Vict. cc., e.g. 10 Viet. cc. 8
    ('VICT. e.', 'Stat. Vict.', false),  -- 44, read as VICT. c., e.g. 10 VICT. e. 17
    ('Vict. ec.', 'Stat. Vict.', false),  -- 44, read as Vict. cc., e.g. 11 Vict. ec. 38
    ('Vill. IV. c.', 'Stat. Will.', false),  -- 44, read as Will. IV. c., e.g. 1 Vill. IV. c. 2
    ('Anu. c.', 'Stat. Anne', false),  -- 43, read as Ann. c., e.g. 0 Anu. c. 7
    ('Vicl.', 'Stat. Vict.', false),  -- 43, read as Vict., e.g. 11 Vicl.,    214
    ('Gco. Ill. ch.', 'Stat. Geo.', false),  -- 42, read as Geo. Ill. ch., e.g. 10 Gco. Ill. ch. 48
    ('Geo. ni. c.', 'Stat. Geo.', false),  -- 42, read as Geo. iii. c., e.g. 10 Geo. ni. c. 51
    ('Edw. Il. c.', 'Stat. Edw.', false),  -- 41, read as Edw. II. c., e.g. 12 Edw. Il. c. 3
    ('Gco. Iv. c.', 'Stat. Geo.', false),  -- 41, read as Geo. Iv. c., e.g. 0 Gco. Iv. c. 129
    ('Gco. ill. c.', 'Stat. Geo.', false),  -- 41, read as Geo. ill. c., e.g. 13 Gco. ill. c. 21
    ('V. & M. c.', 'Stat. W. & M.', false),  -- 40, read as W. & M. c., e.g. 1 V. & M. c. 10
    ('Viet., o.', 'Stat. Vict.', false),  -- 39, read as Vict., o., e.g. 12 Viet., o. 36
    ('Car. II, e.', 'Stat. Car.', false),  -- 38, read as Car. II, c., e.g. 12 Car. II, e. 24
    ('Gco. II. ch.', 'Stat. Geo.', false),  -- 38, read as Geo. II. ch., e.g. 11 Gco. II. ch. 19
    ('Car. II., e.', 'Stat. Car.', false),  -- 37, read as Car. II., c., e.g. 12 Car. II., e. 1
    ('Geo. Il.', 'Stat. Geo.', false),  -- 37, read as Geo. II., e.g. 12 Geo. Il. 1
    ('Geo. lV. c.', 'Stat. Geo.', false),  -- 37, read as Geo. IV. c., e.g. 10 Geo. lV. c. 43
    ('Vicl c.', 'Stat. Vict.', false),  -- 37, read as Vict c., e.g. 11 Vicl c. 6
    ('Vict., Cli.', 'Stat. Vict.', false),  -- 37, read as Vict., Ch., e.g. 10 Vict., Cli. 93
    ('Eliz., e.', 'Stat. Eliz.', false),  -- 36, read as Eliz., c., e.g. 13 Eliz., e. 10
    ('Geo. . e.', 'Stat. Geo.', false),  -- 36, read as Geo. . c., e.g. 10 Geo. . e. 7
    ('Gco. II. .', 'Stat. Geo.', false),  -- 35, read as Geo. II. ., e.g. 0 Gco. II. . 20
    ('Gco. in. c.', 'Stat. Geo.', false),  -- 35, read as Geo. in. c., e.g. 0 Gco. in. c. 25
    ('Geo. II., e.', 'Stat. Geo.', false),  -- 35, read as Geo. II., c., e.g. 10 Geo. II., e. 1
    ('Hen. VIII, e.', 'Stat. Hen.', false),  -- 35, read as Hen. VIII, c., e.g. 21 Hen. VIII, e. 20
    ('Viet. No.', 'Stat. Vict.', false),  -- 35, read as Vict. No., e.g. 0 Viet. No. 31
    ('Geo. IIt. c.', 'Stat. Geo.', false),  -- 34, read as Geo. IIl. c., e.g. 14 Geo. IIt. c. 4
    ('Vicl. C.', 'Stat. Vict.', false),  -- 34, read as Vict. C., e.g. 11 Vicl. C. 31
    ('Viet e.', 'Stat. Vict.', false),  -- 34, read as Vict e., e.g. 10 Viet e. 95
    ('Gco. IV. cap.', 'Stat. Geo.', false),  -- 33, read as Geo. IV. cap., e.g. 0 Gco. IV. cap. 129
    ('Gco. c.', 'Stat. Geo.', false),  -- 33, read as Geo. c., e.g. 10 Gco. c. 28
    ('Hen. VIl. c.', 'Stat. Hen.', false),  -- 33, read as Hen. VII. c., e.g. 11 Hen. VIl. c. 17
    ('Rie. II. c.', 'Stat. Ric.', false),  -- 33, read as Ric. II. c., e.g. 11 Rie. II. c. 10
    ('Ann, e.', 'Stat. Anne', false),  -- 32, read as Ann, c., e.g. 0 Ann, e. 10
    ('Gco. I. st.', 'Stat. Geo.', false),  -- 32, read as Geo. I. st., e.g. 1 Gco. I. st. 2
    ('Vicl.. c.', 'Stat. Vict.', false),  -- 32, read as Vict.. c., e.g. 0 Vicl.. c. 59
    ('Wm. IV., e.', 'Stat. Will.', false),  -- 32, read as Wm. IV., c., e.g. 1 Wm. IV., e. 20
    ('Ceo. V. c.', 'Stat. Geo.', false),  -- 31, read as Geo. V. c., e.g. 10 Ceo. V.    c. 63
    ('Gco. , c.', 'Stat. Geo.', false),  -- 31, read as Geo. , c., e.g. 11 Gco. , c. 19
    ('Geo. Il. e.', 'Stat. Geo.', false),  -- 31, read as Geo. II. e., e.g. 10 Geo. Il. e. 51
    ('Vict . e.', 'Stat. Vict.', false),  -- 31, read as Vict . c., e.g. 0 Vict . e. 60
    ('Vict. d.', 'Stat. Vict.', false),  -- 31, read as Vict. cl., e.g. 10 Vict. d. 28
    ('Ceo. Ill., c.', 'Stat. Geo.', false),  -- 30, read as Geo. Ill., c., e.g. 12 Ceo. Ill., c. 72
    ('Gco. II. cap.', 'Stat. Geo.', false),  -- 30, read as Geo. II. cap., e.g. 12 Gco. II. cap. 29
    ('Gco. Ill. o.', 'Stat. Geo.', false),  -- 30, read as Geo. Ill. o., e.g. 10 Gco. Ill. o. 18
    ('Geo. IlI c.', 'Stat. Geo.', false),  -- 30, read as Geo. III c., e.g. 10 Geo. IlI c. 1
    ('Hen. IV. e.', 'Stat. Hen.', false),  -- 30, read as Hen. IV. c., e.g. 13 Hen. IV. e. 7
    ('Will. iv. e.', 'Stat. Will.', false),  -- 30, read as Will. iv. c., e.g. 1 Will. iv. e. 37
    ('Geo. IIl c.', 'Stat. Geo.', false),  -- 29, read as Geo. III c., e.g. 11 Geo. IIl c. 53
    ('Geo. IL e.', 'Stat. Geo.', false),  -- 29, read as Geo. IL c., e.g. 11 Geo. IL e. 19
    ('Geo. li. c.', 'Stat. Geo.', false),  -- 29, read as Geo. Ii. c., e.g. 10 Geo. li. c. 50
    ('Viet . c.', 'Stat. Vict.', false),  -- 29, read as Vict . c., e.g. 10 Viet . c. 73
    ('Gco. I., c.', 'Stat. Geo.', false),  -- 28, read as Geo. I., c., e.g. 10 Gco. I., c. 4
    ('Geo. III e.', 'Stat. Geo.', false),  -- 28, read as Geo. III c., e.g. 115 Geo. III e. 28
    ('Hen. Ill. e.', 'Stat. Hen.', false),  -- 28, read as Hen. Ill. c., e.g. 0 Hen. Ill. e. 9
    ('Hen. V1II. c.', 'Stat. Hen.', false),  -- 28, read as Hen. VII. c., e.g. 21 Hen. V1II. c. 7
    ('Jae. I, c.', 'Stat. Jac.', false),  -- 28, read as Jac. I, c., e.g. 1 Jae. I, c. 11
    ('Viet. . .', 'Stat. Vict.', false),  -- 28, read as Vict. . ., e.g. 10 Viet. . . 17
    ('Viet. ..', 'Stat. Vict.', false),  -- 28, read as Vict. .., e.g. 12 Viet. .. 43
    ('Geo. I, e.', 'Stat. Geo.', false),  -- 27, read as Geo. I, c., e.g. 10 Geo. I, e. 50
    ('Jac. I, e.', 'Stat. Jac.', false),  -- 27, read as Jac. I, c., e.g. 1 Jac. I, e. 11
    ('Will. IV, e.', 'Stat. Will.', false),  -- 27, read as Will. IV, c., e.g. 1 Will. IV, e. 05
    ('Anne, cli.', 'Stat. Anne', false),  -- 26, read as Anne, ch., e.g. 10 Anne, cli. 19
    ('Edw. Ill. s.', 'Stat. Edw.', false),  -- 26, read as Edw. Ill. f., e.g. 10 Edw. Ill. s. 3
    ('Edw. Vl. c.', 'Stat. Edw.', false),  -- 26, read as Edw. VI. c., e.g. 1 Edw. Vl. c. 1
    ('Gco. I.', 'Stat. Geo.', false),  -- 26, read as Geo. I., e.g. 0 Gco. I. 1719
    ('Geo. Ilt. c.', 'Stat. Geo.', false),  -- 26, read as Geo. Ill. c., e.g. 13 Geo. Ilt. c. 31
    ('Geo. S. e.', 'Stat. Geo.', false),  -- 26, read as Geo. S. c., e.g. 15 Geo. S. e. 51
    ('Geo. it. c.', 'Stat. Geo.', false),  -- 26, read as Geo. il. c., e.g. 10 Geo. it. c. 28
    ('Viet. e', 'Stat. Vict.', false),  -- 26, read as Vict. e, e.g. 11 Viet. e, 112
    ('Edw. Il.', 'Stat. Edw.', false),  -- 25, read as Edw. II., e.g. 0 Edw. Il., 9
    ('Etiz. c.', 'Stat. Eliz.', false),  -- 25, read as Eliz. c., e.g. 113 Etiz. c. 5
    ('Gco. IIL c.', 'Stat. Geo.', false),  -- 25, read as Geo. IIL c., e.g. 14 Gco. IIL c. 78
    ('Geo. IIl.', 'Stat. Geo.', false),  -- 25, read as Geo. Ill., e.g. 23 Geo. IIl. 5
    ('Vict., eh.', 'Stat. Vict.', false),  -- 25, read as Vict., ch., e.g. 10 Vict., eh. 03
    ('Viet', 'Stat. Vict.', false),  -- 25, read as Vict, e.g. 10 Viet, 4
    ('Viet..', 'Stat. Vict.', false),  -- 25, read as Vict.., e.g. 0 Viet.. 100
    ('Will. Il. c.', 'Stat. Will.', false),  -- 25, read as Will. II. c., e.g. 0 Will. Il. c. 15
    ('viet. c.', 'Stat. Vict.', false),  -- 25, read as vict. c., e.g. 12 viet. c. 42
    ('Gco .', 'Stat. Geo.', false),  -- 24, read as Geo ., e.g. 10 Gco . 1
    ('Geo. Il.c.', 'Stat. Geo.', false),  -- 24, read as Geo. II.c., e.g. 11 Geo. Il.c. 19
    ('Geo. iv. e.', 'Stat. Geo.', false),  -- 24, read as Geo. iv. c., e.g. 2 Geo. iv. e. 39
    ('Ph. & M. e.', 'Stat. Ph. & M.', false),  -- 24, read as Ph. & M. c., e.g. 2 Ph. & M. e. 10
    ('Vict. ce.', 'Stat. Vict.', false),  -- 24, read as Vict. cc., e.g. 0 Vict. ce. 101
    ('Gco. IV., cap.', 'Stat. Geo.', false),  -- 23, read as Geo. IV., cap., e.g. 10 Gco. IV., cap. 55
    ('Geo. IlL, c.', 'Stat. Geo.', false),  -- 23, read as Geo. IIL, c., e.g. 19 Geo. IlL, c. 44
    ('Geo. ilI. c.', 'Stat. Geo.', false),  -- 23, read as Geo. ill. c., e.g. 13 Geo. ilI. c. 31
    ('Geo. l, c.', 'Stat. Geo.', false),  -- 23, read as Geo. I, c., e.g. 0 Geo. l, c. 18
    ('Ric. II. e.', 'Stat. Ric.', false),  -- 23, read as Ric. II. c., e.g. 11 Ric. II. e. 10
    ('VIet. c.', 'Stat. Vict.', false),  -- 23, read as VIct. c., e.g. 10 VIet. c. 3
    ('Viet, e.', 'Stat. Vict.', false),  -- 23, read as Vict, e., e.g. 11 Viet, e. 82
    ('Anne e.', 'Stat. Anne', false),  -- 22, read as Anne c., e.g. 0 Anne e. 31
    ('Ehz. c.', 'Stat. Eliz.', false),  -- 22, read as Eliz. c., e.g. 13 Ehz. c. 20
    ('Gco. i. c.', 'Stat. Geo.', false),  -- 22, read as Geo. i. c., e.g. 12 Gco. i. c. 21
    ('Hen. Vl. c.', 'Stat. Hen.', false),  -- 22, read as Hen. VI. c., e.g. 11 Hen. Vl. c. 8
    ('Will. lV. c.', 'Stat. Will.', false),  -- 22, read as Will. IV. c., e.g. 1 Will. lV. c. 47
    ('Annc, st.', 'Stat. Anne', false),  -- 21, read as Anne, st., e.g. 12 Annc, st. 1
    ('Gco. II c.', 'Stat. Geo.', false),  -- 21, read as Geo. II c., e.g. 10 Gco. II c. 32
    ('Gco. il. c.', 'Stat. Geo.', false),  -- 21, read as Geo. il. c., e.g. 112 Gco. il. c. 72
    ('Geo. IIL e.', 'Stat. Geo.', false),  -- 21, read as Geo. IIL c., e.g. 10 Geo. IIL e. 39
    ('Ann. ft.', 'Stat. Anne', false),  -- 20, read as Ann. st., e.g. 12 Ann. ft. 1
    ('CEO.', 'Stat. Geo.', false),  -- 20, read as GEO., e.g. 11 CEO. 5
    ('Car. lI. c.', 'Stat. Car.', false),  -- 20, read as Car. II. c., e.g. 12 Car. lI. c. 24
    ('Ceo. Ill.', 'Stat. Geo.', false),  -- 20, read as Geo. Ill., e.g. 11 Ceo. Ill., 2
    ('Edw. II. e.', 'Stat. Edw.', false),  -- 20, read as Edw. II. c., e.g. 12 Edw. II. e. 3
    ('Edw. VII, e.', 'Stat. Edw.', false),  -- 20, read as Edw. VII, c., e.g. 0 Edw. VII, e. 23
    ('Gco. Ill. c', 'Stat. Geo.', false),  -- 20, read as Geo. Ill. c, e.g. 12 Gco. Ill. c, 72
    ('Jac. l, c.', 'Stat. Jac.', false),  -- 20, read as Jac. I, c., e.g. 1 Jac. l, c. 16
    ('Vicl. cap.', 'Stat. Vict.', false),  -- 20, read as Vict. cap., e.g. 15 Vicl. cap. 27
    ('Viet, o.', 'Stat. Vict.', false),  -- 20, read as Vict, o., e.g. 11 Viet, o. 95
    ('VlCT. c.', 'Stat. Vict.', false),  -- 20, read as VICT. c., e.g. 14 VlCT. c. 93
    ('ii', NULL, true),  -- 3696, read as n, e.g. 001 ii, 599
    ('Wol.', NULL, true),  -- 986, read as Vol., e.g. 17 Wol. 445
    ('l', NULL, true),  -- 939, read as I, e.g. 000    l 3
    ('BI. Cor.', NULL, true),  -- 843, read as Bl. Cor., e.g. 11 BI. Cor. 478
    ('lIen.', NULL, true),  -- 843, read as llen., e.g. 0 lIen. 20
    ('W. c.', NULL, true),  -- 832, read as V. c., e.g. 0 W. c. 0
    ('Cye.', NULL, true),  -- 779, read as Cyc., e.g. 0 Cye. 1
    ('CI.', NULL, true),  -- 778, read as Cl., e.g. 01 CI. 5
    ('Iid.', NULL, true),  -- 764, read as lid., e.g. 00 Iid. 580
    ('Sec', NULL, true),  -- 736, read as See, e.g. 001 Sec 51
    ('Dee.', NULL, true),  -- 726, read as Dec., e.g. 000 Dee. 1
    ('e.', NULL, true),  -- 723, read as c., e.g. 001    e. 68
    ('l .', NULL, true),  -- 699, read as I ., e.g. 00 l . 1
    ('Chit. Pl.', NULL, true),  -- 678, read as Chit. PI., e.g. 073 Chit. Pl. 1031
    ('G. S.', NULL, true),  -- 640, read as C. S., e.g. 01 G. S. 1913
    ('Sco.', NULL, true),  -- 631, read as Seo., e.g. 0 Sco. 050
    ('Gro.', NULL, true),  -- 607, read as Cro., e.g. 0 Gro. 1
    ('Ilen.', NULL, true),  -- 604, read as llen., e.g. 0 Ilen. 6
    ('If.', NULL, true),  -- 599, read as Is., e.g. 001 If. 2
    ('V.', NULL, true),  -- 583, read as W., e.g. 00    V. 15
    ('Blac. Com.', NULL, true),  -- 581, read as Blac. Corn., e.g. 0 Blac. Com. 310
    ('in', NULL, true),  -- 575, read as iii, e.g. 000 in    1
    ('cd.', NULL, true),  -- 549, read as ed., e.g. 017    cd. 2
    ('e', NULL, true),  -- 548, read as c, e.g. 000    e 6
    ('Ell. & BI.', NULL, true),  -- 539, read as Ell. & Bl., e.g. 0 Ell. & BI. 284
    ('Inft.', NULL, true),  -- 532, read as Inst., e.g. 1 Inft. 1
    ('BI. Coin.', NULL, true),  -- 529, read as Bl. Coin., e.g. 114 BI. Coin. 296
    ('pt.', NULL, true),  -- 523, read as pl., e.g. 101 pt. 6
    ('st.', NULL, true),  -- 522, read as ft., e.g. 008    st. 1
    ('Vi.', NULL, true),  -- 512, read as Wi., e.g. 01 Vi. 207
    ('ni.', NULL, true),  -- 487, read as iii., e.g. 007 ni. 2
    ('G. L.', NULL, true),  -- 470, read as C. L., e.g. 0 G. L. 201
    ('el seq.', NULL, true),  -- 468, read as et seq., e.g. 001 el seq., 1003
    ('Ien.', NULL, true),  -- 457, read as len., e.g. 0 Ien. 111
    ('(co.', NULL, true),  -- 455, read as (eo., e.g. 0 (co. 3
    ('Slat.', NULL, true),  -- 442, read as Stat., e.g. 0 Slat. 1
    ('Iust.', NULL, true),  -- 441, read as Inst., e.g. 0 Iust., 4
    ('G. &', NULL, true),  -- 427, read as C. &, e.g. 0 G. & 1
    ('Hawk. e.', NULL, true),  -- 424, read as Hawk. c., e.g. 1 Hawk. e. 0
    ('. l', NULL, true),  -- 421, read as . I, e.g. 03 . l 2
    ('l.', NULL, true),  -- 411, read as I., e.g. 000 l., 16
    ('cl.', NULL, true),  -- 405, read as d., e.g. 009    cl. 2
    ('aud', NULL, true),  -- 399, read as and, e.g. 037 aud 6500
    ('Cco.', NULL, true),  -- 397, read as Ceo., e.g. 00 Cco. 111
    ('(G.', NULL, true),  -- 395, read as (C., e.g. 09 (G. 5641
    ('Ws.', NULL, true),  -- 372, read as Vs., e.g. 004 Ws. 289
    ('IW.', NULL, true),  -- 367, read as IV., e.g. 10 IW. 11
    ('Oco.', NULL, true),  -- 367, read as Oeo., e.g. 0 Oco. 1
    ('(C. A.', NULL, true),  -- 361, read as (G. A., e.g. 018 (C. A. 40
    ('iil.', NULL, true),  -- 356, read as nl., e.g. 02 iil. 22
    ('nI.', NULL, true),  -- 352, read as nl., e.g. 100 nI. 11
    ('f', NULL, true),  -- 351, read as s, e.g. 000    f 10
    ('und', NULL, true),  -- 343, read as nnd, e.g. 000 und 17
    ('Sl.', NULL, true),  -- 340, read as St., e.g. 0 Sl. 1
    ('V .', NULL, true),  -- 340, read as W ., e.g. 03 V . 1
    ('J. & V.', NULL, true),  -- 325, read as J. & W., e.g. 01 J. & V. 276
    ('Cul.', NULL, true),  -- 314, read as Gul., e.g. 02 Cul. 200
    ('llen. VIll. c.', NULL, true),  -- 312, read as lIen. VIll. c., e.g. 11 llen. VIll. c. 13
    ('cl seq.', NULL, true),  -- 307, read as ct seq., e.g. 000 cl seq. 4
    ('ct scq.', NULL, true),  -- 307, read as ct seq., e.g. 033 ct scq., 1146
    ('see.', NULL, true),  -- 306, read as sec., e.g. 029 see. 20
    ('I l', NULL, true),  -- 289, read as I I, e.g. 011 I l 1111
    ('BI. Con.', NULL, true),  -- 275, read as Bl. Con., e.g. 12 BI. Con. 110
    ('Sclw. N. P.', NULL, true),  -- 273, read as Selw. N. P., e.g. 1 Sclw. N. P. 0
    ('(l.', NULL, true),  -- 251, read as (t., e.g. 02 (l. 631
    ('Slat. at L.', NULL, true),  -- 251, read as Stat. at L., e.g. 0 Slat. at L. 1132
    ('u.', NULL, true),  -- 250, read as n., e.g. 006 u., 1010
    ('Iev.', NULL, true),  -- 249, read as lev., e.g. 10 Iev. 217
    ('nt.', NULL, true),  -- 249, read as nl., e.g. 00 nt. 1
    ('G. .', NULL, true),  -- 247, read as C. ., e.g. 0 G. . 41
    ('Dcc.', NULL, true),  -- 246, read as Dec., e.g. 000 Dcc. 31
    ('. l.', NULL, true),  -- 241, read as . I., e.g. 001 . l. 5000
    ('ls.', NULL, true),  -- 234, read as Is., e.g. 000 ls. 100
    ('el.', NULL, true),  -- 233, read as et., e.g. 01 el. 15
    ('ti.', NULL, true),  -- 228, read as li., e.g. 051 ti. 1
    ('Co. Lilt.', NULL, true),  -- 227, read as Co. Litt., e.g. 01 Co. Lilt. 122
    ('u', NULL, true),  -- 223, read as n, e.g. 015 u    1015
    ('(li.', NULL, true),  -- 221, read as (h., e.g. 011 (li. 22
    ('Insl.', NULL, true),  -- 220, read as Inst., e.g. 011 Insl. 172
    ('(Cli.', NULL, true),  -- 210, read as (Ch., e.g. 020    (Cli. 5
    ('C. II. c.', NULL, true),  -- 208, read as G. II. c., e.g. 09 C. II. c. 3
    ('l I', NULL, true),  -- 205, read as I I, e.g. 00 l I 0
    ('Gec.', NULL, true),  -- 202, read as Gee., e.g. 10 Gec. 2
    ('IIen. VIll. c.', NULL, true),  -- 199, read as lIen. VIll. c., e.g. 15 IIen. VIll. c. 5
    ('V., e.', NULL, true),  -- 197, read as V., c., e.g. 0 V., e. 109
    ('Ncv. & Man.', NULL, true),  -- 194, read as Nev. & Man., e.g. 1 Ncv. & Man. 1
    ('ct.', NULL, true),  -- 192, read as et., e.g. 043 ct. 68
    ('.l.', NULL, true),  -- 191, read as .I., e.g. 0 .l. 502
    ('Ch. Pl.', NULL, true),  -- 189, read as Ch. PI., e.g. 1 Ch. Pl. 1
    ('Coo.', NULL, true),  -- 184, read as Goo., e.g. 0 Coo. 4
    ('lbid.', NULL, true),  -- 183, read as Ibid., e.g. 0 lbid. 072
    ('ant', NULL, true),  -- 181, read as anl, e.g. 002 ant 27
    ('. n.', NULL, true),  -- 180, read as . ii., e.g. 07 . n. 22
    ('Cli. Ap.', NULL, true),  -- 180, read as Ch. Ap., e.g. 0 Cli. Ap. 483
    ('Rev. Slat.', NULL, true),  -- 178, read as Rev. Stat., e.g. 1 Rev. Slat. 075
    ('Halc', NULL, true),  -- 174, read as Hale, e.g. 12 Halc, 114
    ('Vilt. c.', NULL, true),  -- 173, read as Vitt. c., e.g. 10 Vilt. c. 11
    ('S. l.', NULL, true),  -- 172, read as S. I., e.g. 01 S. l. 5
    ('andt', NULL, true),  -- 167, read as andl, e.g. 02 andt 191
    ('antd', NULL, true),  -- 161, read as anld, e.g. 023 antd 1404
    ('Hawk. P. C. e.', NULL, true),  -- 159, read as Hawk. P. C. c., e.g. 1 Hawk. P. C. e. 05
    ('. e.', NULL, true),  -- 157, read as . c., e.g. 107 . e. 12
    ('Bae. Abr.', NULL, true),  -- 156, read as Bac. Abr., e.g. 1 Bae. Abr. 2
    ('Bult.', NULL, true),  -- 154, read as Bull., e.g. 11 Bult. 2
    ('W. IV. e.', NULL, true),  -- 150, read as W. IV. c., e.g. 0 W. IV. e. 78
    ('Boaw.', NULL, true),  -- 139, read as Boav., e.g. 0 Boaw. 214
    ('Hate', NULL, true),  -- 135, read as Hale, e.g. 10 Hate, 122
    ('fl.', NULL, true),  -- 133, read as ft., e.g. 00 fl. 4
    ('h.', NULL, true),  -- 131, read as li., e.g. 01    h. 13
    ('lnt.', NULL, true),  -- 127, read as Int., e.g. 0 lnt. 214
    ('Vid. c.', NULL, true),  -- 126, read as Vicl. c., e.g. 0 Vid. c. 181
    ('GO.', NULL, true),  -- 125, read as CO., e.g. 00 GO. 044
    ('Pli.', NULL, true),  -- 118, read as Ph., e.g. 0 Pli. 107
    ('hd.', NULL, true),  -- 117, read as lid., e.g. 100 hd. 543
    ('lo', NULL, true),  -- 117, read as to, e.g. 001 lo, 102
    ('(l)', NULL, true),  -- 116, read as (I), e.g. 0 (l), 2
    ('Code Com. Art.', NULL, true),  -- 113, read as Code Corn. Art., e.g. 2 Code Com. Art. 1
    ('Ce.', NULL, true),  -- 112, read as Ge., e.g. 0 Ce. 10
    ('I3s.', NULL, true),  -- 111, read as Is., e.g. 006 I3s. 4
    ('td.', NULL, true),  -- 108, read as ld., e.g. 03 td. 3
    ('Bos. & Pnl.', NULL, true),  -- 105, read as Bos. & Pul., e.g. 1 Bos. & Pnl. 102
    ('Vic. c', NULL, true),  -- 102, read as Vie. c, e.g. 10 Vic. c 111
    ('Sec p.', NULL, true),  -- 101, read as See p., e.g. 0 Sec p. 1
    ('Gl.', NULL, true),  -- 98, read as Cl., e.g. 10 Gl. 127
    ('l,. T.', NULL, true),  -- 98, read as I,. T., e.g. 0 l,. T. 45
    ('Viol. c.', NULL, true),  -- 97, read as Viot. c., e.g. 04 Viol. c. 25
    ('Bos. & Put.', NULL, true),  -- 96, read as Bos. & Pul., e.g. 1 Bos. & Put. 1
    ('Cli. at p.', NULL, true),  -- 96, read as Ch. at p., e.g. 0 Cli. at p. 3
    ('iet. c.', NULL, true),  -- 96, read as ict. c., e.g. 11 iet. c. 89
    ('Prcst. Conv.', NULL, true),  -- 94, read as Prest. Conv., e.g. 1 Prcst. Conv. 1
    ('(secs.', NULL, true),  -- 93, read as (sees., e.g. 000 (secs. 2797
    ('Gc.', NULL, true),  -- 93, read as Ge., e.g. 0 Gc. 199
    ('Russ. by Grca.', NULL, true),  -- 93, read as Russ. by Grea., e.g. 1 Russ. by Grca. 101
    ('See See.', NULL, true),  -- 93, read as See Sec., e.g. 0 See See. 5
    ('Tann.', NULL, true),  -- 93, read as Taun., e.g. 101 Tann. 188
    ('cd.)', NULL, true),  -- 93, read as ed.), e.g. 0 cd.) 4
    ('Vcnt.', NULL, true),  -- 92, read as Vent., e.g. 1 Vcnt. 1
    ('Vlcr. c.', NULL, true),  -- 91, read as VIcr. c., e.g. 10 Vlcr. c. 93
    ('East, P. C. e.', NULL, true),  -- 89, read as East, P. C. c., e.g. 1 East, P. C. e. 10
    ('Vint. c.', NULL, true),  -- 89, read as Viut. c., e.g. 10 Vint. c. 67
    ('tilt', NULL, true),  -- 89, read as till, e.g. 011 tilt, 111
    ('n1l.', NULL, true),  -- 87, read as nl., e.g. 100 n1l. 3
    ('Bcnth. Jud. Ev.', NULL, true),  -- 86, read as Benth. Jud. Ev., e.g. 1 Bcnth. Jud. Ev. 127
    ('Viu. Abr.', NULL, true),  -- 86, read as Vin. Abr., e.g. 10 Viu. Abr. 423
    ('Code, see.', NULL, true),  -- 85, read as Code, sec., e.g. 10 Code, see. 1164
    ('Vier. c.', NULL, true),  -- 85, read as Vicr. c., e.g. 06 Vier. c. 18
    ('& S Gco.', NULL, true),  -- 84, read as & S Geo., e.g. 4 & S Gco. 5
    ('Molt.', NULL, true),  -- 84, read as Moll., e.g. 10 Molt. 15
    ('llarv. L. Rev.', NULL, true),  -- 84, read as Ilarv. L. Rev., e.g. 10 llarv. L. Rev. 4
    ('(f.', NULL, true),  -- 83, read as (s., e.g. 0 (f. 1
    ('(u).', NULL, true),  -- 83, read as (n)., e.g. 100 (u). 13
    ('V. N.', NULL, true),  -- 83, read as W. N., e.g. 021 V. N. 144
    ('Walk. Cop.', NULL, true),  -- 83, read as Watk. Cop., e.g. 1 Walk. Cop. 1
    ('(Amending See.', NULL, true),  -- 82, read as (Amending Sec., e.g. 10    (Amending See. 1
    (', ii.', NULL, true),  -- 82, read as , n., e.g. 11 , ii. 170
    ('licl.', NULL, true),  -- 82, read as lid., e.g. 106 licl. 1
    ('Gco. Ill. e.', NULL, true),  -- 81, read as Gco. Ill. c., e.g. 10 Gco. Ill. e. 77
    ('(ii.)', NULL, true),  -- 80, read as (n.), e.g. 10 (ii.), 4
    ('tlie', NULL, true),  -- 79, read as the, e.g. 011 tlie 21
    ('Sce', NULL, true),  -- 78, read as See, e.g. 00 Sce 184
    ('Cooper, Eq. Pl.', NULL, true),  -- 76, read as Cooper, Eq. PI., e.g. 1 Cooper, Eq. Pl. 11
    ('G. IV. e.', NULL, true),  -- 76, read as G. IV. c., e.g. 0 G. IV. e. 32
    ('Motl.', NULL, true),  -- 75, read as Moll., e.g. 0 Motl. 211
    ('W. Ill. e.', NULL, true),  -- 75, read as W. Ill. c., e.g. 10 W. Ill. e. 11
    ('(see.', NULL, true),  -- 74, read as (sec., e.g. 000 (see. 36
    ('.e.', NULL, true),  -- 74, read as .c., e.g. 0 .e.    0
    ('W. ii.', NULL, true),  -- 74, read as W. n., e.g. 0 W. ii. 693
    ('(s).', NULL, true),  -- 73, read as (f)., e.g. 078 (s). 70
    ('Ann. Gas.', NULL, true),  -- 73, read as Ann. Cas., e.g. 10 Ann. Gas. 1128
    ('Gco. Il. c.', NULL, true),  -- 73, read as Gco. II. c., e.g. 10 Gco. Il. c. 44
    ('Sec ante, p.', NULL, true),  -- 73, read as See ante, p., e.g. 0 Sec ante, p. 156
    ('lnst', NULL, true),  -- 73, read as Inst, e.g. 10 lnst, 34
    ('(ii)', NULL, true),  -- 72, read as (n), e.g. 110 (ii) 118
    ('Gce.', NULL, true),  -- 72, read as Gee., e.g. 0 Gce. 4
    ('Gco. IV. e.', NULL, true),  -- 72, read as Gco. IV. c., e.g. 0 Gco. IV. e. 16
    ('Rosc', NULL, true),  -- 72, read as Rose, e.g. 1 Rosc, 146
    ('Not. P. L.', NULL, true),  -- 70, read as Nol. P. L., e.g. 1 Not. P. L. 128
    ('ni', NULL, true),  -- 70, read as iii, e.g. 0 ni, 1
    ('of tlie Act of', NULL, true),  -- 69, read as of the Act of, e.g. 101 of tlie Act of 1908
    ('(e), p.', NULL, true),  -- 68, read as (c), p., e.g. 105 (e), p. 164
    ('( l', NULL, true),  -- 67, read as ( I, e.g. 0 ( l 1
    ('Junc', NULL, true),  -- 67, read as June, e.g. 10    Junc, 1878
    ('Prcst. Est.', NULL, true),  -- 67, read as Prest. Est., e.g. 1 Prcst. Est., 10
    ('See see.', NULL, true),  -- 67, read as See sec., e.g. 0 See see. 16
    ('El. e.', NULL, true),  -- 66, read as El. c., e.g. 13 El. e. 1
    ('ict. e.', NULL, true),  -- 66, read as ict. c., e.g. 0 ict. e. 100
    ('W. IIl. c.', NULL, true),  -- 63, read as W. Ill. c., e.g. 10 W. IIl. c. 14
    ('(antc, p.', NULL, true),  -- 62, read as (ante, p., e.g. 007 (antc, p. 227
    ('Act, see.', NULL, true),  -- 62, read as Act, sec., e.g. 03 Act, see. 43
    ('Ciff.', NULL, true),  -- 62, read as Giff., e.g. 1 Ciff. 1
    ('IIarv. L. Rev.', NULL, true),  -- 62, read as Ilarv. L. Rev., e.g. 10 IIarv. L. Rev. 1
    ('Scc.', NULL, true),  -- 62, read as Sec., e.g. 0 Scc. 55
    ('lV.', NULL, true),  -- 62, read as IV., e.g. 06 lV. 358
    ('nud', NULL, true),  -- 62, read as nnd, e.g. 114 nud 83
    ('Ang.', NULL, true),  -- 61, read as Aug., e.g. 10 Ang. 1785
    ('Parl. Dcb.', NULL, true),  -- 61, read as Parl. Deb., e.g. 109 Parl. Dcb. 4
    ('Prcst. Abst.', NULL, true),  -- 61, read as Prest. Abst., e.g. 1 Prcst. Abst. 114
    ('icll.', NULL, true),  -- 61, read as idl., e.g. 013 icll. 256
    ('w.', NULL, true),  -- 61, read as v., e.g. 102 w., 362
    ('Vil. c.', NULL, true),  -- 60, read as Vit. c., e.g. 10 Vil. c. 27
    ('(cd.', NULL, true),  -- 59, read as (ed., e.g. 0 (cd. 1871
    ('Dan. Cli. Pr.', NULL, true),  -- 59, read as Dan. Ch. Pr., e.g. 1 Dan. Cli. Pr. 111
    ('Foubl. Eq. B.', NULL, true),  -- 59, read as Fonbl. Eq. B., e.g. 1 Foubl. Eq. B. 1
    ('idt.', NULL, true),  -- 59, read as idl., e.g. 0 idt. 109
    ('.n.', NULL, true),  -- 58, read as .ii., e.g. 01 .n. 22
    ('Part. Hist.', NULL, true),  -- 58, read as Parl. Hist., e.g. 13 Part. Hist. 704
    ('Slat. L.', NULL, true),  -- 58, read as Stat. L., e.g. 12 Slat. L. 292
    ('Win. IV. e.', NULL, true),  -- 58, read as Win. IV. c., e.g. 1 Win. IV. e. 40
    (', e.', NULL, true),  -- 57, read as , c., e.g. 016 , e. 12
    ('pI.', NULL, true),  -- 57, read as pl., e.g. 080 pI. 2
    ('R. L. e.', NULL, true),  -- 56, read as R. L. c., e.g. 001 R. L. e. 29
    ('Sec note', NULL, true),  -- 56, read as See note, e.g. 12 Sec note 121
    ('Vici. e.', NULL, true),  -- 56, read as Vici. c., e.g. 10 Vici. e. 93
    ('R. G.', NULL, true),  -- 55, read as R. C., e.g. 10 R. G. 22
    ('I7s.', NULL, true),  -- 54, read as Is., e.g. 038 I7s. 4
    ('V. Slat.', NULL, true),  -- 54, read as V. Stat., e.g. 09 V. Slat. 185
    ('Sce.', NULL, true),  -- 53, read as See., e.g. 021 Sce. 8
    ('lien. VIll. e.', NULL, true),  -- 53, read as lien. VIll. c., e.g. 15 lien. VIll. e. 5
    ('G. Il. c.', NULL, true),  -- 52, read as G. II. c., e.g. 11 G. Il. c. 15
    ('. f.', NULL, true),  -- 51, read as . s., e.g. 0 . f. 1
    ('Slats.', NULL, true),  -- 51, read as Stats., e.g. 10 Slats., 162
    ('Stal.', NULL, true),  -- 51, read as Stat., e.g. 10 Stal. 1
    ('Jnne', NULL, true),  -- 50, read as June, e.g. 08 Jnne 4
    ('Vitl. c.', NULL, true),  -- 50, read as Vitt. c., e.g. 10 Vitl. c. 36
    ('tb.', NULL, true),  -- 50, read as lb., e.g. 10 tb., 129
    ('(sec p.', NULL, true),  -- 49, read as (see p., e.g. 000 (sec p. 1
    ('C. . c.', NULL, true),  -- 49, read as G. . c., e.g. 10 C. . c. 18
    ('G. Ill. e.', NULL, true),  -- 49, read as G. Ill. c., e.g. 17 G. Ill. e. 57
    ('Wms. Sannd.', NULL, true),  -- 47, read as Wms. Saund., e.g. 1 Wms. Sannd.    13
    ('ct seq..', NULL, true),  -- 47, read as et seq.., e.g. 066 ct seq.. 2071
    ('& C Vict. c.', NULL, true),  -- 46, read as & G Vict. c., e.g. 5 & C Vict. c. 1
    ('Gco. II. e.', NULL, true),  -- 46, read as Gco. II. c., e.g. 0 Gco. II. e. 10
    ('Rot. Abr.', NULL, true),  -- 46, read as Rol. Abr., e.g. 11 Rot. Abr. 6119
    ('Vic.. c.', NULL, true),  -- 46, read as Vie.. c., e.g. 00 Vic.. c. 47
    ('Fcl.', NULL, true),  -- 45, read as Fel., e.g. 018 Fcl. 372
    ('V. IV. c.', NULL, true),  -- 45, read as W. IV. c., e.g. 10 V. IV. c. 17
    ('Vlct. e.', NULL, true),  -- 45, read as Vlct. c., e.g. 0 Vlct. e. 110
    ('Jau.', NULL, true),  -- 44, read as Jan., e.g. 000 Jau. 2
    ('Yiet. c.', NULL, true),  -- 44, read as Yict. c., e.g. 10 Yiet. c. 95
    ('ul.', NULL, true),  -- 44, read as nl., e.g. 0 ul. 0
    ('Haw. e.', NULL, true),  -- 43, read as Haw. c., e.g. 1 Haw. e. 31
    ('I2s.', NULL, true),  -- 43, read as Is., e.g. 104 I2s. 2
    ('Vtct. c.', NULL, true),  -- 43, read as Vlct. c., e.g. 10 Vtct. c. 04
    ('tnst.', NULL, true),  -- 43, read as lnst., e.g. 1 tnst. 118
    ('& S Viet. c.', NULL, true),  -- 42, read as & S Vict. c., e.g. 2 & S Viet. c. 3
    ('(sec', NULL, true),  -- 42, read as (see, e.g. 101 (sec 5
    ('Fcb.', NULL, true),  -- 42, read as Feb., e.g. 10 Fcb. 1641
    ('VIer. c.', NULL, true),  -- 42, read as VIcr. c., e.g. 0 VIer. c. 20
    ('ln', NULL, true),  -- 42, read as In, e.g. 0 ln, 194
    ('Bae. Ab.', NULL, true),  -- 41, read as Bac. Ab., e.g. 1 Bae. Ab. 109
    ('Scction', NULL, true),  -- 41, read as Section, e.g. 02    Scction 1
    ('Selv. N. P.', NULL, true),  -- 41, read as Selw. N. P., e.g. 1 Selv. N. P. 10
    ('& C Will.', NULL, true),  -- 40, read as & G Will., e.g. 3 & C Will. 4
    ('Blae. Corn.', NULL, true),  -- 40, read as Blac. Corn., e.g. 1 Blae. Corn. 175
    ('Bulf.', NULL, true),  -- 40, read as Buls., e.g. 1 Bulf. 184
    ('ct seq.)', NULL, true),  -- 40, read as et seq.), e.g. 077 ct seq.) 283
    ('C. S. c.', NULL, true),  -- 39, read as G. S. c., e.g. 17 C. S. c. 77
    ('Cromp. & Mces.', NULL, true),  -- 39, read as Cromp. & Mees., e.g. 11 Cromp. & Mces. 191
    ('G. IIl. c.', NULL, true),  -- 39, read as G. Ill. c., e.g. 20 G. IIl. c. 24
    ('Iut.', NULL, true),  -- 39, read as Int., e.g. 0 Iut. 2
    ('Jnly', NULL, true),  -- 39, read as July, e.g. 00 Jnly 20
    ('Mareh', NULL, true),  -- 39, read as March, e.g. 042 Mareh 15
    ('Vic., Cap.', NULL, true),  -- 39, read as Vie., Cap., e.g. 15 Vic., Cap. 93
    ('aiid', NULL, true),  -- 39, read as and, e.g. 01 aiid 1
    ('lieu.', NULL, true),  -- 39, read as lien., e.g. 0 lieu. 1
    ('of tlie', NULL, true),  -- 39, read as of the, e.g. 0 of tlie 40
    ('Cr. Ev.', NULL, true),  -- 38, read as Gr. Ev., e.g. 1 Cr. Ev. 1
    ('Vic. cc.', NULL, true),  -- 38, read as Vie. cc., e.g. 11 Vic. cc. 11
    ('Viel. e.', NULL, true),  -- 38, read as Vicl. e., e.g. 10 Viel. e. 25
    ('of see.', NULL, true),  -- 38, read as of sec., e.g. 12 of see. 528
    ('& S Vict. e.', NULL, true),  -- 37, read as & S Vict. c., e.g. 2 & S Vict. e. 55
    ('C. & H.', NULL, true),  -- 37, read as G. & H., e.g. 1 C. & H. 11
    ('I6s.', NULL, true),  -- 37, read as Is., e.g. 103 I6s. 8
    ('Sec pp.', NULL, true),  -- 37, read as See pp., e.g. 104 Sec pp. 692
    ('VieL c.', NULL, true),  -- 37, read as VicL c., e.g. 10 VieL c. 95
    ('De Gcx, J. & S.', NULL, true),  -- 36, read as De Gex, J. & S., e.g. 1 De Gcx, J. & S. 1
    ('Henry VIll. e.', NULL, true),  -- 36, read as Henry VIll. c., e.g. 21 Henry VIll. e. 5
    ('Tauu.', NULL, true),  -- 36, read as Taun., e.g. 1 Tauu. 121
    ('Virl. c.', NULL, true),  -- 36, read as Virt. c., e.g. 12 Virl. c. 43
    ('Viu. Ab.', NULL, true),  -- 36, read as Vin. Ab., e.g. 12 Viu. Ab. 123
    ('W. and M. e.', NULL, true),  -- 36, read as W. and M. c., e.g. 0 W. and M. e. 7
    ('Scct.', NULL, true),  -- 35, read as Sect., e.g. 0 Scct. 15
    ('Sngd. Pow.', NULL, true),  -- 35, read as Sugd. Pow., e.g. 1 Sngd. Pow. 116
    ('a, ii.', NULL, true),  -- 35, read as a, n., e.g. 0 a, ii. 78
    ('an1d', NULL, true),  -- 35, read as and, e.g. 003 an1d 1910
    ('andI', NULL, true),  -- 35, read as andl, e.g. 000 andI 1
    ('(l).', NULL, true),  -- 34, read as (I)., e.g. 103 (l). 2
    ('Bl. Comrn.', NULL, true),  -- 34, read as Bl. Comm., e.g. 12 Bl. Comrn. 344
    ('East, P. G.', NULL, true),  -- 34, read as East, P. C., e.g. 1 East, P. G. 165
    ('aul', NULL, true),  -- 34, read as anl, e.g. 0 aul 1
    ('Gco. IIl. c.', NULL, true),  -- 33, read as Gco. Ill. c., e.g. 10 Gco. IIl. c. 47
    ('tid.', NULL, true),  -- 33, read as lid., e.g. 104 tid. 596
    ('tlen.', NULL, true),  -- 33, read as llen., e.g. 11 tlen. 7
    ('&e.', NULL, true),  -- 32, read as &c., e.g. 0 &e. 11
    ('. . l', NULL, true),  -- 32, read as . . I, e.g. 111 . . l 14
    ('F1l.', NULL, true),  -- 32, read as Fel., e.g. 011 F1l. 750
    ('I5s.', NULL, true),  -- 32, read as Is., e.g. 029 I5s. 6
    ('Jcbb & Sym.', NULL, true),  -- 32, read as Jebb & Sym., e.g. 1 Jcbb & Sym. 131
    ('Vice. e.', NULL, true),  -- 32, read as Vice. c., e.g. 02 Vice. e. 16
    ('Viei. c.', NULL, true),  -- 32, read as Vici. c., e.g. 10 Viei. c. 591
    ('& Viet. c.', NULL, true),  -- 31, read as & Vict. c., e.g. 12 & Viet. c. 45
    ('(cli.', NULL, true),  -- 31, read as (ch., e.g. 0 (cli. 152
    ('I9s.', NULL, true),  -- 31, read as Is., e.g. 103 I9s. 9
    ('P. & M. e.', NULL, true),  -- 31, read as P. & M. c., e.g. 1 P. & M. e. 12
    ('Seet.', NULL, true),  -- 31, read as Sect., e.g. 106    Seet. 1
    ('Yict. e.', NULL, true),  -- 31, read as Yict. c., e.g. 12 Yict. e. 43
    ('and ii.', NULL, true),  -- 31, read as and n., e.g. 116 and ii. 170
    ('Anu. Cas.', NULL, true),  -- 30, read as Ann. Cas., e.g. 10 Anu. Cas. 111
    ('Benth. Jnd. Ev.', NULL, true),  -- 30, read as Benth. Jud. Ev., e.g. 1 Benth. Jnd. Ev. 129
    ('Jonrn.', NULL, true),  -- 30, read as Journ., e.g. 104 Jonrn. 819
    ('Sirn. N. S.', NULL, true),  -- 30, read as Sim. N. S., e.g. 1 Sirn. N. S. 155
    ('Viol. o.', NULL, true),  -- 30, read as Viot. o., e.g. 0 Viol. o. 47
    ('nole', NULL, true),  -- 30, read as note, e.g. 038 nole 3
    ('(anle, p.', NULL, true),  -- 29, read as (ante, p., e.g. 0 (anle, p. 3
    ('. Il.', NULL, true),  -- 29, read as . It., e.g. 09 . Il. 039
    ('Fict. e.', NULL, true),  -- 29, read as Fict. c., e.g. 11 Fict. e. 66
    ('Hat. P. C.', NULL, true),  -- 29, read as Hal. P. C., e.g. 1 Hat. P. C. 1
    ('Joum.', NULL, true),  -- 29, read as Journ., e.g. 10 Joum. 1
    ('Vlet. e.', NULL, true),  -- 29, read as Vlet. c., e.g. 0 Vlet. e. 100
    ('icl.', NULL, true),  -- 29, read as id., e.g. 101 icl. 26
    ('licn.', NULL, true),  -- 29, read as lien., e.g. 10 licn. 7
    ('(C1h.', NULL, true),  -- 28, read as (Ch., e.g. 132    (C1h. 7
    ('Gco. Ill. K. B.', NULL, true),  -- 28, read as Geo. Ill. K. B., e.g. 21 Gco. Ill. K. B. 1
    ('Sec above, p.', NULL, true),  -- 28, read as See above, p., e.g. 1 Sec above, p. 13
    ('Slark. Ev.', NULL, true),  -- 28, read as Stark. Ev., e.g. 1 Slark. Ev. 171
    ('V. Ill. c.', NULL, true),  -- 28, read as W. Ill. c., e.g. 10 V. Ill. c. 15
    ('Win. IV., e.', NULL, true),  -- 28, read as Win. IV., c., e.g. 1 Win. IV., e. 2
    ('(co. Ill. c.', NULL, true),  -- 27, read as (eo. Ill. c., e.g. 11 (co. Ill. c. 76
    (',. cd.', NULL, true),  -- 27, read as ,. ed., e.g. 15 ,. cd. 90
    ('Cli. PI.', NULL, true),  -- 27, read as Ch. PI., e.g. 1 Cli. PI. 186
    ('Drcw.', NULL, true),  -- 27, read as Drew., e.g. 1 Drcw. 389
    ('Ed. VII. e.', NULL, true),  -- 27, read as Ed. VII. c., e.g. 0 Ed. VII. e. 55
    ('Oco. Ill. c.', NULL, true),  -- 27, read as Oeo. Ill. c., e.g. 12 Oco. Ill. c. 72
    ('Veut.', NULL, true),  -- 27, read as Vent., e.g. 1 Veut. 1
    ('Vic., e.', NULL, true),  -- 27, read as Vic., c., e.g. 11 Vic., e. 102
    ('Vic., o.', NULL, true),  -- 27, read as Vie., o., e.g. 10 Vic., o. 111
    ('Will. & M. e.', NULL, true),  -- 27, read as Will. & M. c., e.g. 1 Will. & M. e. 18
    ('anId', NULL, true),  -- 27, read as anld, e.g. 115 anId 148
    ('antc.', NULL, true),  -- 27, read as ante., e.g. 008 antc. 32
    ('iu.', NULL, true),  -- 27, read as in., e.g. 007 iu. 6
    ('Dow & CI.', NULL, true),  -- 26, read as Dow & Cl., e.g. 1 Dow & CI. 125
    ('Halc, P. C.', NULL, true),  -- 26, read as Hale, P. C., e.g. 1 Halc, P. C. 1
    ('I4s.', NULL, true),  -- 26, read as Is., e.g. 0 I4s. 5
    ('Juue', NULL, true),  -- 26, read as June, e.g. 0 Juue, 1872
    ('Oco. II. c.', NULL, true),  -- 26, read as Oeo. II. c., e.g. 0 Oco. II. c. 36
    ('Oco. IV. c.', NULL, true),  -- 26, read as Oeo. IV. c., e.g. 0 Oco. IV. c. 3
    ('PACE', NULL, true),  -- 26, read as PAGE, e.g. 00    PACE 2
    ('Rev. Stats., e.', NULL, true),  -- 26, read as Rev. Stats., c., e.g. 108 Rev. Stats., e. 17
    ('Viot., e.', NULL, true),  -- 26, read as Viot., c., e.g. 12 Viot., e. 92
    ('lcn.', NULL, true),  -- 26, read as len., e.g. 04 lcn. 17
    ('lhe', NULL, true),  -- 26, read as the, e.g. 101 lhe 11
    ('Antc, p.', NULL, true),  -- 25, read as Ante, p., e.g. 1 Antc, p. 122
    ('Aud.', NULL, true),  -- 25, read as And., e.g. 1 Aud. 105
    ('Aun. Cas.', NULL, true),  -- 25, read as Ann. Cas., e.g. 0 Aun. Cas. 61
    ('Dc Gex, J. & S.', NULL, true),  -- 25, read as De Gex, J. & S., e.g. 1 Dc Gex, J. & S. 149
    ('W. e.', NULL, true),  -- 25, read as V. e., e.g. 10 W. e. 11
    ('W1i.', NULL, true),  -- 25, read as Wi., e.g. 03 W1i. 04
    ('ct seq., infra.', NULL, true),  -- 25, read as et seq., infra., e.g. 017 ct seq., infra. 23
    ('& I Vict. e.', NULL, true),  -- 24, read as & I Vict. c., e.g. 1 & I Vict. e. 105
    ('& l Vict. c.', NULL, true),  -- 24, read as & I Vict. c., e.g. 10 & l Vict. c. 14
    ('(co. IV. c.', NULL, true),  -- 24, read as (eo. IV. c., e.g. 15 (co. IV. c. 83
    ('Anno, e.', NULL, true),  -- 24, read as Anno, c., e.g. 0 Anno, e. 14
    ('BIa. Corn.', NULL, true),  -- 24, read as Bla. Corn., e.g. 1 BIa. Corn. 251
    ('Bt. Corn.', NULL, true),  -- 24, read as Bl. Corn., e.g. 1 Bt. Corn. 47
    ('C1AP.', NULL, true),  -- 24, read as CAP., e.g. 01 C1AP. 2
    ('Cruisc, Dig.', NULL, true),  -- 24, read as Cruise, Dig., e.g. 1 Cruisc, Dig. 1604
    ('Easl, P. C.', NULL, true),  -- 24, read as East, P. C., e.g. 1 Easl, P. C. 180
    ('Fcbruary', NULL, true),  -- 24, read as February, e.g. 10 Fcbruary, 1841
    ('I8s.', NULL, true),  -- 24, read as Is., e.g. 051 I8s. 31
    ('Jnr. N. s.', NULL, true),  -- 24, read as Jur. N. s., e.g. 10 Jnr. N. s. 608
    ('. . l.', NULL, true),  -- 23, read as . . I., e.g. 01 . . l. 31
    ('Bl. Cornm.', NULL, true),  -- 23, read as Bl. Comm., e.g. 1 Bl. Cornm. 138
    ('Cco. Ill. c.', NULL, true),  -- 23, read as Gco. Ill. c., e.g. 13 Cco. Ill. c. 31
    ('G. . e.', NULL, true),  -- 23, read as G. . c., e.g. 17 G. . e. 37
    ('H1ale', NULL, true),  -- 23, read as Hale, e.g. 1 H1ale, , 137
    ('I1t.', NULL, true),  -- 23, read as It., e.g. 109 I1t. 579
    ('Rcg.', NULL, true),  -- 23, read as Reg., e.g. 11 Rcg. 4
    ('Viit. e.', NULL, true),  -- 23, read as Viit. c., e.g. 0 Viit. e. 1
    ('Will. & Mary, e.', NULL, true),  -- 23, read as Will. & Mary, c., e.g. 1 Will. & Mary, e. 14
    ('aiil', NULL, true),  -- 23, read as anl, e.g. 10 aiil 20
    ('lbid., p.', NULL, true),  -- 23, read as Ibid., p., e.g. 0 lbid., p. 9
    ('lns.', NULL, true),  -- 23, read as Ins., e.g. 102 lns. 27
    ('t1o', NULL, true),  -- 23, read as to, e.g. 000 t1o 25
    ('& IS Viet. c.', NULL, true),  -- 22, read as & IS Vict. c., e.g. 17 & IS Viet. c. 1
    ('Arlicle', NULL, true),  -- 22, read as Article, e.g. 123 Arlicle 44
    ('Crn. Dig.', NULL, true),  -- 22, read as Cru. Dig., e.g. 1 Crn. Dig. 101
    ('Elizabeth, e.', NULL, true),  -- 22, read as Elizabeth, c., e.g. 13 Elizabeth, e. 10
    ('Ibid. f.', NULL, true),  -- 22, read as Ibid. s., e.g. 13 Ibid. f. 56
    ('No. Gas.', NULL, true),  -- 22, read as No. Cas., e.g. 1 No. Gas. 362
    ('Rcv. Stat.', NULL, true),  -- 22, read as Rev. Stat., e.g. 1 Rcv. Stat., 704
    ('Seetion', NULL, true),  -- 22, read as Section, e.g. 0 Seetion 419
    ('ct seq., and', NULL, true),  -- 22, read as et seq., and, e.g. 107 ct seq., and 276
    ('v. Buller', NULL, true),  -- 22, read as v. Butler, e.g. 03    v. Buller, 1018
    ('& I Viet. c.', NULL, true),  -- 21, read as & I Vict. c., e.g. 1 & I Viet. c. 8
    ('& Vict. e.', NULL, true),  -- 21, read as & Vict. c., e.g. 18 & Vict. e. 124
    ('Chil. PI.', NULL, true),  -- 21, read as Chit. PI., e.g. 1 Chil. PI. 13
    ('De Cex, J. & S.', NULL, true),  -- 21, read as De Gex, J. & S., e.g. 1 De Cex, J. & S. 14
    ('E1x.', NULL, true),  -- 21, read as Ex., e.g. 0 E1x. 150
    ('Ed. I. e.', NULL, true),  -- 21, read as Ed. I. c., e.g. 13 Ed. I. e. 12
    ('Exeh. Rep.', NULL, true),  -- 21, read as Exch. Rep., e.g. 10 Exeh. Rep. 259
    ('Lead. Gas. Eq.', NULL, true),  -- 21, read as Lead. Cas. Eq., e.g. 1 Lead. Gas. Eq. 120
    ('Victoria, e.', NULL, true),  -- 21, read as Victoria, c., e.g. 11 Victoria, e. 33
    ('Viut. e.', NULL, true),  -- 21, read as Viut. c., e.g. 10 Viut. e. 20
    ('Wrns. Saund.', NULL, true),  -- 21, read as Wms. Saund., e.g. 1 Wrns. Saund. 211
    ('scq.', NULL, true),  -- 21, read as seq., e.g. 015 scq. 501
    ('to see.', NULL, true),  -- 21, read as to sec., e.g. 10 to see. 1
    ('v. Slate', NULL, true),  -- 21, read as v. State, e.g. 112    v. Slate, 8
    ('& S Ceo.', NULL, true),  -- 20, read as & S Geo., e.g. 4 & S Ceo. 5
    ('B1l. Corn.', NULL, true),  -- 20, read as Bl. Corn., e.g. 11 B1l. Corn. 399
    ('Cromp. & Mecs.', NULL, true),  -- 20, read as Cromp. & Mees., e.g. 1 Cromp. & Mecs. 333
    ('Edcn', NULL, true),  -- 20, read as Eden, e.g. 1 Edcn 11
    ('Now.', NULL, true),  -- 20, read as Nov., e.g. 10 Now. 1
    ('of See.', NULL, true),  -- 20, read as of Sec., e.g. 119 of See. 2685
    ('of the Aet of', NULL, true)  -- 20, read as of the Act of, e.g. 10 of the Aet of 1800
ON CONFLICT (reporter_found) DO NOTHING;

-- migrate:down
SET ROLE = law_admin;

-- Remove exactly the rows seeded above, and only while they still carry
-- the classification this migration gave them.
DELETE FROM legalhist.whitelist
 WHERE (junk OR reporter_standard LIKE 'Stat. %')
   AND reporter_found IN (
    'Gco. I. c.',
    'Hen. VIll. e.',
    'Gco',
    'Gco. Ill., c.',
    'Geo. ll. c.',
    'Gco. III, c.',
    'Gco. IV., c.',
    'Ceo. Ill. c.',
    'Gco. II, c.',
    'Ceo. IV. c.',
    'Gco. II., c.',
    'Gco. IV, c.',
    'Gco. V. c.',
    'Ceo. II. c.',
    'Geo. It. c.',
    'Gco. IV. &',
    'Jac. I. e.',
    'Geo. n. c.',
    'Jae. I. c.',
    'Gco. Ill.',
    'Viet. c',
    'W. & M. e.',
    'Geo. Ill., e.',
    'Gco. iv. c.',
    'Viet., e.',
    'Gco. Ill. cap.',
    'Geo. lI. c.',
    'Anue, c.',
    'Will. IV., e.',
    'Geo. I1I. c.',
    'Gco. Ill, c.',
    'Viet. r.',
    'Gco. S. c.',
    'Will. Ill. e.',
    'Gco. . c.',
    'Geo. IlI, c.',
    'Edw. VI. e.',
    'Geo. IlL. c.',
    'Geo. l. c.',
    'Geo. IIl, c.',
    'Geo. IV., e.',
    'Car. Il. c.',
    'Edw. Ill. e.',
    'Gco. I, c.',
    'Geo. V. e.',
    'Gco. Ill. .',
    'Geo. Il, c.',
    'Vict.. e.',
    'Wm. IV. e.',
    'Aune, c.',
    'Geo. III, e.',
    'Geo. IV, e.',
    'Geo. iI. c.',
    'Rie.',
    'Hcn.',
    'Vict. eh.',
    'Gco. II.',
    'Gco. IV. and',
    'Hen. VIIl. c.',
    'Car. I. e.',
    'Gco. III c.',
    'Vict., cli.',
    'Viet.c.',
    'Aun. c.',
    'Heu.',
    'Viet . .',
    'Geo. II, e.',
    'Geo. Il c.',
    'Viet .',
    'Ceo',
    'Hen. VI. e.',
    'Vicl. .',
    'Vict. e .',
    'Viet. ch.',
    'Geo. IIl., c.',
    'Hen. VII. e.',
    'Geo. I.e.',
    'Ceo. I. c.',
    'Vict. n.',
    'Viet., Ch.',
    'Viet., cap.',
    'Viet.. c.',
    'Edw. IIl. c.',
    'Gco. IV.',
    'Vict.e.',
    'Geo. Il., c.',
    'Viet. (.',
    'Viet. cc.',
    'VICT. e.',
    'Vict. ec.',
    'Vill. IV. c.',
    'Anu. c.',
    'Vicl.',
    'Gco. Ill. ch.',
    'Geo. ni. c.',
    'Edw. Il. c.',
    'Gco. Iv. c.',
    'Gco. ill. c.',
    'V. & M. c.',
    'Viet., o.',
    'Car. II, e.',
    'Gco. II. ch.',
    'Car. II., e.',
    'Geo. Il.',
    'Geo. lV. c.',
    'Vicl c.',
    'Vict., Cli.',
    'Eliz., e.',
    'Geo. . e.',
    'Gco. II. .',
    'Gco. in. c.',
    'Geo. II., e.',
    'Hen. VIII, e.',
    'Viet. No.',
    'Geo. IIt. c.',
    'Vicl. C.',
    'Viet e.',
    'Gco. IV. cap.',
    'Gco. c.',
    'Hen. VIl. c.',
    'Rie. II. c.',
    'Ann, e.',
    'Gco. I. st.',
    'Vicl.. c.',
    'Wm. IV., e.',
    'Ceo. V. c.',
    'Gco. , c.',
    'Geo. Il. e.',
    'Vict . e.',
    'Vict. d.',
    'Ceo. Ill., c.',
    'Gco. II. cap.',
    'Gco. Ill. o.',
    'Geo. IlI c.',
    'Hen. IV. e.',
    'Will. iv. e.',
    'Geo. IIl c.',
    'Geo. IL e.',
    'Geo. li. c.',
    'Viet . c.',
    'Gco. I., c.',
    'Geo. III e.',
    'Hen. Ill. e.',
    'Hen. V1II. c.',
    'Jae. I, c.',
    'Viet. . .',
    'Viet. ..',
    'Geo. I, e.',
    'Jac. I, e.',
    'Will. IV, e.',
    'Anne, cli.',
    'Edw. Ill. s.',
    'Edw. Vl. c.',
    'Gco. I.',
    'Geo. Ilt. c.',
    'Geo. S. e.',
    'Geo. it. c.',
    'Viet. e',
    'Edw. Il.',
    'Etiz. c.',
    'Gco. IIL c.',
    'Geo. IIl.',
    'Vict., eh.',
    'Viet',
    'Viet..',
    'Will. Il. c.',
    'viet. c.',
    'Gco .',
    'Geo. Il.c.',
    'Geo. iv. e.',
    'Ph. & M. e.',
    'Vict. ce.',
    'Gco. IV., cap.',
    'Geo. IlL, c.',
    'Geo. ilI. c.',
    'Geo. l, c.',
    'Ric. II. e.',
    'VIet. c.',
    'Viet, e.',
    'Anne e.',
    'Ehz. c.',
    'Gco. i. c.',
    'Hen. Vl. c.',
    'Will. lV. c.',
    'Annc, st.',
    'Gco. II c.',
    'Gco. il. c.',
    'Geo. IIL e.',
    'Ann. ft.',
    'CEO.',
    'Car. lI. c.',
    'Ceo. Ill.',
    'Edw. II. e.',
    'Edw. VII, e.',
    'Gco. Ill. c',
    'Jac. l, c.',
    'Vicl. cap.',
    'Viet, o.',
    'VlCT. c.',
    'ii',
    'Wol.',
    'l',
    'BI. Cor.',
    'lIen.',
    'W. c.',
    'Cye.',
    'CI.',
    'Iid.',
    'Sec',
    'Dee.',
    'e.',
    'l .',
    'Chit. Pl.',
    'G. S.',
    'Sco.',
    'Gro.',
    'Ilen.',
    'If.',
    'V.',
    'Blac. Com.',
    'in',
    'cd.',
    'e',
    'Ell. & BI.',
    'Inft.',
    'BI. Coin.',
    'pt.',
    'st.',
    'Vi.',
    'ni.',
    'G. L.',
    'el seq.',
    'Ien.',
    '(co.',
    'Slat.',
    'Iust.',
    'G. &',
    'Hawk. e.',
    '. l',
    'l.',
    'cl.',
    'aud',
    'Cco.',
    '(G.',
    'Ws.',
    'IW.',
    'Oco.',
    '(C. A.',
    'iil.',
    'nI.',
    'f',
    'und',
    'Sl.',
    'V .',
    'J. & V.',
    'Cul.',
    'llen. VIll. c.',
    'cl seq.',
    'ct scq.',
    'see.',
    'I l',
    'BI. Con.',
    'Sclw. N. P.',
    '(l.',
    'Slat. at L.',
    'u.',
    'Iev.',
    'nt.',
    'G. .',
    'Dcc.',
    '. l.',
    'ls.',
    'el.',
    'ti.',
    'Co. Lilt.',
    'u',
    '(li.',
    'Insl.',
    '(Cli.',
    'C. II. c.',
    'l I',
    'Gec.',
    'IIen. VIll. c.',
    'V., e.',
    'Ncv. & Man.',
    'ct.',
    '.l.',
    'Ch. Pl.',
    'Coo.',
    'lbid.',
    'ant',
    '. n.',
    'Cli. Ap.',
    'Rev. Slat.',
    'Halc',
    'Vilt. c.',
    'S. l.',
    'andt',
    'antd',
    'Hawk. P. C. e.',
    '. e.',
    'Bae. Abr.',
    'Bult.',
    'W. IV. e.',
    'Boaw.',
    'Hate',
    'fl.',
    'h.',
    'lnt.',
    'Vid. c.',
    'GO.',
    'Pli.',
    'hd.',
    'lo',
    '(l)',
    'Code Com. Art.',
    'Ce.',
    'I3s.',
    'td.',
    'Bos. & Pnl.',
    'Vic. c',
    'Sec p.',
    'Gl.',
    'l,. T.',
    'Viol. c.',
    'Bos. & Put.',
    'Cli. at p.',
    'iet. c.',
    'Prcst. Conv.',
    '(secs.',
    'Gc.',
    'Russ. by Grca.',
    'See See.',
    'Tann.',
    'cd.)',
    'Vcnt.',
    'Vlcr. c.',
    'East, P. C. e.',
    'Vint. c.',
    'tilt',
    'n1l.',
    'Bcnth. Jud. Ev.',
    'Viu. Abr.',
    'Code, see.',
    'Vier. c.',
    '& S Gco.',
    'Molt.',
    'llarv. L. Rev.',
    '(f.',
    '(u).',
    'V. N.',
    'Walk. Cop.',
    '(Amending See.',
    ', ii.',
    'licl.',
    'Gco. Ill. e.',
    '(ii.)',
    'tlie',
    'Sce',
    'Cooper, Eq. Pl.',
    'G. IV. e.',
    'Motl.',
    'W. Ill. e.',
    '(see.',
    '.e.',
    'W. ii.',
    '(s).',
    'Ann. Gas.',
    'Gco. Il. c.',
    'Sec ante, p.',
    'lnst',
    '(ii)',
    'Gce.',
    'Gco. IV. e.',
    'Rosc',
    'Not. P. L.',
    'ni',
    'of tlie Act of',
    '(e), p.',
    '( l',
    'Junc',
    'Prcst. Est.',
    'See see.',
    'El. e.',
    'ict. e.',
    'W. IIl. c.',
    '(antc, p.',
    'Act, see.',
    'Ciff.',
    'IIarv. L. Rev.',
    'Scc.',
    'lV.',
    'nud',
    'Ang.',
    'Parl. Dcb.',
    'Prcst. Abst.',
    'icll.',
    'w.',
    'Vil. c.',
    '(cd.',
    'Dan. Cli. Pr.',
    'Foubl. Eq. B.',
    'idt.',
    '.n.',
    'Part. Hist.',
    'Slat. L.',
    'Win. IV. e.',
    ', e.',
    'pI.',
    'R. L. e.',
    'Sec note',
    'Vici. e.',
    'R. G.',
    'I7s.',
    'V. Slat.',
    'Sce.',
    'lien. VIll. e.',
    'G. Il. c.',
    '. f.',
    'Slats.',
    'Stal.',
    'Jnne',
    'Vitl. c.',
    'tb.',
    '(sec p.',
    'C. . c.',
    'G. Ill. e.',
    'Wms. Sannd.',
    'ct seq..',
    '& C Vict. c.',
    'Gco. II. e.',
    'Rot. Abr.',
    'Vic.. c.',
    'Fcl.',
    'V. IV. c.',
    'Vlct. e.',
    'Jau.',
    'Yiet. c.',
    'ul.',
    'Haw. e.',
    'I2s.',
    'Vtct. c.',
    'tnst.',
    '& S Viet. c.',
    '(sec',
    'Fcb.',
    'VIer. c.',
    'ln',
    'Bae. Ab.',
    'Scction',
    'Selv. N. P.',
    '& C Will.',
    'Blae. Corn.',
    'Bulf.',
    'ct seq.)',
    'C. S. c.',
    'Cromp. & Mces.',
    'G. IIl. c.',
    'Iut.',
    'Jnly',
    'Mareh',
    'Vic., Cap.',
    'aiid',
    'lieu.',
    'of tlie',
    'Cr. Ev.',
    'Vic. cc.',
    'Viel. e.',
    'of see.',
    '& S Vict. e.',
    'C. & H.',
    'I6s.',
    'Sec pp.',
    'VieL c.',
    'De Gcx, J. & S.',
    'Henry VIll. e.',
    'Tauu.',
    'Virl. c.',
    'Viu. Ab.',
    'W. and M. e.',
    'Scct.',
    'Sngd. Pow.',
    'a, ii.',
    'an1d',
    'andI',
    '(l).',
    'Bl. Comrn.',
    'East, P. G.',
    'aul',
    'Gco. IIl. c.',
    'tid.',
    'tlen.',
    '&e.',
    '. . l',
    'F1l.',
    'I5s.',
    'Jcbb & Sym.',
    'Vice. e.',
    'Viei. c.',
    '& Viet. c.',
    '(cli.',
    'I9s.',
    'P. & M. e.',
    'Seet.',
    'Yict. e.',
    'and ii.',
    'Anu. Cas.',
    'Benth. Jnd. Ev.',
    'Jonrn.',
    'Sirn. N. S.',
    'Viol. o.',
    'nole',
    '(anle, p.',
    '. Il.',
    'Fict. e.',
    'Hat. P. C.',
    'Joum.',
    'Vlet. e.',
    'icl.',
    'licn.',
    '(C1h.',
    'Gco. Ill. K. B.',
    'Sec above, p.',
    'Slark. Ev.',
    'V. Ill. c.',
    'Win. IV., e.',
    '(co. Ill. c.',
    ',. cd.',
    'Cli. PI.',
    'Drcw.',
    'Ed. VII. e.',
    'Oco. Ill. c.',
    'Veut.',
    'Vic., e.',
    'Vic., o.',
    'Will. & M. e.',
    'anId',
    'antc.',
    'iu.',
    'Dow & CI.',
    'Halc, P. C.',
    'I4s.',
    'Juue',
    'Oco. II. c.',
    'Oco. IV. c.',
    'PACE',
    'Rev. Stats., e.',
    'Viot., e.',
    'lcn.',
    'lhe',
    'Antc, p.',
    'Aud.',
    'Aun. Cas.',
    'Dc Gex, J. & S.',
    'W. e.',
    'W1i.',
    'ct seq., infra.',
    '& I Vict. e.',
    '& l Vict. c.',
    '(co. IV. c.',
    'Anno, e.',
    'BIa. Corn.',
    'Bt. Corn.',
    'C1AP.',
    'Cruisc, Dig.',
    'Easl, P. C.',
    'Fcbruary',
    'I8s.',
    'Jnr. N. s.',
    '. . l.',
    'Bl. Cornm.',
    'Cco. Ill. c.',
    'G. . e.',
    'H1ale',
    'I1t.',
    'Rcg.',
    'Viit. e.',
    'Will. & Mary, e.',
    'aiil',
    'lbid., p.',
    'lns.',
    't1o',
    '& IS Viet. c.',
    'Arlicle',
    'Crn. Dig.',
    'Elizabeth, e.',
    'Ibid. f.',
    'No. Gas.',
    'Rcv. Stat.',
    'Seetion',
    'ct seq., and',
    'v. Buller',
    '& I Viet. c.',
    '& Vict. e.',
    'Chil. PI.',
    'De Cex, J. & S.',
    'E1x.',
    'Ed. I. e.',
    'Exeh. Rep.',
    'Lead. Gas. Eq.',
    'Victoria, e.',
    'Viut. e.',
    'Wrns. Saund.',
    'scq.',
    'to see.',
    'v. Slate',
    '& S Ceo.',
    'B1l. Corn.',
    'Cromp. & Mecs.',
    'Edcn',
    'Now.',
    'of See.',
    'of the Aet of'
);
