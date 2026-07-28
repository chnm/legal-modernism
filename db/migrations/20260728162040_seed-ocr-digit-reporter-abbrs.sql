-- migrate:up
SET ROLE = law_admin;

-- Seed legalhist.whitelist with reporter abbreviations the OCR corrupted by
-- reading a letter as a digit: "Fed." scanned as "F1ed.", "Mass." as
-- "M1ass.". GenericOCRDigitDetector finds these citations, and like every
-- other detector it records the spelling as it appeared, so they sit at
-- skipped_not_whitelisted until the whitelist carries the spelling.
--
-- Candidates were compiled by TestOCRDigitWhitelistSuggestions over a
-- 1,052,158-page sample (10% of moml.page_ocrtext) and are recorded in full,
-- including the ones rejected here, in db/whitelist-candidates-ocr-digits.tsv.
--
-- Two filters were applied to the 778 proposals in that file:
--
--   1. Canonical only. The spelling with its digits removed must itself be a
--      reporter in legalhist.reporters, not another whitelist variant that
--      maps onward. This drops proposals that would inherit an existing
--      entry's looseness, e.g. "E1q." -> "Eq." -> C.L.R.
--
--   2. Unambiguous only. Removing the digit assumes the OCR INSERTED it. The
--      OCR can equally well have SUBSTITUTED a digit for a letter, which
--      points at a different reporter, so a candidate is dropped when some
--      letter in place of the digit yields a whitelist entry for a different
--      reporter that is common enough in the corpus to be real competition
--      (at least 10% as frequent as the strip reading in
--      moml_citations.citations_unlinked). Rare alternates do not count:
--      "M1o." could be "Moo." (Mood) in principle, but "Mo." outnumbers it
--      37 to 1. 9 canonical proposals were dropped this way:
--        "M1d." -> Md., but also reads as "Mod." -> Mod at 71% of its frequency; "Mad." -> Madd at 15% of its frequency
--        "M3d." -> Md., but also reads as "Mod." -> Mod at 71% of its frequency; "Mad." -> Madd at 15% of its frequency
--        "M5d." -> Md., but also reads as "Mod." -> Mod at 71% of its frequency; "Mad." -> Madd at 15% of its frequency
--        "M6d." -> Md., but also reads as "Mod." -> Mod at 71% of its frequency; "Mad." -> Madd at 15% of its frequency
--        "Wi1s." -> Wis., but also reads as "Wils." -> Wils KB at 17% of its frequency
--        "Bu1r." -> Bur., but also reads as "Burr." -> Burr at 845% of its frequency
--        "M2d." -> Md., but also reads as "Mod." -> Mod at 71% of its frequency; "Mad." -> Madd at 15% of its frequency
--        "M9d." -> Md., but also reads as "Mod." -> Mod at 71% of its frequency; "Mad." -> Madd at 15% of its frequency
--        "Wi9s." -> Wis., but also reads as "Wils." -> Wils KB at 17% of its frequency
--
-- 197 rows remain. Counts below are occurrences in the 10% sample, so the
-- corpus-wide figure is roughly ten times each.

INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk) VALUES
    ('M1o.', 'Mo.', false),  -- 107, e.g. 85 M1o. 23
    ('M3o.', 'Mo.', false),  -- 47, e.g. 45 M3o.    7
    ('M1ass.', 'Mass.', false),  -- 36, e.g. 106 M1ass. 559
    ('M1o. App.', 'Mo. App.', false),  -- 29, e.g. 37 M1o. App., 576
    ('M1e.', 'Me.', false),  -- 27, e.g. 13 M1e. 216
    ('M3ass.', 'Mass.', false),  -- 25, e.g. 133 M3ass. 78
    ('M3e.', 'Me.', false),  -- 22, e.g. 78 M3e. 514
    ('H1ow.', 'How.', false),  -- 18, e.g. 10 H1ow. 474
    ('M5o.', 'Mo.', false),  -- 18, e.g. 10 M5o. 755
    ('M1ich.', 'Mich.', false),  -- 15, e.g. 22 M1ich. 3
    ('M3ich.', 'Mich.', false),  -- 15, e.g. 48 M3ich. 27
    ('M3o. App.', 'Mo. App.', false),  -- 14, e.g. 11 M3o. App. 555
    ('In1d.', 'Ind.', false),  -- 13, e.g. 6 In1d. 1
    ('P1a.', 'Pa.', false),  -- 13, e.g. 92 P1a. 112
    ('B1arb.', 'Barb.', false),  -- 12, e.g. 33 B1arb. 1
    ('H1ill', 'Hill', false),  -- 12, e.g. 2 H1ill, 243
    ('M1inn.', 'Minn.', false),  -- 11, e.g. 8 M1inn. 13
    ('I1nd.', 'Ind.', false),  -- 9, e.g. 1 I1nd. 622
    ('Ma1ss.', 'Mass.', false),  -- 9, e.g. 147 Ma1ss. 455
    ('B3arb.', 'Barb.', false),  -- 8, e.g. 23 B3arb. 420
    ('M3inn.', 'Minn.', false),  -- 8, e.g. 38 M3inn. 3
    ('K1an.', 'Kan.', false),  -- 7, e.g. 16 K1an. 5
    ('M0o.', 'Mo.', false),  -- 7, e.g. 134 M0o. 162
    ('M3iss.', 'Miss.', false),  -- 7, e.g. 24 M3iss. 520
    ('H1ow. Pr.', 'How. Pr.', false),  -- 6, e.g. 9 H1ow. Pr. 93
    ('L. E1d.', 'L. Ed.', false),  -- 6, e.g. 40 L. E1d. 6134
    ('M1iss.', 'Miss.', false),  -- 6, e.g. 29 M1iss. 21
    ('M3ans.', 'Mans.', false),  -- 6, e.g. 6 M3ans. 256
    ('H1un', 'Hun', false),  -- 5, e.g. 35 H1un 41
    ('Am. Re1p.', 'Am. Rep.', false),  -- 4, e.g. 32 Am. Re1p. 423
    ('H1ilt.', 'Hilt.', false),  -- 4, e.g. 1 H1ilt. 2
    ('K1y.', 'Ky.', false),  -- 4, e.g. 2 K1y. 532
    ('Ma8ss.', 'Mass.', false),  -- 4, e.g. 5 Ma8ss. 40
    ('Am. R1ep.', 'Am. Rep.', false),  -- 3, e.g. 30 Am. R1ep. 58
    ('Am. St. Re1p.', 'Am. St. Rep.', false),  -- 3, e.g. 9 Am. St. Re1p. 211
    ('Ka1n.', 'Kan.', false),  -- 3, e.g. 30 Ka1n. 10
    ('M1isc.', 'Misc.', false),  -- 3, e.g. 12 M1isc. 382
    ('M1ont.', 'Mont.', false),  -- 3, e.g. 7 M1ont. 449
    ('M3et.', 'Met.', false),  -- 3, e.g. 2 M3et., 06
    ('M5ass.', 'Mass.', false),  -- 3, e.g. 144 M5ass. 153
    ('M5e.', 'Me.', false),  -- 3, e.g. 69 M5e. 1
    ('M5inn.', 'Minn.', false),  -- 3, e.g. 30 M5inn. 372
    ('Oh1io St.', 'Ohio St.', false),  -- 3, e.g. 41 Oh1io St. 141
    ('W1ash.', 'Wash.', false),  -- 3, e.g. 12 W1ash. 358
    ('W1is.', 'Wis.', false),  -- 3, e.g. 18 W1is. 88
    ('Ab1b.', 'Abb. Pr.', false),  -- 2, e.g. 2 Ab1b. 382
    ('Am. St. R1ep.', 'Am. St. Rep.', false),  -- 2, e.g. 58 Am. St. R1ep. 1
    ('B1ibb', 'Bibb', false),  -- 2, e.g. 4 B1ibb, 268
    ('Ba3rb.', 'Barb.', false),  -- 2, e.g. 58 Ba3rb. 77
    ('Bar1b.', 'Barb.', false),  -- 2, e.g. 20 Bar1b. 252
    ('C1al.', 'Cal.', false),  -- 2, e.g. 40 C1al. 344
    ('Co1nn.', 'Conn.', false),  -- 2, e.g. 48 Co1nn. 11
    ('F1la.', 'Fla.', false),  -- 2, e.g. 17 F1la. 289
    ('H1are', 'Hare', false),  -- 2, e.g. 7 H1are 251
    ('H2ow.', 'How.', false),  -- 2, e.g. 29 H2ow., 97
    ('How. P1r.', 'How. Pr.', false),  -- 2, e.g. 81 How. P1r., 508
    ('I1nd. App.', 'Ind. App.', false),  -- 2, e.g. 19 I1nd. App. 272
    ('I9nd.', 'Ind.', false),  -- 2, e.g. 193 I9nd. 347
    ('Ind. Ap1p.', 'Ind. App.', false),  -- 2, e.g. 10 Ind. Ap1p. 221
    ('John1s.', 'Johns.', false),  -- 2, e.g. 9 John1s. 349
    ('M. & R1y.', 'M. & Ry.', false),  -- 2, e.g. 3 M. & R1y. 121
    ('M1ason', 'Mason', false),  -- 2, e.g. 4 M1ason, 4
    ('M1oo. & P.', 'Moo. & P.', false),  -- 2, e.g. 4 M1oo. & P. 36
    ('M3acph.', 'Macph.', false),  -- 2, e.g. 2 M3acph. 1357
    ('M3isc.', 'Misc.', false),  -- 2, e.g. 0 M3isc. 427
    ('M3ont.', 'Mont.', false),  -- 2, e.g. 22 M3ont. 312
    ('M5et.', 'Met.', false),  -- 2, e.g. 11 M5et. 331
    ('M5o. App.', 'Mo. App.', false),  -- 2, e.g. 9 M5o. App. 5
    ('Ma3ss.', 'Mass.', false),  -- 2, e.g. 17 Ma3ss. 581
    ('Ma5ss.', 'Mass.', false),  -- 2, e.g. 147 Ma5ss. 5
    ('Mo. Ap1p.', 'Mo. App.', false),  -- 2, e.g. 29 Mo. Ap1p. 125
    ('P1et.', 'Pet.', false),  -- 2, e.g. 13 P1et. 359
    ('P1ick.', 'Pick.', false),  -- 2, e.g. 19 P1ick. 330
    ('T1ex.', 'Tex.', false),  -- 2, e.g. 8 T1ex. 5
    ('W1all.', 'Wall.', false),  -- 2, e.g. 3 W1all. 97
    ('Wa1sh.', 'Wash.', false),  -- 2, e.g. 53 Wa1sh. 168
    ('A1m. St. Rep.', 'Am. St. Rep.', false),  -- 1, e.g. 58 A1m. St. Rep. 627
    ('A4bb.', 'Abb. Pr.', false),  -- 1, e.g. 1 A4bb. 6
    ('A4la.', 'Ala.', false),  -- 1, e.g. 1 A4la. 70
    ('A4m. Dec.', 'Am. Dec.', false),  -- 1, e.g. 0 A4m. Dec.,    553
    ('A9m. Dec.', 'Am. Dec.', false),  -- 1, e.g. 78 A9m. Dec. 632
    ('All1en', 'Allen', false),  -- 1, e.g. 7 All1en 1
    ('Alle1n', 'Allen', false),  -- 1, e.g. 3 Alle1n 1
    ('Am. D1ec.', 'Am. Dec.', false),  -- 1, e.g. 30 Am. D1ec. 75
    ('Am. S8t. Rep.', 'Am. St. Rep.', false),  -- 1, e.g. 30 Am. S8t. Rep. 61
    ('B1all & B.', 'Ball & B.', false),  -- 1, e.g. 1 B1all & B. 255
    ('B1arr', 'Barr', false),  -- 1, e.g. 5 B1arr, 71
    ('B1en.', 'Ben.', false),  -- 1, e.g. 1 B1en. 40
    ('B1inn.', 'Binn.', false),  -- 1, e.g. 2 B1inn. 497
    ('B1radf.', 'Bradf.', false),  -- 1, e.g. 2 B1radf. 357
    ('B1ur.', 'Bur.', false),  -- 1, e.g. 3 B1ur. 1394
    ('B1ush', 'Bush', false),  -- 1, e.g. 11 B1ush, 34
    ('B3eav', 'Beav', false),  -- 1, e.g. 2 B3eav, 153
    ('B3iss.', 'Biss.', false),  -- 1, e.g. 6 B3iss. 98
    ('B3radf.', 'Bradf.', false),  -- 1, e.g. 4 B3radf. 218
    ('B7arb.', 'Barb.', false),  -- 1, e.g. 12 B7arb. 484
    ('Ba1il.', 'Bail.', false),  -- 1, e.g. 29 Ba1il., 59
    ('Bl3ack', 'Black', false),  -- 1, e.g. 1 Bl3ack, 381
    ('Bl3atchf.', 'Blatchf.', false),  -- 1, e.g. 10 Bl3atchf. 271
    ('Bla3tchf.', 'Blatchf.', false),  -- 1, e.g. 3 Bla3tchf. 148
    ('Bu1sh', 'Bush', false),  -- 1, e.g. 9 Bu1sh, 565
    ('C1ush.', 'Cush.', false),  -- 1, e.g. 5 C1ush. 488
    ('C7onn.', 'Conn.', false),  -- 1, e.g. 7 C7onn. 5
    ('Ca0l.', 'Cal.', false),  -- 1, e.g. 36 Ca0l. 115
    ('Cal. Ap1p.', 'Cal. App.', false),  -- 1, e.g. 12 Cal. Ap1p. 521
    ('Co0nn.', 'Conn.', false),  -- 1, e.g. 46 Co0nn. 42
    ('Colo. Ap1p.', 'Colo. App.', false),  -- 1, e.g. 13 Colo. Ap1p. 393
    ('Con1n.', 'Conn.', false),  -- 1, e.g. 41 Con1n. 55
    ('Cow7en', 'Cow.', false),  -- 1, e.g. 3 Cow7en, 5
    ('Cran1ch', 'Cranch', false),  -- 1, e.g. 7 Cran1ch, 116
    ('D1ev. Eq.', 'Dev. Eq.', false),  -- 1, e.g. 1 D1ev. Eq. 1
    ('D3ay', 'Day', false),  -- 1, e.g. 3 D3ay, 90
    ('Dan1a', 'Dana', false),  -- 1, e.g. 5 Dan1a 359
    ('E2ast', 'East', false),  -- 1, e.g. 15 E2ast, 474
    ('E4dw. Ch.', 'Edw. Ch.', false),  -- 1, e.g. 1 E4dw. Ch., 451
    ('F1lip.', 'Flip.', false),  -- 1, e.g. 2 F1lip. 88
    ('F1oster', 'Foster', false),  -- 1, e.g. 1 F1oster, 011
    ('G0ray', 'Gray', false),  -- 1, e.g. 5 G0ray, 97
    ('G1ilm.', 'Gilm.', false),  -- 1, e.g. 1 G1ilm., 600
    ('Ga. Ap1p.', 'Ga. App.', false),  -- 1, e.g. 20 Ga. Ap1p. 648
    ('Gr0ay', 'Gray', false),  -- 1, e.g. 7 Gr0ay, 55
    ('Gran1t', 'Grant', false),  -- 1, e.g. 1 Gran1t, 217
    ('H0ow.', 'How.', false),  -- 1, e.g. 8 H0ow. 207
    ('H1alst.', 'Halst.', false),  -- 1, e.g. 5 H1alst. 168
    ('H1ill Eq.', 'Hill Eq.', false),  -- 1, e.g. 2 H1ill Eq. 71
    ('H1oward', 'Howard', false),  -- 1, e.g. 5 H1oward, 1
    ('H3un', 'Hun', false),  -- 1, e.g. 61 H3un, 633
    ('H7aw.', 'Haw.', false),  -- 1, e.g. 1 H7aw. 110
    ('H7ow.', 'How.', false),  -- 1, e.g. 5 H7ow. 665
    ('Ha1ll', 'Hall', false),  -- 1, e.g. 2 Ha1ll, 1
    ('Hu1n', 'Hun', false),  -- 1, e.g. 25 Hu1n, 341
    ('I3nd. App.', 'Ind. App.', false),  -- 1, e.g. 19 I3nd. App. 383
    ('Io7wa', 'Iowa', false),  -- 1, e.g. 5 Io7wa 2211
    ('Iow0a', 'Iowa', false),  -- 1, e.g. 11 Iow0a, 14
    ('Ir. Ju1r.', 'Ir. Jur.', false),  -- 1, e.g. 4 Ir. Ju1r. 0
    ('J1ohns.', 'Johns.', false),  -- 1, e.g. 10 J1ohns. 3
    ('J3ur.', 'Jur.', false),  -- 1, e.g. 3 J3ur. 314
    ('Jo1hns.', 'Johns.', false),  -- 1, e.g. 14 Jo1hns. 325
    ('Joh1ns.', 'Johns.', false),  -- 1, e.g. 1 Joh1ns. 498
    ('Johns. C1h.', 'Johns. Ch.', false),  -- 1, e.g. 7 Johns. C1h. 25
    ('K0an.', 'Kan.', false),  -- 1, e.g. 0 K0an. 41
    ('L. J. B1ank.', 'L. J. Bank.', false),  -- 1, e.g. 45 L. J. B1ank.    100
    ('L1a.', 'La.', false),  -- 1, e.g. 135 L1a., 19
    ('L4a.', 'La.', false),  -- 1, e.g. 1 L4a. 5
    ('M0ass.', 'Mass.', false),  -- 1, e.g. 118 M0ass. 277
    ('M1acph.', 'Macph.', false),  -- 1, e.g. 3 M1acph. 1
    ('M1cCrary', 'McCrary', false),  -- 1, e.g. 2    M1cCrary, 48
    ('M1eg.', 'Meg.', false),  -- 1, e.g. 1 M1eg. 92
    ('M1et.', 'Met.', false),  -- 1, e.g. 5 M1et. 452
    ('M1ol.', 'Mol.', false),  -- 1, e.g. 2 M1ol. 545
    ('M1ont. & A.', 'Mont. & A.', false),  -- 1, e.g. 2 M1ont. & A. 5
    ('M3artin', 'Martin', false),  -- 1, e.g. 7 M3artin, 464
    ('M3cLean', 'McLean', false),  -- 1, e.g. 4 M3cLean, 531
    ('M3ill', 'Mill', false),  -- 1, e.g. 13 M3ill 5312
    ('M3unf.', 'Munf.', false),  -- 1, e.g. 0 M3unf. 60
    ('M4o.', 'Mo.', false),  -- 1, e.g. 44 M4o. 843
    ('M5ich.', 'Mich.', false),  -- 1, e.g. 35 M5ich. 434
    ('M5iss.', 'Miss.', false),  -- 1, e.g. 43 M5iss. 3
    ('M5or.', 'Mor.', false),  -- 1, e.g. 7 M5or. 246
    ('M6o.', 'Mo.', false),  -- 1, e.g. 11 M6o. 870
    ('M7ich.', 'Mich.', false),  -- 1, e.g. 45 M7ich. 313
    ('M8ass.', 'Mass.', false),  -- 1, e.g. 98 M8ass. 101
    ('M8o.', 'Mo.', false),  -- 1, e.g. 213 M8o. 66
    ('Ma0ss.', 'Mass.', false),  -- 1, e.g. 1 Ma0ss. 261
    ('Mas4s.', 'Mass.', false),  -- 1, e.g. 107 Mas4s. 331
    ('Mi1nn.', 'Minn.', false),  -- 1, e.g. 2 Mi1nn. 159
    ('Mi2ch.', 'Mich.', false),  -- 1, e.g. 27 Mi2ch. 15
    ('N1eb.', 'Neb.', false),  -- 1, e.g. 1 N1eb. 109
    ('N7eb.', 'Neb.', false),  -- 1, e.g. 25 N7eb. 207
    ('Nott & M1cC.', 'Nott & McC.', false),  -- 1, e.g. 2 Nott & M1cC., 267
    ('Nott & M3cC.', 'Nott & McC.', false),  -- 1, e.g. 2 Nott & M3cC. 243
    ('O0hio', 'Ohio', false),  -- 1, e.g. 5 O0hio 5
    ('O0hio St.', 'Ohio St.', false),  -- 1, e.g. 21 O0hio St. 107
    ('Oh7io St.', 'Ohio St.', false),  -- 1, e.g. 67 Oh7io St. 157
    ('Pe1t.', 'Pet.', false),  -- 1, e.g. 11 Pe1t. 351
    ('R1and.', 'Rand.', false),  -- 1, e.g. 6 R1and. 618
    ('R1ob.', 'Rob.', false),  -- 1, e.g. 2 R1ob. 52
    ('S1o.', 'So.', false),  -- 1, e.g. 90 S1o. 475
    ('S3o.', 'So.', false),  -- 1, e.g. 70 S3o.    202
    ('Te1nn.', 'Tenn.', false),  -- 1, e.g. 87 Te1nn. 575
    ('Tyr1w.', 'Tyrw.', false),  -- 1, e.g. 5 Tyr1w. 522
    ('U1tah', 'Utah', false),  -- 1, e.g. 9    U1tah 340
    ('V2a.', 'Va.', false),  -- 1, e.g. 125 V2a. 5
    ('Ve2rn', 'Vern', false),  -- 1, e.g. 2 Ve2rn, 665
    ('W0in.', 'Win.', false),  -- 1, e.g. 118 W0in. 18
    ('W1end.', 'Wend.', false),  -- 1, e.g. 16 W1end. 631
    ('W1in.', 'Win.', false),  -- 1, e.g. 2 W1in. 4
    ('W5ash.', 'Wash.', false),  -- 1, e.g. 5 W5ash. 152
    ('W7end.', 'Wend.', false),  -- 1, e.g. 3 W7end. 376
    ('W7heat.', 'Wheat.', false),  -- 1, e.g. 6 W7heat., 542
    ('W7is.', 'Wis.', false),  -- 1, e.g. 60 W7is. 233
    ('Wa3ll.', 'Wall.', false),  -- 1, e.g. 21 Wa3ll. 155
    ('Wa5tts & Serg.', 'Watts & Serg.', false),  -- 1, e.g. 7 Wa5tts & Serg. 89
    ('Wi1l.', 'Wil.', false),  -- 1, e.g. 2 Wi1l. 137
    ('Wi2l.', 'Wil.', false),  -- 1, e.g. 1 Wi2l. 4
    ('Woo1ds.', 'Woods.', false),  -- 1, e.g. 1 Woo1ds. 60
    ('Z1ab.', 'Zab.', false)  -- 1, e.g. 2 Z1ab. 37
