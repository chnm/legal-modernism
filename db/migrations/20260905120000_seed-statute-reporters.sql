-- migrate:up
SET ROLE = law_admin;

-- Regnal-year statute citations as a class of their own (issue #246, part of
-- #165). A citation like "3 & 4 Will. 4, c. 74" or "13 Eliz. c. 5" has the
-- shape of a case citation, so the detectors record it (volume 3, reporter
-- "Will.", page 4) and the linker then has to say something about it. Until
-- now the whitelist said one of two wrong things: most regnal spellings were
-- junk ("Vict. c.", "Geo.", "Eliz. c.", "Hen.", "Car."), which lumps a real
-- citation in with OCR noise, and three families were routed to single-volume
-- case reporters whose abbreviation happens to be a monarch's name, where they
-- fail as no_match or, worse, link to a case that was never cited.
--
-- This migration gives them a place to go: one legalhist.reporters row per
-- monarch family, type = 'statute', jurisdiction 'uk:stat'. cite-linker reads
-- the type through GetReporterWhitelist and records skipped_statute for any
-- citation whose spelling maps to such a row, without probing. The rows are
-- per family rather than per spelling because the detector keeps only the
-- monarch's name; the ordinal and the chapter are in citations_unlinked.raw.
-- The "Stat." prefix keeps the names clear of the Will./Edw./Jac. alternate
-- spellings in legalhist.reporters_abbreviations (issue #272).
--
-- Measured against the live database on 2026-09-05 (moml_citations.
-- citations_unlinked rows per spelling; counts below are per row):
--
--   380 junk spellings that begin with a monarch's name and continue with a
--   regnal ordinal or a chapter marker (c., cap., ch., st., sess., or the OCR
--   readings e. and o. of c.), carrying 1,800,318 rows, are rerouted from junk
--   to the statute rows. By family:
--     Stat. Vict.        94 spellings    932,450 rows
--     Stat. Geo.        118 spellings    491,185 rows
--     Stat. Will.        42 spellings     69,860 rows
--     Stat. Edw.         47 spellings     34,727 rows
--     Stat. Jac.          2 spellings      8,727 rows
--     Stat. Car.         22 spellings     64,183 rows
--     Stat. Hen.         12 spellings     73,054 rows
--     Stat. Eliz.         6 spellings     55,375 rows
--     Stat. Anne         28 spellings     58,318 rows
--     Stat. Ric.          7 spellings      5,038 rows
--     Stat. W. & M.       1 spellings      6,056 rows
--     Stat. Ph. & M.      1 spellings      1,345 rows
--   Two junk spellings that match the same pattern are deliberately left as
--   junk: "Ann. Cas." (American Annotated Cases, a reporter absent from CAP,
--   20,832 rows) and "Geo. Ill. K. B." (474 rows, not a statute form).
--
--   The ten spellings routed to Wil. (Williams' Massachusetts Reports, one
--   volume) are rerouted to Stat. Will.: 165,971 of that reporter's rows are
--   no_match and 151,284 of them cite page 3 or 4, i.e. William III or IV;
--   3,170 of its 6,460 links sit at those pages and are almost certainly wrong.
--   The bare spellings Edw and Edw. (Edwards' Admiralty Reports, one volume)
--   are rerouted to Stat. Edw.: 77,040 of its 110,583 no_match rows cite a
--   page of 1-7 (a regnal ordinal), and 20,183 of its 25,953 English Reports
--   links are at page 1 or 6, i.e. Edward I and VI statutes linked to the
--   first and sixth page of Edwards. Edwards, Edward, Edw. Adm., Edw. P.C.,
--   Edw. PC and Edw. Pr. Cas. stay on Edwards. Jac. is left on Jacob's
--   Chancery Reports: 38,531 of its rows are statutes at page 1 or 2, but
--   19,234 of its 19,696 links are genuine, and only the spellings that carry
--   a chapter marker ("Jac. I. c.") are rerouted. The reporters rows Wil. and
--   Edw survive, with their alternates and single_vol flags; only the routing
--   of the spellings changes.
--
-- Nothing in citation_links is rewritten here: the linker rerun records
-- skipped_statute for these rows, and cite-linker --reset now deletes that
-- status too. The whitelist CHECK constraints require junk and
-- reporter_standard to change together, which the UPDATEs below do.

INSERT INTO legalhist.reporters
    (reporter_standard, reporter_title, level, jurisdiction, year_start, year_end, single_vol, type)
SELECT v.reporter_standard, v.reporter_title, 'national', 'uk:stat', v.year_start, v.year_end, NULL, 'statute'
FROM (VALUES
    ('Stat. Vict.', 'Regnal-year statute citations, reign of Victoria (1837-1901)', 1837, 1901),
    ('Stat. Geo.', 'Regnal-year statute citations, reigns of George I-IV (1714-1830) and George V (1910-1936)', 1714, 1936),
    ('Stat. Will.', 'Regnal-year statute citations, reigns of William III (1689-1702) and William IV (1830-1837)', 1689, 1837),
    ('Stat. Edw.', 'Regnal-year statute citations, reigns of Edward I-VI (1272-1553) and Edward VII (1901-1910)', 1272, 1910),
    ('Stat. Jac.', 'Regnal-year statute citations, reigns of James I and II (1603-1688)', 1603, 1688),
    ('Stat. Car.', 'Regnal-year statute citations, reigns of Charles I and II (1625-1685)', 1625, 1685),
    ('Stat. Hen.', 'Regnal-year statute citations, reigns of Henry III-VIII (1216-1547)', 1216, 1547),
    ('Stat. Eliz.', 'Regnal-year statute citations, reign of Elizabeth I (1558-1603)', 1558, 1603),
    ('Stat. Anne', 'Regnal-year statute citations, reign of Anne (1702-1714)', 1702, 1714),
    ('Stat. Ric.', 'Regnal-year statute citations, reigns of Richard I-III (1189-1485)', 1189, 1485),
    ('Stat. W. & M.', 'Regnal-year statute citations, reign of William and Mary (1689-1694)', 1689, 1694),
    ('Stat. Ph. & M.', 'Regnal-year statute citations, reign of Philip and Mary (1553-1558)', 1553, 1558)
) AS v(reporter_standard, reporter_title, year_start, year_end)
WHERE NOT EXISTS (
    SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = v.reporter_standard
);

-- Junk spellings that are regnal-year citations, rerouted by monarch family.
-- Only rows still marked junk are touched, so applying this twice is a no-op
-- and a spelling someone has since re-pointed by hand is left alone.
UPDATE legalhist.whitelist w
   SET junk = false, reporter_standard = v.reporter_standard
  FROM (VALUES
    ('Vict. c.', 'Stat. Vict.'),  -- 746664
    ('Vict., c.', 'Stat. Vict.'),  -- 39152
    ('Vict. e.', 'Stat. Vict.'),  -- 24355
    ('Vict. o.', 'Stat. Vict.'),  -- 22902
    ('Vict. cap.', 'Stat. Vict.'),  -- 10452
    ('Vict.', 'Stat. Vict.'),  -- 10295
    ('Vict c.', 'Stat. Vict.'),  -- 8120
    ('VICT. c.', 'Stat. Vict.'),  -- 6684
    ('Vict. .', 'Stat. Vict.'),  -- 6272
    ('Vict, c.', 'Stat. Vict.'),  -- 5293
    ('Vict. c', 'Stat. Vict.'),  -- 4892
    ('VICT. C.', 'Stat. Vict.'),  -- 4632
    ('Vict., cap.', 'Stat. Vict.'),  -- 3064
    ('Vict. ch.', 'Stat. Vict.'),  -- 3061
    ('VICT. CAP.', 'Stat. Vict.'),  -- 2870
    ('Vict. C.', 'Stat. Vict.'),  -- 2583
    ('Vict. cc.', 'Stat. Vict.'),  -- 2491
    ('Vict., Ch.', 'Stat. Vict.'),  -- 2138
    ('Vict. a.', 'Stat. Vict.'),  -- 2086
    ('Vict.c.', 'Stat. Vict.'),  -- 1807
    ('Vict., ch.', 'Stat. Vict.'),  -- 1348
    ('Vict . .', 'Stat. Vict.'),  -- 1153
    ('Vict.. c.', 'Stat. Vict.'),  -- 1054
    ('Vict. No.', 'Stat. Vict.'),  -- 991
    ('Vict., e.', 'Stat. Vict.'),  -- 885
    ('Vict .', 'Stat. Vict.'),  -- 857
    ('Vict., Oh.', 'Stat. Vict.'),  -- 852
    ('Vict., o.', 'Stat. Vict.'),  -- 681
    ('Vict. r.', 'Stat. Vict.'),  -- 678
    ('Vict. chap.', 'Stat. Vict.'),  -- 664
    ('Vict . c.', 'Stat. Vict.'),  -- 537
    ('VICT., CAP.', 'Stat. Vict.'),  -- 522
    ('Vict. s.', 'Stat. Vict.'),  -- 466
    ('Vict', 'Stat. Vict.'),  -- 437
    ('Vict..', 'Stat. Vict.'),  -- 429
    ('vict. c.', 'Stat. Vict.'),  -- 420
    ('VICT.', 'Stat. Vict.'),  -- 404
    ('Vict., chap.', 'Stat. Vict.'),  -- 399
    ('Vict. (.', 'Stat. Vict.'),  -- 352
    ('Vict. Cap.', 'Stat. Vict.'),  -- 322
    ('VIct. c.', 'Stat. Vict.'),  -- 298
    ('Vict e.', 'Stat. Vict.'),  -- 287
    ('VICT. o.', 'Stat. Vict.'),  -- 274
    ('Vict., c', 'Stat. Vict.'),  -- 274
    ('Vict. ..', 'Stat. Vict.'),  -- 259
    ('Vict. Vict. c.', 'Stat. Vict.'),  -- 251
    ('Vict., C.', 'Stat. Vict.'),  -- 241
    ('Vict, e.', 'Stat. Vict.'),  -- 237
    ('Vict. Ch.', 'Stat. Vict.'),  -- 235
    ('Vict.,c.', 'Stat. Vict.'),  -- 231
    ('Vict. . .', 'Stat. Vict.'),  -- 221
    ('Vict. e', 'Stat. Vict.'),  -- 213
    ('Vict. sess.', 'Stat. Vict.'),  -- 212
    ('Vict. c..', 'Stat. Vict.'),  -- 210
    ('VICT. CH.', 'Stat. Vict.'),  -- 209
    ('Vict. c. .', 'Stat. Vict.'),  -- 207
    ('Vict o.', 'Stat. Vict.'),  -- 203
    ('Vict. cl.', 'Stat. Vict.'),  -- 203
    ('Vict, o.', 'Stat. Vict.'),  -- 201
    ('Vict., No.', 'Stat. Vict.'),  -- 195
    ('Vict., .', 'Stat. Vict.'),  -- 194
    ('Vict., Cap.', 'Stat. Vict.'),  -- 194
    ('Vict. to', 'Stat. Vict.'),  -- 192
    ('Vict.. .', 'Stat. Vict.'),  -- 192
    ('VICT., c.', 'Stat. Vict.'),  -- 187
    ('VIcT. c.', 'Stat. Vict.'),  -- 183
    ('Vict. oh.', 'Stat. Vict.'),  -- 183
    ('Vict. By', 'Stat. Vict.'),  -- 182
    ('Vict. cli.', 'Stat. Vict.'),  -- 171
    ('VICT. .', 'Stat. Vict.'),  -- 170
    ('Vict. a', 'Stat. Vict.'),  -- 163
    ('VICT., C.', 'Stat. Vict.'),  -- 155
    ('ViCt. c.', 'Stat. Vict.'),  -- 155
    ('Vict,, c.', 'Stat. Vict.'),  -- 155
    ('Vict..c.', 'Stat. Vict.'),  -- 155
    ('Vict c', 'Stat. Vict.'),  -- 153
    ('VICT. CHAP.', 'Stat. Vict.'),  -- 152
    ('Vict, .', 'Stat. Vict.'),  -- 150
    ('VICT. a.', 'Stat. Vict.'),  -- 149
    ('Vict c. .', 'Stat. Vict.'),  -- 138
    ('Vict. u.', 'Stat. Vict.'),  -- 130
    ('Vict, cap.', 'Stat. Vict.'),  -- 129
    ('Vict. ,.', 'Stat. Vict.'),  -- 122
    ('Vict. c .', 'Stat. Vict.'),  -- 121
    ('Vict. Sect.', 'Stat. Vict.'),  -- 116
    ('vict.', 'Stat. Vict.'),  -- 116
    ('Vict. p.', 'Stat. Vict.'),  -- 110
    ('Vict. c. c.', 'Stat. Vict.'),  -- 104
    ('VicT. c.', 'Stat. Vict.'),  -- 102
    ('Vict. co.', 'Stat. Vict.'),  -- 102
    ('Vict,', 'Stat. Vict.'),  -- 0
    ('Vict. Act,', 'Stat. Vict.'),  -- 0
    ('Vict. c,', 'Stat. Vict.'),  -- 0
    ('Vict.,', 'Stat. Vict.'),  -- 0
    ('Geo.', 'Stat. Geo.'),  -- 297295
    ('Geo. Ill. c.', 'Stat. Geo.'),  -- 51724
    ('Geo. IV. c.', 'Stat. Geo.'),  -- 34989
    ('Geo. II. c.', 'Stat. Geo.'),  -- 30412
    ('Geo. I. c.', 'Stat. Geo.'),  -- 7798
    ('Geo. Ill., c.', 'Stat. Geo.'),  -- 5973
    ('Geo. IV., c.', 'Stat. Geo.'),  -- 3859
    ('Geo', 'Stat. Geo.'),  -- 3367
    ('Geo. III, c.', 'Stat. Geo.'),  -- 3335
    ('Geo. II., c.', 'Stat. Geo.'),  -- 3047
    ('Geo. II, c.', 'Stat. Geo.'),  -- 2661
    ('GEO.', 'Stat. Geo.'),  -- 2395
    ('Geo. IV. &', 'Stat. Geo.'),  -- 1989
    ('Geo. V. c.', 'Stat. Geo.'),  -- 1956
    ('Geo. IV, c.', 'Stat. Geo.'),  -- 1848
    ('Geo. S. c.', 'Stat. Geo.'),  -- 1558
    ('Geo. Ill.', 'Stat. Geo.'),  -- 1524
    ('Geo. iv. c.', 'Stat. Geo.'),  -- 1205
    ('Geo. Ill. e.', 'Stat. Geo.'),  -- 1187
    ('Geo. . c.', 'Stat. Geo.'),  -- 1143
    ('Geo. IV. e.', 'Stat. Geo.'),  -- 1053
    ('Geo. I, c.', 'Stat. Geo.'),  -- 996
    ('Geo. IV. and', 'Stat. Geo.'),  -- 980
    ('Geo. Ill. .', 'Stat. Geo.'),  -- 941
    ('Geo. Ill. o.', 'Stat. Geo.'),  -- 934
    ('Geo. Ill. cap.', 'Stat. Geo.'),  -- 933
    ('Geo. III c.', 'Stat. Geo.'),  -- 863
    ('Geo. Ill, c.', 'Stat. Geo.'),  -- 831
    ('Geo. Il. c.', 'Stat. Geo.'),  -- 817
    ('Geo. II.', 'Stat. Geo.'),  -- 791
    ('Geo. IV. o.', 'Stat. Geo.'),  -- 747
    ('Geo. I., c.', 'Stat. Geo.'),  -- 725
    ('Geo. Ill. ch.', 'Stat. Geo.'),  -- 688
    ('Geo. in. c.', 'Stat. Geo.'),  -- 666
    ('Geo. II. e.', 'Stat. Geo.'),  -- 653
    ('Geo. IIL c.', 'Stat. Geo.'),  -- 604
    ('Geo. IV. cap.', 'Stat. Geo.'),  -- 596
    ('Geo. Iv. c.', 'Stat. Geo.'),  -- 585
    ('Geo. c.', 'Stat. Geo.'),  -- 563
    ('Geo. IV.', 'Stat. Geo.'),  -- 557
    ('Geo. II. ch.', 'Stat. Geo.'),  -- 540
    ('Geo. IL c.', 'Stat. Geo.'),  -- 527
    ('Geo. II. o.', 'Stat. Geo.'),  -- 497
    ('Geo. I. st.', 'Stat. Geo.'),  -- 484
    ('Geo. IIl. c.', 'Stat. Geo.'),  -- 465
    ('Geo. II c.', 'Stat. Geo.'),  -- 446
    ('Geo. II., ch.', 'Stat. Geo.'),  -- 419
    ('Geo. Ill. c', 'Stat. Geo.'),  -- 404
    ('Geo. II. cap.', 'Stat. Geo.'),  -- 388
    ('Geo. IV. ch.', 'Stat. Geo.'),  -- 381
    ('Geo. ill. c.', 'Stat. Geo.'),  -- 362
    ('Geo. Ill. sess.', 'Stat. Geo.'),  -- 353
    ('Geo. Ill., ch.', 'Stat. Geo.'),  -- 336
    ('Geo. Ill., cap.', 'Stat. Geo.'),  -- 328
    ('GEO. Ill. c.', 'Stat. Geo.'),  -- 325
    ('GEO. IV. c.', 'Stat. Geo.'),  -- 320
    ('Geo. , c.', 'Stat. Geo.'),  -- 317
    ('GEo.', 'Stat. Geo.'),  -- 310
    ('Geo. i. c.', 'Stat. Geo.'),  -- 309
    ('Geo. IV. c', 'Stat. Geo.'),  -- 308
    ('Geo. II. .', 'Stat. Geo.'),  -- 303
    ('Geo. iii. c.', 'Stat. Geo.'),  -- 266
    ('Geo. ii. c.', 'Stat. Geo.'),  -- 252
    ('Geo. I.', 'Stat. Geo.'),  -- 241
    ('Geo. IV., ch.', 'Stat. Geo.'),  -- 241
    ('Geo Ill. c.', 'Stat. Geo.'),  -- 237
    ('Geo. IV., cap.', 'Stat. Geo.'),  -- 236
    ('Geo. .', 'Stat. Geo.'),  -- 232
    ('Geo. II. c', 'Stat. Geo.'),  -- 231
    ('Geo. HI. c.', 'Stat. Geo.'),  -- 211
    ('Geo. IV. .', 'Stat. Geo.'),  -- 211
    ('Geo. Ill. and', 'Stat. Geo.'),  -- 204
    ('Geo .', 'Stat. Geo.'),  -- 195
    ('Geo. il. c.', 'Stat. Geo.'),  -- 193
    ('Geo. I.c.', 'Stat. Geo.'),  -- 190
    ('Geo. IIL. c.', 'Stat. Geo.'),  -- 186
    ('Geo. IV., and', 'Stat. Geo.'),  -- 182
    ('Geo. Ill. &', 'Stat. Geo.'),  -- 182
    ('Geo, Ill. c.', 'Stat. Geo.'),  -- 181
    ('Geo. IL. c.', 'Stat. Geo.'),  -- 179
    ('Geo. I. .', 'Stat. Geo.'),  -- 172
    ('Geo. IV.c.', 'Stat. Geo.'),  -- 170
    ('Geo. I c.', 'Stat. Geo.'),  -- 164
    ('Geo. I. e.', 'Stat. Geo.'),  -- 161
    ('Geo. II. reg.', 'Stat. Geo.'),  -- 154
    ('Geo. II., cap.', 'Stat. Geo.'),  -- 151
    ('Geo IV. c.', 'Stat. Geo.'),  -- 149
    ('Geo. Ii. c.', 'Stat. Geo.'),  -- 149
    ('Geo. IV c.', 'Stat. Geo.'),  -- 147
    ('Geo. IlL c.', 'Stat. Geo.'),  -- 147
    ('Geo II. c.', 'Stat. Geo.'),  -- 134
    ('Geo. I. ch.', 'Stat. Geo.'),  -- 134
    ('Geo. I. stat.', 'Stat. Geo.'),  -- 133
    ('Geo. III, cap.', 'Stat. Geo.'),  -- 133
    ('Geo. II, ch.', 'Stat. Geo.'),  -- 132
    ('Geo. III, ch.', 'Stat. Geo.'),  -- 131
    ('Geo. Ill.c.', 'Stat. Geo.'),  -- 130
    ('Geo. iv. cap.', 'Stat. Geo.'),  -- 126
    ('Geo. I. o.', 'Stat. Geo.'),  -- 125
    ('Geo. IV. cc.', 'Stat. Geo.'),  -- 124
    ('Geo, II. c.', 'Stat. Geo.'),  -- 123
    ('Geo. Ill. chap.', 'Stat. Geo.'),  -- 123
    ('Geo. I., ch.', 'Stat. Geo.'),  -- 120
    ('Geo. Ill c.', 'Stat. Geo.'),  -- 116
    ('GEO. II. c.', 'Stat. Geo.'),  -- 115
    ('GEO. IV. CAP.', 'Stat. Geo.'),  -- 113
    ('Geo. IIT. c.', 'Stat. Geo.'),  -- 113
    ('Geo. I V. c.', 'Stat. Geo.'),  -- 110
    ('Geo. II.c.', 'Stat. Geo.'),  -- 110
    ('Geo. IIL, c.', 'Stat. Geo.'),  -- 108
    ('Geo. Ill. Cap.', 'Stat. Geo.'),  -- 105
    ('Geo . c.', 'Stat. Geo.'),  -- 103
    ('Geo,', 'Stat. Geo.'),  -- 0
    ('Geo. II.,', 'Stat. Geo.'),  -- 0
    ('Geo. IV. c,', 'Stat. Geo.'),  -- 0
    ('Geo. IV.,', 'Stat. Geo.'),  -- 0
    ('Geo. Ill. c,', 'Stat. Geo.'),  -- 0
    ('Geo. Ill.,', 'Stat. Geo.'),  -- 0
    ('Will. IV. c.', 'Stat. Will.'),  -- 31336
    ('Wm.', 'Stat. Will.'),  -- 11079
    ('Wm. IV. c.', 'Stat. Will.'),  -- 4159
    ('Will. Ill. c.', 'Stat. Will.'),  -- 3786
    ('Will. IV., c.', 'Stat. Will.'),  -- 3341
    ('Will. IV. &', 'Stat. Will.'),  -- 2239
    ('Wm. IV., c.', 'Stat. Will.'),  -- 1372
    ('Will. IV, c.', 'Stat. Will.'),  -- 1333
    ('Will. IV. o.', 'Stat. Will.'),  -- 1036
    ('Will. iv. c.', 'Stat. Will.'),  -- 1022
    ('Will. IV. e.', 'Stat. Will.'),  -- 1014
    ('Will IV. c.', 'Stat. Will.'),  -- 667
    ('Will. IV. and', 'Stat. Will.'),  -- 544
    ('Will. IV. cap.', 'Stat. Will.'),  -- 490
    ('Will. IV.', 'Stat. Will.'),  -- 488
    ('Will. Iv. c.', 'Stat. Will.'),  -- 474
    ('Wm. IV, c.', 'Stat. Will.'),  -- 462
    ('Wm. Ill. c.', 'Stat. Will.'),  -- 459
    ('Will. Ill., c.', 'Stat. Will.'),  -- 396
    ('WILL. IV. c.', 'Stat. Will.'),  -- 356
    ('Will. IV., cap.', 'Stat. Will.'),  -- 281
    ('Will. c.', 'Stat. Will.'),  -- 279
    ('Wm. & M. c.', 'Stat. Will.'),  -- 257
    ('Will. IV. r.', 'Stat. Will.'),  -- 239
    ('Wm. Ill., c.', 'Stat. Will.'),  -- 223
    ('Will. IV. .', 'Stat. Will.'),  -- 221
    ('Will. IV. c', 'Stat. Will.'),  -- 217
    ('Will. III, c.', 'Stat. Will.'),  -- 201
    ('Will. II. c.', 'Stat. Will.'),  -- 182
    ('Wm. IV. and', 'Stat. Will.'),  -- 177
    ('Wm. IV. &', 'Stat. Will.'),  -- 176
    ('Will. IV. ch.', 'Stat. Will.'),  -- 172
    ('Wm. III, c.', 'Stat. Will.'),  -- 169
    ('WILL. IV. CAP.', 'Stat. Will.'),  -- 162
    ('Will. IV.c.', 'Stat. Will.'),  -- 136
    ('Will. IV., ch.', 'Stat. Will.'),  -- 135
    ('WILL. IV. C.', 'Stat. Will.'),  -- 127
    ('Wm. IV. o.', 'Stat. Will.'),  -- 121
    ('Will. IV c.', 'Stat. Will.'),  -- 115
    ('Wm. IV. ch.', 'Stat. Will.'),  -- 113
    ('Wm. iv. c.', 'Stat. Will.'),  -- 104
    ('Will. IV.,', 'Stat. Will.'),  -- 0
    ('Edw. I. c.', 'Stat. Edw.'),  -- 5657
    ('Edw. Ill. c.', 'Stat. Edw.'),  -- 4348
    ('Edw. VII. c.', 'Stat. Edw.'),  -- 4286
    ('Edw. VI. c.', 'Stat. Edw.'),  -- 3417
    ('Edw. IV.', 'Stat. Edw.'),  -- 2064
    ('Edw. Ill.', 'Stat. Edw.'),  -- 1700
    ('Edw. Ill. st.', 'Stat. Edw.'),  -- 1240
    ('Edw. I, c.', 'Stat. Edw.'),  -- 964
    ('Edw. I. st.', 'Stat. Edw.'),  -- 881
    ('Edw. I., c.', 'Stat. Edw.'),  -- 781
    ('Edw. I. p.', 'Stat. Edw.'),  -- 742
    ('Edw. I.', 'Stat. Edw.'),  -- 741
    ('Edw. II. c.', 'Stat. Edw.'),  -- 735
    ('Edw. Ill., c.', 'Stat. Edw.'),  -- 554
    ('Edw. III, c.', 'Stat. Edw.'),  -- 553
    ('Edw. II. (No.', 'Stat. Edw.'),  -- 521
    ('Edw. IV. c.', 'Stat. Edw.'),  -- 498
    ('Edw. VII., c.', 'Stat. Edw.'),  -- 496
    ('Edw. VII, c.', 'Stat. Edw.'),  -- 444
    ('Edw. II.', 'Stat. Edw.'),  -- 362
    ('Edw. II. st.', 'Stat. Edw.'),  -- 343
    ('Edw. VI, c.', 'Stat. Edw.'),  -- 336
    ('Edw. Ill. stat.', 'Stat. Edw.'),  -- 323
    ('Edw. VI., c.', 'Stat. Edw.'),  -- 312
    ('Edw. I. stat.', 'Stat. Edw.'),  -- 242
    ('Edw. Ill., st.', 'Stat. Edw.'),  -- 228
    ('Edw. Ill. f.', 'Stat. Edw.'),  -- 198
    ('Edw. I.) c.', 'Stat. Edw.'),  -- 191
    ('Edw. VII. e.', 'Stat. Edw.'),  -- 177
    ('Edw. I., ch.', 'Stat. Edw.'),  -- 174
    ('Edw. VII. o.', 'Stat. Edw.'),  -- 171
    ('Edw. I., st.', 'Stat. Edw.'),  -- 144
    ('Edw. II. f.', 'Stat. Edw.'),  -- 127
    ('Edw. I, st.', 'Stat. Edw.'),  -- 123
    ('Edw. I. o.', 'Stat. Edw.'),  -- 121
    ('Edw. I. .', 'Stat. Edw.'),  -- 109
    ('Edw. I. m.', 'Stat. Edw.'),  -- 109
    ('Edw. I. e.', 'Stat. Edw.'),  -- 108
    ('Edw. Ill. p.', 'Stat. Edw.'),  -- 105
    ('Edw. Ill., ch.', 'Stat. Edw.'),  -- 102
    ('Edw. I,', 'Stat. Edw.'),  -- 0
    ('Edw. I.,', 'Stat. Edw.'),  -- 0
    ('Edw. II,', 'Stat. Edw.'),  -- 0
    ('Edw. III,', 'Stat. Edw.'),  -- 0
    ('Edw. IV,', 'Stat. Edw.'),  -- 0
    ('Edw. IV.,', 'Stat. Edw.'),  -- 0
    ('Edw. Ill.,', 'Stat. Edw.'),  -- 0
    ('Jac. I. c.', 'Stat. Jac.'),  -- 7708
    ('Jac. I, c.', 'Stat. Jac.'),  -- 1019
    ('Car.', 'Stat. Car.'),  -- 39853
    ('Car. II. c.', 'Stat. Car.'),  -- 14657
    ('Car. I. c.', 'Stat. Car.'),  -- 2220
    ('Car. II., c.', 'Stat. Car.'),  -- 1698
    ('Car. II, c.', 'Stat. Car.'),  -- 1590
    ('Car. II. st.', 'Stat. Car.'),  -- 598
    ('Car. II.', 'Stat. Car.'),  -- 533
    ('Car. II. e.', 'Stat. Car.'),  -- 369
    ('Car. II. o.', 'Stat. Car.'),  -- 333
    ('Car. I, c.', 'Stat. Car.'),  -- 283
    ('Car. IL c.', 'Stat. Car.'),  -- 264
    ('Car. I., c.', 'Stat. Car.'),  -- 249
    ('Car. c.', 'Stat. Car.'),  -- 225
    ('Car. II c.', 'Stat. Car.'),  -- 194
    ('Car. II. ch.', 'Stat. Car.'),  -- 191
    ('Car. II. .', 'Stat. Car.'),  -- 183
    ('Car. II. cap.', 'Stat. Car.'),  -- 170
    ('Car. II., ch.', 'Stat. Car.'),  -- 169
    ('Car. II. stat.', 'Stat. Car.'),  -- 157
    ('Car. II. reg.', 'Stat. Car.'),  -- 140
    ('Car. I. sess.', 'Stat. Car.'),  -- 107
    ('Car. II.,', 'Stat. Car.'),  -- 0
    ('Hen.', 'Stat. Hen.'),  -- 37147
    ('Hen. VIll. c.', 'Stat. Hen.'),  -- 17416
    ('Hen. VII. c.', 'Stat. Hen.'),  -- 3027
    ('Hen. VI.', 'Stat. Hen.'),  -- 2853
    ('Hen. VI. c.', 'Stat. Hen.'),  -- 2583
    ('Hen. VIII, c.', 'Stat. Hen.'),  -- 2005
    ('Hen. VIll., c.', 'Stat. Hen.'),  -- 1707
    ('Hen. VII.', 'Stat. Hen.'),  -- 1414
    ('Hen. IV. c.', 'Stat. Hen.'),  -- 1365
    ('Hen. IV.', 'Stat. Hen.'),  -- 1316
    ('Hen. Ill. c.', 'Stat. Hen.'),  -- 1316
    ('Hen. VIll.', 'Stat. Hen.'),  -- 905
    ('Eliz. c.', 'Stat. Eliz.'),  -- 47341
    ('Eliz., c.', 'Stat. Eliz.'),  -- 2502
    ('Eliz.', 'Stat. Eliz.'),  -- 2139
    ('Eliz. e.', 'Stat. Eliz.'),  -- 1297
    ('Eliz. cap.', 'Stat. Eliz.'),  -- 1050
    ('Eliz. o.', 'Stat. Eliz.'),  -- 1046
    ('Anne, c.', 'Stat. Anne'),  -- 23243
    ('Ann. c.', 'Stat. Anne'),  -- 21528
    ('Ann, c.', 'Stat. Anne'),  -- 2061
    ('Ann. st.', 'Stat. Anne'),  -- 1664
    ('Ann.', 'Stat. Anne'),  -- 1242
    ('Anne, st.', 'Stat. Anne'),  -- 986
    ('Ann., c.', 'Stat. Anne'),  -- 808
    ('Anne c.', 'Stat. Anne'),  -- 775
    ('Anne, o.', 'Stat. Anne'),  -- 638
    ('Anne, ch.', 'Stat. Anne'),  -- 634
    ('Anne, e.', 'Stat. Anne'),  -- 620
    ('Anne, stat.', 'Stat. Anne'),  -- 616
    ('Ann. stat.', 'Stat. Anne'),  -- 505
    ('Ann. e.', 'Stat. Anne'),  -- 433
    ('Anne, cap.', 'Stat. Anne'),  -- 356
    ('Anne. c.', 'Stat. Anne'),  -- 309
    ('Ann. o.', 'Stat. Anne'),  -- 286
    ('Ann. c', 'Stat. Anne'),  -- 259
    ('Ann. Cns.', 'Stat. Anne'),  -- 253
    ('Ann. cap.', 'Stat. Anne'),  -- 205
    ('Anne, chap.', 'Stat. Anne'),  -- 202
    ('Ann. ch.', 'Stat. Anne'),  -- 173
    ('Ann., ch.', 'Stat. Anne'),  -- 164
    ('Ann. .', 'Stat. Anne'),  -- 135
    ('Anne, chapter', 'Stat. Anne'),  -- 112
    ('Ann. St.', 'Stat. Anne'),  -- 111
    ('Ann. c,', 'Stat. Anne'),  -- 0
    ('Anne,', 'Stat. Anne'),  -- 0
    ('Ric.', 'Stat. Ric.'),  -- 2170
    ('Ric. II. c.', 'Stat. Ric.'),  -- 1852
    ('Ric. II. st.', 'Stat. Ric.'),  -- 332
    ('Ric. II, c.', 'Stat. Ric.'),  -- 266
    ('Ric. II., c.', 'Stat. Ric.'),  -- 181
    ('Ric. II.', 'Stat. Ric.'),  -- 132
    ('Ric. Ill. c.', 'Stat. Ric.'),  -- 105
    ('W. & M. c.', 'Stat. W. & M.'),  -- 6056
    ('Ph. & M. c.', 'Stat. Ph. & M.')  -- 1345
  ) AS v(reporter_found, reporter_standard)
 WHERE w.reporter_found = v.reporter_found
   AND w.junk = true;

-- The Williams' Massachusetts family, all ten spellings the whitelist routes to Wil.
UPDATE legalhist.whitelist
   SET reporter_standard = 'Stat. Will.'
 WHERE reporter_found IN ('Wil.', 'Will.', 'WILL.', 'will.', 'W ill.', 'Vill.', 'WVill.', 'Wnll.', 'Wi1l.', 'Wi2l.')
   AND reporter_standard = 'Wil.';

-- The bare Edwards' Admiralty spellings; the longer forms keep their reporter.
UPDATE legalhist.whitelist
   SET reporter_standard = 'Stat. Edw.'
 WHERE reporter_found IN ('Edw', 'Edw.')
   AND reporter_standard = 'Edw';

-- migrate:down
SET ROLE = law_admin;

-- Reverse in dependency order: spellings first, then the reporters rows the
-- whitelist's foreign key points at. Each step touches only rows that still
-- carry the value this migration set.
UPDATE legalhist.whitelist
   SET reporter_standard = 'Edw'
 WHERE reporter_found IN ('Edw', 'Edw.')
   AND reporter_standard = 'Stat. Edw.';

UPDATE legalhist.whitelist
   SET reporter_standard = 'Wil.'
 WHERE reporter_found IN ('Wil.', 'Will.', 'WILL.', 'will.', 'W ill.', 'Vill.', 'WVill.', 'Wnll.', 'Wi1l.', 'Wi2l.')
   AND reporter_standard = 'Stat. Will.';

UPDATE legalhist.whitelist
   SET junk = true, reporter_standard = NULL
 WHERE reporter_standard LIKE 'Stat. %'
   AND reporter_found IN (
    'Vict. c.',
    'Vict., c.',
    'Vict. e.',
    'Vict. o.',
    'Vict. cap.',
    'Vict.',
    'Vict c.',
    'VICT. c.',
    'Vict. .',
    'Vict, c.',
    'Vict. c',
    'VICT. C.',
    'Vict., cap.',
    'Vict. ch.',
    'VICT. CAP.',
    'Vict. C.',
    'Vict. cc.',
    'Vict., Ch.',
    'Vict. a.',
    'Vict.c.',
    'Vict., ch.',
    'Vict . .',
    'Vict.. c.',
    'Vict. No.',
    'Vict., e.',
    'Vict .',
    'Vict., Oh.',
    'Vict., o.',
    'Vict. r.',
    'Vict. chap.',
    'Vict . c.',
    'VICT., CAP.',
    'Vict. s.',
    'Vict',
    'Vict..',
    'vict. c.',
    'VICT.',
    'Vict., chap.',
    'Vict. (.',
    'Vict. Cap.',
    'VIct. c.',
    'Vict e.',
    'VICT. o.',
    'Vict., c',
    'Vict. ..',
    'Vict. Vict. c.',
    'Vict., C.',
    'Vict, e.',
    'Vict. Ch.',
    'Vict.,c.',
    'Vict. . .',
    'Vict. e',
    'Vict. sess.',
    'Vict. c..',
    'VICT. CH.',
    'Vict. c. .',
    'Vict o.',
    'Vict. cl.',
    'Vict, o.',
    'Vict., No.',
    'Vict., .',
    'Vict., Cap.',
    'Vict. to',
    'Vict.. .',
    'VICT., c.',
    'VIcT. c.',
    'Vict. oh.',
    'Vict. By',
    'Vict. cli.',
    'VICT. .',
    'Vict. a',
    'VICT., C.',
    'ViCt. c.',
    'Vict,, c.',
    'Vict..c.',
    'Vict c',
    'VICT. CHAP.',
    'Vict, .',
    'VICT. a.',
    'Vict c. .',
    'Vict. u.',
    'Vict, cap.',
    'Vict. ,.',
    'Vict. c .',
    'Vict. Sect.',
    'vict.',
    'Vict. p.',
    'Vict. c. c.',
    'VicT. c.',
    'Vict. co.',
    'Vict,',
    'Vict. Act,',
    'Vict. c,',
    'Vict.,',
    'Geo.',
    'Geo. Ill. c.',
    'Geo. IV. c.',
    'Geo. II. c.',
    'Geo. I. c.',
    'Geo. Ill., c.',
    'Geo. IV., c.',
    'Geo',
    'Geo. III, c.',
    'Geo. II., c.',
    'Geo. II, c.',
    'GEO.',
    'Geo. IV. &',
    'Geo. V. c.',
    'Geo. IV, c.',
    'Geo. S. c.',
    'Geo. Ill.',
    'Geo. iv. c.',
    'Geo. Ill. e.',
    'Geo. . c.',
    'Geo. IV. e.',
    'Geo. I, c.',
    'Geo. IV. and',
    'Geo. Ill. .',
    'Geo. Ill. o.',
    'Geo. Ill. cap.',
    'Geo. III c.',
    'Geo. Ill, c.',
    'Geo. Il. c.',
    'Geo. II.',
    'Geo. IV. o.',
    'Geo. I., c.',
    'Geo. Ill. ch.',
    'Geo. in. c.',
    'Geo. II. e.',
    'Geo. IIL c.',
    'Geo. IV. cap.',
    'Geo. Iv. c.',
    'Geo. c.',
    'Geo. IV.',
    'Geo. II. ch.',
    'Geo. IL c.',
    'Geo. II. o.',
    'Geo. I. st.',
    'Geo. IIl. c.',
    'Geo. II c.',
    'Geo. II., ch.',
    'Geo. Ill. c',
    'Geo. II. cap.',
    'Geo. IV. ch.',
    'Geo. ill. c.',
    'Geo. Ill. sess.',
    'Geo. Ill., ch.',
    'Geo. Ill., cap.',
    'GEO. Ill. c.',
    'GEO. IV. c.',
    'Geo. , c.',
    'GEo.',
    'Geo. i. c.',
    'Geo. IV. c',
    'Geo. II. .',
    'Geo. iii. c.',
    'Geo. ii. c.',
    'Geo. I.',
    'Geo. IV., ch.',
    'Geo Ill. c.',
    'Geo. IV., cap.',
    'Geo. .',
    'Geo. II. c',
    'Geo. HI. c.',
    'Geo. IV. .',
    'Geo. Ill. and',
    'Geo .',
    'Geo. il. c.',
    'Geo. I.c.',
    'Geo. IIL. c.',
    'Geo. IV., and',
    'Geo. Ill. &',
    'Geo, Ill. c.',
    'Geo. IL. c.',
    'Geo. I. .',
    'Geo. IV.c.',
    'Geo. I c.',
    'Geo. I. e.',
    'Geo. II. reg.',
    'Geo. II., cap.',
    'Geo IV. c.',
    'Geo. Ii. c.',
    'Geo. IV c.',
    'Geo. IlL c.',
    'Geo II. c.',
    'Geo. I. ch.',
    'Geo. I. stat.',
    'Geo. III, cap.',
    'Geo. II, ch.',
    'Geo. III, ch.',
    'Geo. Ill.c.',
    'Geo. iv. cap.',
    'Geo. I. o.',
    'Geo. IV. cc.',
    'Geo, II. c.',
    'Geo. Ill. chap.',
    'Geo. I., ch.',
    'Geo. Ill c.',
    'GEO. II. c.',
    'GEO. IV. CAP.',
    'Geo. IIT. c.',
    'Geo. I V. c.',
    'Geo. II.c.',
    'Geo. IIL, c.',
    'Geo. Ill. Cap.',
    'Geo . c.',
    'Geo,',
    'Geo. II.,',
    'Geo. IV. c,',
    'Geo. IV.,',
    'Geo. Ill. c,',
    'Geo. Ill.,',
    'Will. IV. c.',
    'Wm.',
    'Wm. IV. c.',
    'Will. Ill. c.',
    'Will. IV., c.',
    'Will. IV. &',
    'Wm. IV., c.',
    'Will. IV, c.',
    'Will. IV. o.',
    'Will. iv. c.',
    'Will. IV. e.',
    'Will IV. c.',
    'Will. IV. and',
    'Will. IV. cap.',
    'Will. IV.',
    'Will. Iv. c.',
    'Wm. IV, c.',
    'Wm. Ill. c.',
    'Will. Ill., c.',
    'WILL. IV. c.',
    'Will. IV., cap.',
    'Will. c.',
    'Wm. & M. c.',
    'Will. IV. r.',
    'Wm. Ill., c.',
    'Will. IV. .',
    'Will. IV. c',
    'Will. III, c.',
    'Will. II. c.',
    'Wm. IV. and',
    'Wm. IV. &',
    'Will. IV. ch.',
    'Wm. III, c.',
    'WILL. IV. CAP.',
    'Will. IV.c.',
    'Will. IV., ch.',
    'WILL. IV. C.',
    'Wm. IV. o.',
    'Will. IV c.',
    'Wm. IV. ch.',
    'Wm. iv. c.',
    'Will. IV.,',
    'Edw. I. c.',
    'Edw. Ill. c.',
    'Edw. VII. c.',
    'Edw. VI. c.',
    'Edw. IV.',
    'Edw. Ill.',
    'Edw. Ill. st.',
    'Edw. I, c.',
    'Edw. I. st.',
    'Edw. I., c.',
    'Edw. I. p.',
    'Edw. I.',
    'Edw. II. c.',
    'Edw. Ill., c.',
    'Edw. III, c.',
    'Edw. II. (No.',
    'Edw. IV. c.',
    'Edw. VII., c.',
    'Edw. VII, c.',
    'Edw. II.',
    'Edw. II. st.',
    'Edw. VI, c.',
    'Edw. Ill. stat.',
    'Edw. VI., c.',
    'Edw. I. stat.',
    'Edw. Ill., st.',
    'Edw. Ill. f.',
    'Edw. I.) c.',
    'Edw. VII. e.',
    'Edw. I., ch.',
    'Edw. VII. o.',
    'Edw. I., st.',
    'Edw. II. f.',
    'Edw. I, st.',
    'Edw. I. o.',
    'Edw. I. .',
    'Edw. I. m.',
    'Edw. I. e.',
    'Edw. Ill. p.',
    'Edw. Ill., ch.',
    'Edw. I,',
    'Edw. I.,',
    'Edw. II,',
    'Edw. III,',
    'Edw. IV,',
    'Edw. IV.,',
    'Edw. Ill.,',
    'Jac. I. c.',
    'Jac. I, c.',
    'Car.',
    'Car. II. c.',
    'Car. I. c.',
    'Car. II., c.',
    'Car. II, c.',
    'Car. II. st.',
    'Car. II.',
    'Car. II. e.',
    'Car. II. o.',
    'Car. I, c.',
    'Car. IL c.',
    'Car. I., c.',
    'Car. c.',
    'Car. II c.',
    'Car. II. ch.',
    'Car. II. .',
    'Car. II. cap.',
    'Car. II., ch.',
    'Car. II. stat.',
    'Car. II. reg.',
    'Car. I. sess.',
    'Car. II.,',
    'Hen.',
    'Hen. VIll. c.',
    'Hen. VII. c.',
    'Hen. VI.',
    'Hen. VI. c.',
    'Hen. VIII, c.',
    'Hen. VIll., c.',
    'Hen. VII.',
    'Hen. IV. c.',
    'Hen. IV.',
    'Hen. Ill. c.',
    'Hen. VIll.',
    'Eliz. c.',
    'Eliz., c.',
    'Eliz.',
    'Eliz. e.',
    'Eliz. cap.',
    'Eliz. o.',
    'Anne, c.',
    'Ann. c.',
    'Ann, c.',
    'Ann. st.',
    'Ann.',
    'Anne, st.',
    'Ann., c.',
    'Anne c.',
    'Anne, o.',
    'Anne, ch.',
    'Anne, e.',
    'Anne, stat.',
    'Ann. stat.',
    'Ann. e.',
    'Anne, cap.',
    'Anne. c.',
    'Ann. o.',
    'Ann. c',
    'Ann. Cns.',
    'Ann. cap.',
    'Anne, chap.',
    'Ann. ch.',
    'Ann., ch.',
    'Ann. .',
    'Anne, chapter',
    'Ann. St.',
    'Ann. c,',
    'Anne,',
    'Ric.',
    'Ric. II. c.',
    'Ric. II. st.',
    'Ric. II, c.',
    'Ric. II., c.',
    'Ric. II.',
    'Ric. Ill. c.',
    'W. & M. c.',
    'Ph. & M. c.'
   );

DELETE FROM legalhist.reporters
 WHERE type = 'statute'
   AND reporter_standard IN ('Stat. Vict.', 'Stat. Geo.', 'Stat. Will.', 'Stat. Edw.', 'Stat. Jac.', 'Stat. Car.', 'Stat. Hen.', 'Stat. Eliz.', 'Stat. Anne', 'Stat. Ric.', 'Stat. W. & M.', 'Stat. Ph. & M.');
