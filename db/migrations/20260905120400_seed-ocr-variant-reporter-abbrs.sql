-- migrate:up
SET ROLE = law_admin;

-- Seed legalhist.whitelist with reporter abbreviations the OCR corrupted by
-- confusing one letter for another ("Vill." for "Will.", "Fcd." for "Fed.",
-- "Cornst." for "Comst.") or by reading a letter as a digit ("I1l." for
-- "Ill."). Every detector records the spelling as it appeared, so these
-- citations sit at skipped_not_whitelisted until the whitelist carries the
-- spelling (issue #247, part of #165).
--
-- Candidates were compiled by TestOCRVariantWhitelistSuggestions from
-- moml_citations.citations_unlinked, spellings not in the whitelist with at least 20 rows, 2026-09-05,
-- and are recorded in full, including the ones rejected here, in
-- db/whitelist-candidates-ocr-variants.tsv.
--
-- A candidate is one non-whitelisted spelling with a reading that is already
-- an exact whitelist entry; readings come from stripping a letter-flanked
-- digit or from one substitution in the OCR confusion table of
-- go/citations/ocrvariant.go. Nothing is normalized. Two filters were then
-- applied to the 2035 proposals in that file:
--
--   1. Canonical only. The corrected spelling must itself be a reporter in
--      legalhist.reporters, not another whitelist variant that maps onward,
--      which would inherit that entry's looseness. 1735 proposals dropped.
--
--   2. Unambiguous only. The harness compares every reporter a spelling's
--      readings reach by the corpus frequency of the spellings that reach
--      them, and sets aside any spelling whose runner-up is at least 10%% as
--      frequent as its winner; 18 such spellings are in the file's AMBIGUOUS
--      section and none is seeded. A reading reached only by taking a digit
--      for an arbitrary letter (rule digit_letter) is the check's evidence,
--      not a proposal: 6 canonical proposals of that kind were dropped.
--
--   3. Long enough. A letter-for-letter reading is seeded only when the
--      corrected spelling has at least 5 letters, because on a shorter
--      abbreviation the reading is as plausible a word as the scanned one
--      ("Out." is Outerbridge, not Ontario; "Gal." is as likely Gallison as
--      California). Digit readings are exempt: a digit inside an abbreviation
--      is always an OCR error. 145 canonical proposals were set aside for
--      review this way, 20,670 rows in all:
--        "Belt"         -> Bell           (Bell)         1402 rows
--        "Out."         -> Ont.           (Ont.)         1075 rows
--        "Se."          -> Sc.            (Sc.)           864 rows
--        "Gal."         -> Cal.           (Cal.)          853 rows
--        "Ducr"         -> Duer           (Duer)          651 rows
--        "Mot."         -> Mol.           (Mol.)          548 rows
--        "Cat."         -> Cal.           (Cal.)          478 rows
--        "Litl."        -> Litt.          (Litt.)         465 rows
--        "AId."         -> Ald.           (Ald.)          442 rows
--        "Lilt."        -> Litt.          (Litt.)         421 rows
--        "L. G."        -> L. C.          (L. C.)         420 rows
--        "Gold."        -> Cold.          (Cold.)         415 rows
--        "Vall."        -> Wall.          (Wall.)         390 rows
--        "Scld."        -> Seld.          (Seld.)         370 rows
--        "Hnn"          -> Hun            (Hun)           318 rows
--        "Ncv. & M."    -> Nev. & M.      (Nev. & M.)     314 rows
--        "Weud."        -> Wend.          (Wend.)         306 rows
--        "Wit."         -> Wil.           (Wil.)          300 rows
--        "Cray"         -> Gray           (Gray)          298 rows
--        "Dner"         -> Duer           (Duer)          283 rows
--        "Easl"         -> East           (East)          283 rows
--        "Mel."         -> Met.           (Met.)          273 rows
--        "Itl."         -> Ill.           (Ill.)          267 rows
--        "lll."         -> Ill.           (Ill.)          226 rows
--        "Vl."          -> Vt.            (Vt.)           223 rows
--        "Ircd."        -> Ired.          (Ired.)         210 rows
--        "Hul"          -> Hut            (Hut)           203 rows
--        "Huu"          -> Hun            (Hun)           201 rows
--        "Tenu."        -> Tenn.          (Tenn.)         194 rows
--        "Hilt"         -> Hill           (Hill)          189 rows
--        "Maus."        -> Mans.          (Mans.)         186 rows
--        "Maeq."        -> Macq.          (Macq.)         178 rows
--        "Incl."        -> Ind.           (Ind.)          171 rows
--        "Gall"         -> Call           (Call)          163 rows
--        "Harc"         -> Hare           (Hare)          161 rows
--        "Biun."        -> Binn.          (Binn.)         159 rows
--        "Mnnf."        -> Munf.          (Munf.)         153 rows
--        "Binu."        -> Binn.          (Binn.)         150 rows
--        "Watk."        -> Walker         (Walk.)         148 rows
--        "Ilt."         -> Ill.           (Ill.)          145 rows
--        "Gai."         -> Cai.           (Cai.)          136 rows
--        "Ncv. & P."    -> Nev. & P.      (Nev. & P.)     136 rows
--        "N.V."         -> N.W.           (N.W.)          132 rows
--        "Laus."        -> Lans.          (Lans.)         128 rows
--        "At"           -> Al             (Al)            124 rows
--        "Iova"         -> Iowa           (Iowa)          122 rows
--        "Lntw."        -> Lutw.          (Lutw.)         122 rows
--        "Mout."        -> Mont.          (Mont.)         118 rows
--        "Dovl."        -> Dow PC         (Dowl.)         116 rows
--        "V.R."         -> W.R.           (W.R.)          115 rows
--        "DowI."        -> Dow PC         (Dowl.)         105 rows
--        "L. C. R."     -> L. G. R.       (L. G. R.)      104 rows
--        "Bnsh"         -> Bush           (Bush)          102 rows
--        "Pel."         -> Pet.           (Pet.)          101 rows
--        "Vash."        -> Wash.          (Wash.)          99 rows
--        "Dcac."        -> Deac.          (Deac.)          96 rows
--        "MoI."         -> Mol.           (Mol.)           96 rows
--        "Pct."         -> Pet.           (Pet.)           96 rows
--        "Brcv."        -> Brev.          (Brev.)          95 rows
--        "Tyrv."        -> Tyrw.          (Tyrw.)          94 rows
--        "Mnr."         -> Mur.           (Mur.)           91 rows
--        "Wa."          -> Va.            (Va.)            88 rows
--        "AIa."         -> Ala.           (Ala.)           87 rows
--        "Dca. & C."    -> Dea. & C.      (Dea. & C.)      87 rows
--        "Snmn."        -> Sumn.          (Sumn.)          85 rows
--        "Mcl."         -> Md.            (Md.)            84 rows
--        "Conii."       -> Conn.          (Conn.)          83 rows
--        "Monl."        -> Mont.          (Mont.)          81 rows
--        "Git."         -> Gil.           (Gil.)           80 rows
--        "New."         -> Nev.           (Nev.)           80 rows
--        "V. L. B."     -> W. L. B.       (W. L. B.)       79 rows
--        "Bcn."         -> Ben.           (Ben.)           76 rows
--        "Muuf."        -> Munf.          (Munf.)          74 rows
--        "Vil."         -> Wil.           (Wil.)           72 rows
--        "V. C. C."     -> W. C. C.       (W. C. C.)       69 rows
--        "Deae."        -> Deac.          (Deac.)          67 rows
--        "H. L. Se."    -> H. L. Sc.      (H. L. Sc.)      67 rows
--        "lrv."         -> Irv.           (Irv.)           67 rows
--        "Carl."        -> Cart.          (Cart.)          65 rows
--        "lred."        -> Ired.          (Ired.)          64 rows
--        "Bnr."         -> Bur.           (Bur.)           62 rows
--        "Tenii."       -> Tenn.          (Tenn.)          62 rows
--        "Daua"         -> Dana           (Dana)           58 rows
--        "DalI."        -> Dall.          (Dall.)          57 rows
--        "Wt."          -> Vt.            (Vt.)            56 rows
--        "Brew."        -> Brev.          (Brev.)          55 rows
--        "S. Cl."       -> S. Ct.         (S. Ct.)         55 rows
--        "Coiin."       -> Conn.          (Conn.)          53 rows
--        "Cov."         -> Cow.           (Cow.)           53 rows
--        "Curl."        -> Curt.          (Curt.)          53 rows
--        "Cnrt."        -> Curt.          (Curt.)          51 rows
--        "Eug."         -> Eng.           (Eng.)           51 rows
--        "Vcrn"         -> Vern           (Vern)           50 rows
--        "Sumu."        -> Sumn.          (Sumn.)          49 rows
--        "Ycr."         -> Yer.           (Yer.)           47 rows
--        "Dalt."        -> Dall.          (Dall.)          46 rows
--        "Dnv."         -> Duv.           (Duv.)           46 rows
--        "Atd."         -> Ald.           (Ald.)           45 rows
--        "Ulah"         -> Utah           (Utah)           44 rows
--        "Lutv."        -> Lutw.          (Lutw.)          43 rows
--        "Vest"         -> West           (West)           41 rows
--        "CaI."         -> Cal.           (Cal.)           40 rows
--        "Det."         -> Del.           (Del.)           40 rows
--        "Dowt."        -> Dow PC         (Dowl.)          40 rows
--        "Dew."         -> Dev.           (Dev.)           39 rows
--        "Bcav"         -> Beav           (Beav)           38 rows
--        "Savy."        -> Sawy.          (Sawy.)          38 rows
--        "Halt"         -> Hall           (Hall)           37 rows
--        "Iiid."        -> Ind.           (Ind.)           37 rows
--        "Ata."         -> Ala.           (Ala.)           36 rows
--        "Dcl."         -> Del.           (Del.)           36 rows
--        "Chil."        -> Chit.          (Chit.)          35 rows
--        "Hov."         -> How.           (How.)           35 rows
--        "Lnd."         -> Lud.           (Lud.)           35 rows
--        "Lulw."        -> Lutw.          (Lutw.)          35 rows
--        "Robl."        -> Robt.          (Robt.)          35 rows
--        "Cill"         -> Gill           (Gill)           34 rows
--        "Slew."        -> Stewart        (Stew.)          34 rows
--        "Beu."         -> Ben.           (Ben.)           33 rows
--        "Vyo."         -> Wyo.           (Wyo.)           33 rows
--        "Miiin."       -> Minn.          (Minn.)          31 rows
--        "Onl."         -> Ont.           (Ont.)           31 rows
--        "Lca"          -> Lea            (Lea)            30 rows
--        "WaIl."        -> Wall.          (Wall.)          30 rows
--        "Oliio"        -> Ohio           (Ohio)           29 rows
--        "Bosv."        -> Bosw.          (Bosw.)          28 rows
--        "Conp."        -> Coup.          (Coup.)          28 rows
--        "Kaii."        -> Kan.           (Kan.)           28 rows
--        "Lav J."       -> Law J.         (Law J.)         28 rows
--        "Raud."        -> Rand.          (Rand.)          26 rows
--        "V. P. C."     -> W. P. C.       (W. P. C.)       26 rows
--        "AI"           -> Al             (Al)             25 rows
--        "Sl. Tr."      -> St. Tr.        (St. Tr.)        25 rows
--        "Bait."        -> Bail.          (Bail.)          23 rows
--        "Peek"         -> Peck           (Peck)           23 rows
--        "Walt."        -> Wall.          (Wall.)          23 rows
--        "Marl."        -> Mart.          (Mart.)          22 rows
--        "Swiu."        -> Swin.          (Swin.)          22 rows
--        "Yet"          -> Yel            (Yel)            22 rows
--        "Bcas."        -> Beas.          (Beas.)          21 rows
--        "Sliep."       -> Shep.          (Shep.)          21 rows
--        "Vem"          -> Vern           (Vern)           21 rows
--        "Wiu."         -> Win.           (Win.)           21 rows
--        "S.V."         -> S.W.           (S.W.)           20 rows
--        "Saw"          -> Sav            (Sav)            20 rows
--
-- 149 rows remain, by rule:
--   confusion    141 spellings     16,979 rows
--   digit          4 spellings        110 rows
--   digit_sub      4 spellings      2,099 rows
-- The file also lists 614 spellings whose every reading is junk (TOJUNK) and
-- 11122 with no whitelisted reading (UNRESOLVED); neither is touched here.
-- Counts below are rows in moml_citations.citations_unlinked.

INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk) VALUES
    ('I1l.', 'Ill.', false),  -- 1803, digit_sub, e.g. 00 I1l.    2
    ('Dcnio', 'Denio', false),  -- 1678, confusion, e.g. 1 Dcnio, 002
    ('Cowcn', 'Cow.', false),  -- 1088, confusion, e.g. 0 Cowcn, 1
    ('Gowen', 'Cow.', false),  -- 823, confusion, e.g. 1 Gowen 7
    ('McLcan', 'McLean', false),  -- 536, confusion, e.g. 0 McLcan, 1
    ('Rawlc', 'Rawle', false),  -- 512, confusion, e.g. 10 Rawlc, 179
    ('MeCrary', 'McCrary', false),  -- 456, confusion, e.g. 11 MeCrary, 177
    ('Crancli', 'Cranch', false),  -- 424, confusion, e.g. 0 Crancli, 130
    ('Snecd', 'Sneed', false),  -- 422, confusion, e.g. 1 Snecd, 010
    ('Craneh', 'Cranch', false),  -- 414, confusion, e.g. 0 Craneh, 221
    ('Lcach', 'Leach', false),  -- 372, confusion, e.g. 1 Lcach, 0
    ('Yeatcs', 'Yeates', false),  -- 353, confusion, e.g. 1 Yeatcs, 1
    ('Snced', 'Sneed', false),  -- 332, confusion, e.g. 0 Snced, 482
    ('Crauch', 'Cranch', false),  -- 327, confusion, e.g. 0 Crauch 102
    ('Alleu', 'Allen', false),  -- 312, confusion, e.g. 10 Alleu, 106
    ('Wcntw.', 'Wentw.', false),  -- 292, confusion, e.g. 041 Wcntw. 1
    ('Am. Elee. Cas.', 'Am. Elec. Cas.', false),  -- 286, confusion, e.g. 0 Am. Elee. Cas. 079
    ('Macpli.', 'Macph.', false),  -- 281, confusion, e.g. 0 Macpli. 833
    ('Lcigh', 'Leigh', false),  -- 273, confusion, e.g. 0 Lcigh 271
    ('Hcisk.', 'Heisk.', false),  -- 271, confusion, e.g. 0 Hcisk. 1
    ('I1l. App.', 'Ill. App.', false),  -- 248, digit_sub, e.g. 03 I1l. App. 76
    ('MeCord', 'McCord', false),  -- 235, confusion, e.g. 1 MeCord, 115
    ('Deuio', 'Denio', false),  -- 199, confusion, e.g. 14 Deuio, 153
    ('Allcn', 'Allen', false),  -- 194, confusion, e.g. 0 Allcn,    2
    ('Int. Rev. Ree.', 'Int. Rev. Rec.', false),  -- 193, confusion, e.g. 10 Int. Rev. Ree. 1017
    ('Maeph.', 'Macph.', false),  -- 190, confusion, e.g. 0 Maeph. 233
    ('Int. Rev. Rcc.', 'Int. Rev. Rec.', false),  -- 186, confusion, e.g. 10 Int. Rev. Rcc. 39
    ('Fostcr', 'Foster', false),  -- 181, confusion, e.g. 2 Fostcr, 10
    ('Cratt.', 'Gratt.', false),  -- 179, confusion, e.g. 0 Cratt. 6
    ('MeLean', 'McLean', false),  -- 178, confusion, e.g. 0 MeLean, 558
    ('AlIen', 'Allen', false),  -- 162, confusion, e.g. 10 AlIen, 350
    ('Out. App.', 'Ont. App.', false),  -- 155, confusion, e.g. 10 Out. App. 19
    ('Burrcll', 'Burrell', false),  -- 153, confusion, e.g. 13    Burrcll, 255
    ('Walts', 'Watts', false),  -- 146, confusion, e.g. 10 Walts, 1
    ('Sonth.', 'South.', false),  -- 123, confusion, e.g. 10 Sonth. 110
    ('Pricc', 'Price', false),  -- 114, confusion, e.g. 0 Pricc, 2
    ('Ircd. Eq.', 'Ired. Eq.', false),  -- 108, confusion, e.g. 1 Ircd. Eq. 1
    ('Idalio', 'Idaho', false),  -- 107, confusion, e.g. 10 Idalio, 263
    ('Se. App.', 'Sc. App.', false),  -- 102, confusion, e.g. 1 Se. App. 1
    ('Coweu', 'Cow.', false),  -- 95, confusion, e.g. 0 Coweu, 399
    ('Pcake', 'Peake', false),  -- 95, confusion, e.g. 011 Pcake, 30
    ('Austr.', 'Anstr.', false),  -- 93, confusion, e.g. 1 Austr. 138
    ('Alten', 'Allen', false),  -- 89, confusion, e.g. 11 Alten 365
    ('Drn. & War.', 'Dru. & War.', false),  -- 88, confusion, e.g. 1 Drn. & War. 120
    ('Coven', 'Cow.', false),  -- 87, confusion, e.g. 0 Coven, 437
    ('New Scss. Cas.', 'New Sess. Cas.', false),  -- 87, confusion, e.g. 1 New Scss. Cas. 171
    ('Se. Jur.', 'Sc. Jur.', false),  -- 86, confusion, e.g. 12 Se. Jur. 272
    ('McLeau', 'McLean', false),  -- 85, confusion, e.g. 1 McLeau, 174
    ('Scolt', 'Scott', false),  -- 85, confusion, e.g. 1 Scolt, 1
    ('Dcl. Ch.', 'Del. Ch.', false),  -- 82, confusion, e.g. 0 Dcl. Ch. 163
    ('Sand. Cli.', 'Sand. Ch.', false),  -- 79, confusion, e.g. 1 Sand. Cli. 1
    ('Greeul.', 'Green.', false),  -- 78, confusion, e.g. 0 Greeul. 155
    ('Cales', 'Cates', false),  -- 76, confusion, e.g. 1 Cales, 1
    ('Sueed', 'Sneed', false),  -- 75, confusion, e.g. 1 Sueed, 07
    ('Leaeh', 'Leach', false),  -- 74, confusion, e.g. 1 Leaeh, 1
    ('Vatts', 'Watts', false),  -- 71, confusion, e.g. 10 Vatts 118
    ('Paige Cli.', 'Paige Ch.', false),  -- 69, confusion, e.g. 0 Paige Cli. 20
    ('L. J. Cli. N. S.', 'L. J. Ch. N. S.', false),  -- 68, confusion, e.g. 00 L. J. Cli. N. S. 590
    ('Yeales', 'Yeates', false),  -- 68, confusion, e.g. 13 Yeales, 315
    ('Watls', 'Watts', false),  -- 67, confusion, e.g. 0 Watls, 1
    ('Soutli.', 'South.', false),  -- 66, confusion, e.g. 00 Soutli.    133
    ('Iud. App.', 'Ind. App.', false),  -- 65, confusion, e.g. 10 Iud. App. 47
    ('Painc', 'Paine', false),  -- 64, confusion, e.g. 158 Painc, 284
    ('Oliio St.', 'Ohio St.', false),  -- 62, confusion, e.g. 10 Oliio St. 488
    ('Wlieat.', 'Wheat.', false),  -- 62, confusion, e.g. 0 Wlieat. 693
    ('Johus. Ch.', 'Johns. Ch.', false),  -- 60, confusion, e.g. 0 Johus. Ch. 1
    ('Peakc', 'Peake', false),  -- 60, confusion, e.g. 1 Peakc, 01
    ('Gratl.', 'Gratt.', false),  -- 59, confusion, e.g. 10 Gratl. 284
    ('Boyee', 'Boyce', false),  -- 56, confusion, e.g. 0 Boyee 13
    ('MeAll.', 'McAll.', false),  -- 53, confusion, e.g. 1 MeAll. 08
    ('Va. Gas.', 'Va. Cas.', false),  -- 53, confusion, e.g. 1 Va. Gas. 1
    ('Leacli', 'Leach', false),  -- 51, confusion, e.g. 1 Leacli, 00
    ('Cascy', 'Casey', false),  -- 50, confusion, e.g. 0 Cascy, 489
    ('MeCart.', 'McCart.', false),  -- 50, confusion, e.g. 1 MeCart. 1
    ('Masou', 'Mason', false),  -- 47, confusion, e.g. 1 Masou, 176
    ('Jolins. Cas.', 'Johns. Cas.', false),  -- 45, confusion, e.g. 1 Jolins. Cas. 1
    ('Leg. lnt.', 'Leg. Int.', false),  -- 45, confusion, e.g. 10 Leg. lnt. 11
    ('L. J. Bauk.', 'L. J. Bank.', false),  -- 43, confusion, e.g. 25 L. J. Bauk. 41
    ('Ohio Cir. Dee.', 'Ohio Cir. Dec.', false),  -- 42, confusion, e.g. 0 Ohio Cir. Dee. 357
    ('Hughcs', 'Hughes', false),  -- 40, confusion, e.g. 1 Hughcs, 118
    ('Keycs', 'Keyes', false),  -- 40, confusion, e.g. 143    Keycs, 294
    ('Strangc', 'Strange', false),  -- 40, confusion, e.g. 10 Strangc, 637
    ('Anslr.', 'Anstr.', false),  -- 39, confusion, e.g. 1 Anslr. 109
    ('Joncs', 'Jones', false),  -- 39, confusion, e.g. 124 Joncs, 1
    ('Kcyes', 'Keyes', false),  -- 39, confusion, e.g. 1 Kcyes, 141
    ('Vheat.', 'Wheat.', false),  -- 39, confusion, e.g. 10 Vheat. 199
    ('Alleii', 'Allen', false),  -- 38, confusion, e.g. 12 Alleii, 345
    ('Curn. Cas.', 'Cum. Cas.', false),  -- 38, confusion, e.g. 11 Curn. Cas. 153
    ('Taytor', 'Taylor', false),  -- 38, confusion, e.g. Taytor, 1
    ('Am. Sl. Rep.', 'Am. St. Rep.', false),  -- 37, confusion, e.g. 0 Am. Sl. Rep. 889
    ('Crancli, C. C.', 'Cranch, C. C.', false),  -- 37, confusion, e.g. 1 Crancli, C. C. 1
    ('MeMul.', 'McMul.', false),  -- 37, confusion, e.g. 154 MeMul. 233
    ('Cornst.', 'Comst.', false),  -- 36, confusion, e.g. 1 Cornst. 101
    ('Seott', 'Scott', false),  -- 36, confusion, e.g. 1 Seott, 133
    ('Smilh', 'Smith Pa.', false),  -- 36, confusion, e.g. 1 Smilh, 147
    ('Honst.', 'Houst.', false),  -- 35, confusion, e.g. 1 Honst. 112
    ('Land Dee.', 'Land Dec.', false),  -- 35, confusion, e.g. 12 Land Dee.    5
    ('ldaho', 'Idaho', false),  -- 35, confusion, e.g. 11 ldaho 50
    ('I1ll.', 'Ill.', false),  -- 34, digit, e.g. 105 I1ll. 57
    ('Am. Ncg. Cas.', 'Am. Neg. Cas.', false),  -- 33, confusion, e.g. 10 Am. Ncg. Cas. 008
    ('Dct. Leg. N.', 'Det. Leg. N.', false),  -- 33, confusion, e.g. 0 Dct.    Leg. N. 320
    ('Ir. Jnr.', 'Ir. Jur.', false),  -- 33, confusion, e.g. 0 Ir. Jnr. 257
    ('Kclly', 'Kelly', false),  -- 33, confusion, e.g. 1 Kclly, 200
    ('Ir. Jnr. N. S.', 'Ir. Jur. N. S.', false),  -- 31, confusion, e.g. 0 Ir. Jnr. N. S. 127
    ('Priee', 'Price', false),  -- 31, confusion, e.g. 10 Priee, 138
    ('Ventw.', 'Wentw.', false),  -- 31, confusion, e.g. 10 Ventw. 158
    ('Anftr.', 'Anstr.', false),  -- 30, confusion, e.g. 2 Anftr. 25
    ('Brownc', 'Browne', false),  -- 30, confusion, e.g. 1    Brownc, 1
    ('Cliand.', 'Chand.', false),  -- 30, confusion, e.g. 1 Cliand. 207
    ('Rnss. & M.', 'Russ. & M.', false),  -- 30, confusion, e.g. 1 Rnss. & M. 236
    ('Arn. Rep.', 'Am. Rep.', false),  -- 29, confusion, e.g. 0 Arn. Rep. 193
    ('Gates', 'Cates', false),  -- 29, confusion, e.g. 021 Gates, 44
    ('Wheal.', 'Wheat.', false),  -- 29, confusion, e.g. 0 Wheal., 264
    ('A1a.', 'Ala.', false),  -- 28, digit_sub, e.g. 11 A1a. 01
    ('Fish. Pat. Gas.', 'Fish. Pat. Cas.', false),  -- 28, confusion, e.g. 1 Fish. Pat. Gas. 1
    ('M3or.', 'Mor.', false),  -- 28, digit, e.g. 10 M3or. 141
    ('lll. App.', 'Ill. App.', false),  -- 28, confusion, e.g. 100 lll. App. 22
    ('Granch', 'Cranch', false),  -- 27, confusion, e.g. 1 Granch 0
    ('Huglies', 'Hughes', false),  -- 27, confusion, e.g. 1 Huglies, 101
    ('Stcwart', 'Stewart', false),  -- 27, confusion, e.g. 1 Stcwart, 180
    ('Tannt', 'Taunt', false),  -- 27, confusion, e.g. 1 Tannt 2
    ('Craneh, C. C.', 'Cranch, C. C.', false),  -- 26, confusion, e.g. 1 Craneh, C. C. 552
    ('Creenl.', 'Green.', false),  -- 26, confusion, e.g. 1 Creenl. 219
    ('Leg. Iut.', 'Leg. Int.', false),  -- 26, confusion, e.g. 12 Leg. Iut. 4
    ('Paigc Ch.', 'Paige Ch.', false),  -- 26, confusion, e.g. 10 Paigc Ch. 288
    ('Blalchf.', 'Blatchf.', false),  -- 25, confusion, e.g. 13 Blalchf. 289
    ('Soulh.', 'South.', false),  -- 25, confusion, e.g. 03 Soulh. 774
    ('Ca1l.', 'Cal.', false),  -- 24, digit, e.g. 03 Ca1l., 74
    ('E1ast', 'East', false),  -- 24, digit, e.g. 10 E1ast, 10
    ('Grecne', 'Greene', false),  -- 24, confusion, e.g. 01 Grecne, 441
    ('Jolins', 'Johns.', false),  -- 24, confusion, e.g. 10 Jolins 120
    ('Joues', 'Jones', false),  -- 24, confusion, e.g. 1 Joues, 151
    ('Leigli', 'Leigh', false),  -- 24, confusion, e.g. 11 Leigli, 136
    ('Marlin', 'Martin', false),  -- 24, confusion, e.g. 10 Marlin 1
    ('lr. Jur. N. S.', 'Ir. Jur. N. S.', false),  -- 24, confusion, e.g. 10 lr. Jur. N. S. 3
    ('Bronn', 'Broun', false),  -- 23, confusion, e.g. 1 Bronn 134
    ('Conn. Comp. Dee.', 'Conn. Comp. Dec.', false),  -- 23, confusion, e.g. 1 Conn. Comp. Dee. 103
    ('Scotl', 'Scott', false),  -- 23, confusion, e.g. 1 Scotl, 245
    ('Wenlw.', 'Wentw.', false),  -- 23, confusion, e.g. 10 Wenlw. 152
    ('BIackf.', 'Blackf.', false),  -- 22, confusion, e.g. 1 BIackf. 10
    ('Crauch, C. C.', 'Cranch, C. C.', false),  -- 22, confusion, e.g. 17 Crauch, C. C. 517
    ('Wentv.', 'Wentw.', false),  -- 22, confusion, e.g. 10 Wentv. 251
    ('Weutw.', 'Wentw.', false),  -- 22, confusion, e.g. 10 Weutw. 157
    ('Wharl.', 'Whart.', false),  -- 22, confusion, e.g. 1 Wharl. 19
    ('Dong. Mich.', 'Doug. Mich.', false),  -- 21, confusion, e.g. 1 Dong. Mich. 1
    ('Greenc', 'Greene', false),  -- 21, confusion, e.g. 0 Greenc, 55
    ('Al1en', 'Allen', false),  -- 20, digit_sub, e.g. 12 Al1en, 362
    ('Arn. St. Rep.', 'Am. St. Rep.', false),  -- 20, confusion, e.g. 0 Arn. St. Rep. 2415
    ('Gum. Cas.', 'Cum. Cas.', false)  -- 20, confusion, e.g. 1 Gum. Cas. 164