ON CONFLICT (reporter_found) DO NOTHING;

-- migrate:down
SET ROLE = law_admin;

DELETE FROM legalhist.whitelist WHERE reporter_found IN (
    'M1o.',
    'M3o.',
    'M1ass.',
    'M1o. App.',
    'M1e.',
    'M3ass.',
    'M3e.',
    'H1ow.',
    'M5o.',
    'M1ich.',
    'M3ich.',
    'M3o. App.',
    'In1d.',
    'P1a.',
    'B1arb.',
    'H1ill',
    'M1inn.',
    'I1nd.',
    'Ma1ss.',
    'B3arb.',
    'M3inn.',
    'K1an.',
    'M0o.',
    'M3iss.',
    'H1ow. Pr.',
    'L. E1d.',
    'M1iss.',
    'M3ans.',
    'H1un',
    'Am. Re1p.',
    'H1ilt.',
    'K1y.',
    'Ma8ss.',
    'Am. R1ep.',
    'Am. St. Re1p.',
    'Ka1n.',
    'M1isc.',
    'M1ont.',
    'M3et.',
    'M5ass.',
    'M5e.',
    'M5inn.',
    'Oh1io St.',
    'W1ash.',
    'W1is.',
    'Ab1b.',
    'Am. St. R1ep.',
    'B1ibb',
    'Ba3rb.',
    'Bar1b.',
    'C1al.',
    'Co1nn.',
    'F1la.',
    'H1are',
    'H2ow.',
    'How. P1r.',
    'I1nd. App.',
    'I9nd.',
    'Ind. Ap1p.',
    'John1s.',
    'M. & R1y.',
    'M1ason',
    'M1oo. & P.',
    'M3acph.',
    'M3isc.',
    'M3ont.',
    'M5et.',
    'M5o. App.',
    'Ma3ss.',
    'Ma5ss.',
    'Mo. Ap1p.',
    'P1et.',
    'P1ick.',
    'T1ex.',
    'W1all.',
    'Wa1sh.',
    'A1m. St. Rep.',
    'A4bb.',
    'A4la.',
    'A4m. Dec.',
    'A9m. Dec.',
    'All1en',
    'Alle1n',
    'Am. D1ec.',
    'Am. S8t. Rep.',
    'B1all & B.',
    'B1arr',
    'B1en.',
    'B1inn.',
    'B1radf.',
    'B1ur.',
    'B1ush',
    'B3eav',
    'B3iss.',
    'B3radf.',
    'B7arb.',
    'Ba1il.',
    'Bl3ack',
    'Bl3atchf.',
    'Bla3tchf.',
    'Bu1sh',
    'C1ush.',
    'C7onn.',
    'Ca0l.',
    'Cal. Ap1p.',
    'Co0nn.',
    'Colo. Ap1p.',
    'Con1n.',
    'Cow7en',
    'Cran1ch',
    'D1ev. Eq.',
    'D3ay',
    'Dan1a',
    'E2ast',
    'E4dw. Ch.',
    'F1lip.',
    'F1oster',
    'G0ray',
    'G1ilm.',
    'Ga. Ap1p.',
    'Gr0ay',
    'Gran1t',
    'H0ow.',
    'H1alst.',
    'H1ill Eq.',
    'H1oward',
    'H3un',
    'H7aw.',
    'H7ow.',
    'Ha1ll',
    'Hu1n',
    'I3nd. App.',
    'Io7wa',
    'Iow0a',
    'Ir. Ju1r.',
    'J1ohns.',
    'J3ur.',
    'Jo1hns.',
    'Joh1ns.',
    'Johns. C1h.',
    'K0an.',
    'L. J. B1ank.',
    'L1a.',
    'L4a.',
    'M0ass.',
    'M1acph.',
    'M1cCrary',
    'M1eg.',
    'M1et.',
    'M1ol.',
    'M1ont. & A.',
    'M3artin',
    'M3cLean',
    'M3ill',
    'M3unf.',
    'M4o.',
    'M5ich.',
    'M5iss.',
    'M5or.',
    'M6o.',
    'M7ich.',
    'M8ass.',
    'M8o.',
    'Ma0ss.',
    'Mas4s.',
    'Mi1nn.',
    'Mi2ch.',
    'N1eb.',
    'N7eb.',
    'Nott & M1cC.',
    'Nott & M3cC.',
    'O0hio',
    'O0hio St.',
    'Oh7io St.',
    'Pe1t.',
    'R1and.',
    'R1ob.',
    'S1o.',
    'S3o.',
    'Te1nn.',
    'Tyr1w.',
    'U1tah',
    'V2a.',
    'Ve2rn',
    'W0in.',
    'W1end.',
    'W1in.',
    'W5ash.',
    'W7end.',
    'W7heat.',
    'W7is.',
    'Wa3ll.',
    'Wa5tts & Serg.',
    'Wi1l.',
    'Wi2l.',
    'Woo1ds.',
    'Z1ab.'
);