ON CONFLICT (reporter_found) DO NOTHING;

-- migrate:down
SET ROLE = law_admin;

DELETE FROM legalhist.whitelist WHERE reporter_found IN (
    'I1l.',
    'Dcnio',
    'Cowcn',
    'Gowen',
    'McLcan',
    'Rawlc',
    'MeCrary',
    'Crancli',
    'Snecd',
    'Craneh',
    'Lcach',
    'Yeatcs',
    'Snced',
    'Crauch',
    'Alleu',
    'Wcntw.',
    'Am. Elee. Cas.',
    'Macpli.',
    'Lcigh',
    'Hcisk.',
    'I1l. App.',
    'MeCord',
    'Deuio',
    'Allcn',
    'Int. Rev. Ree.',
    'Maeph.',
    'Int. Rev. Rcc.',
    'Fostcr',
    'Cratt.',
    'MeLean',
    'AlIen',
    'Out. App.',
    'Burrcll',
    'Walts',
    'Sonth.',
    'Pricc',
    'Ircd. Eq.',
    'Idalio',
    'Se. App.',
    'Coweu',
    'Pcake',
    'Austr.',
    'Alten',
    'Drn. & War.',
    'Coven',
    'New Scss. Cas.',
    'Se. Jur.',
    'McLeau',
    'Scolt',
    'Dcl. Ch.',
    'Sand. Cli.',
    'Greeul.',
    'Cales',
    'Sueed',
    'Leaeh',
    'Vatts',
    'Paige Cli.',
    'L. J. Cli. N. S.',
    'Yeales',
    'Watls',
    'Soutli.',
    'Iud. App.',
    'Painc',
    'Oliio St.',
    'Wlieat.',
    'Johus. Ch.',
    'Peakc',
    'Gratl.',
    'Boyee',
    'MeAll.',
    'Va. Gas.',
    'Leacli',
    'Cascy',
    'MeCart.',
    'Masou',
    'Jolins. Cas.',
    'Leg. lnt.',
    'L. J. Bauk.',
    'Ohio Cir. Dee.',
    'Hughcs',
    'Keycs',
    'Strangc',
    'Anslr.',
    'Joncs',
    'Kcyes',
    'Vheat.',
    'Alleii',
    'Curn. Cas.',
    'Taytor',
    'Am. Sl. Rep.',
    'Crancli, C. C.',
    'MeMul.',
    'Cornst.',
    'Seott',
    'Smilh',
    'Honst.',
    'Land Dee.',
    'ldaho',
    'I1ll.',
    'Am. Ncg. Cas.',
    'Dct. Leg. N.',
    'Ir. Jnr.',
    'Kclly',
    'Ir. Jnr. N. S.',
    'Priee',
    'Ventw.',
    'Anftr.',
    'Brownc',
    'Cliand.',
    'Rnss. & M.',
    'Arn. Rep.',
    'Gates',
    'Wheal.',
    'A1a.',
    'Fish. Pat. Gas.',
    'M3or.',
    'lll. App.',
    'Granch',
    'Huglies',
    'Stcwart',
    'Tannt',
    'Craneh, C. C.',
    'Creenl.',
    'Leg. Iut.',
    'Paigc Ch.',
    'Blalchf.',
    'Soulh.',
    'Ca1l.',
    'E1ast',
    'Grecne',
    'Jolins',
    'Joues',
    'Leigli',
    'Marlin',
    'lr. Jur. N. S.',
    'Bronn',
    'Conn. Comp. Dee.',
    'Scotl',
    'Wenlw.',
    'BIackf.',
    'Crauch, C. C.',
    'Wentv.',
    'Weutw.',
    'Wharl.',
    'Dong. Mich.',
    'Greenc',
    'Al1en',
    'Arn. St. Rep.',
    'Gum. Cas.'
);
