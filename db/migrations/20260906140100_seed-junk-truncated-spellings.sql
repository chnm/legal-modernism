-- migrate:up
SET ROLE = law_admin;

-- Truncated whitelist spellings mint fake page numbers (issue #309, found
-- while sizing the stub table for #248).
--
-- The whitelist carries spellings that are truncations of longer spellings
-- of the same reporter: 'L. J. Q.' beside 'L. J. Q. B.', 'Am. B.' beside
-- 'Am. B. R.', 'Jur. (N.' beside 'Jur. (N. S.)', 'B. &' beside 'B. & C.'.
-- Each is there because the OCR sometimes drops the last token. Far more
-- often the OCR misreads that token as digits, and then the truncated
-- spelling matches with the digits as the page and the real page is lost:
-- "50 L. J. Q. B. 33" read as "50 L. J. Q. 1. 33" is detected as 50 L. J.
-- Q. 1, "22 Am. B. R. 320" read as "22 Am. B. 1:. 320" as 22 Am. B. 1, and
-- "7 Jur. (N. S.) 12" read as "7 Jur. (N. 8.) 12" as 7 Jur. (N. 8. The
-- signature is a spelling whose citations sit overwhelmingly on pages 1-15
-- while the full spellings of the same reporter sit there 4-12% of the
-- time; it was what put page 1 at the top of 90 volumes of A.B.R. and page
-- 8 at the top of 32 volumes of Jur. (n.s.) in the stub candidates.
--
-- Two parts. First, every whitelisted spelling that is a token-prefix of
-- another spelling of its reporter, has at least 100 rows in
-- citations_unlinked, and has at least 70% of them on pages 1-15 is marked
-- junk: what it matches is wrong far more often than right, and a
-- truncated citation carries no usable page either way. 69 spellings,
-- 131,781 rows, measured 2026-09-06. Bare 'Am.' is added below the cut
-- because a sample of its citations on high pages is American Reports and
-- American Decisions cites with the second token lost, so it is wrong in
-- every reading. Bare 'L. J.' (49,724 rows, 43%) and 'L. J. C.' (19,800
-- rows, 24%) are left alone: most of their citations are real, though a
-- sample of 'L. J. C.' on high pages is Law Journal Chancery every time,
-- which is a separate whitelist question. The surname single-volume stems
-- with high small-page rates (Wilson, Davis, Martin, Bell) are the #272
-- detector problem, not this one, and are not touched.
--
-- Second, OCR corrections for the dominant misreads, so that the citations
-- are recovered under the full spelling instead of discarded. The
-- corrections table is applied to the page text before detection as a
-- single-pass, longest-mistake-first literal replacement (go/sources/ocr.go),
-- so a rule fires only on its exact text: 'L. J. Q. 13.' never occurs in a
-- correctly read citation, because 'Q.' is never the last token of the
-- abbreviation. The percentages are the share of a random sample of each
-- spelling's citations that ends in that reading; the readings not listed
-- are a long tail no literal rule reaches, which is why the spellings are
-- junked as well. They take effect on the next detection run.

UPDATE legalhist.whitelist w
   SET junk = true,
       reporter_standard = NULL
  FROM (VALUES
    ('Am.', 'A.B.R.'),  -- 20,999 rows, 61% on pages <= 15; the rest are Am. Rep. and Am. Dec. cites, so wrong in every reading
    ('B. &', 'B & A'),  -- 13,676 rows, 85% on pages <= 15; truncates B. & A.
    ('L. J. Q.', 'L.J.K.B.'),  -- 11,405 rows, 97% on pages <= 15; truncates L. J. Q. B.
    ('M. &', 'M & S'),  -- 9,012 rows, 79% on pages <= 15; truncates M. & S.
    ('T. L.', 'T.L.R.'),  -- 7,100 rows, 93% on pages <= 15; truncates T. L. R.
    ('Jur. N.', 'Jur. (n.s.)'),  -- 4,442 rows, 96% on pages <= 15; truncates Jur. N. S.
    ('E. &', 'El & Bl'),  -- 4,350 rows, 94% on pages <= 15; truncates E. & B.
    ('L. J. M.', 'L.J.M.C.'),  -- 3,794 rows, 93% on pages <= 15; truncates L. J. M. C.
    ('S. &', 'Serg. & Rawl.'),  -- 3,080 rows, 92% on pages <= 15; truncates S. & R.
    ('Am. B.', 'A.B.R.'),  -- 2,919 rows, 80% on pages <= 15; truncates Am. B. R.
    ('D. &', 'Dowl. & Ry.'),  -- 2,750 rows, 86% on pages <= 15; truncates D. & R.
    ('L. J. K.', 'L.J.K.B.'),  -- 2,690 rows, 99% on pages <= 15; truncates L. J. K. B.
    ('Bing. N.', 'Bing NC'),  -- 2,557 rows, 90% on pages <= 15; truncates Bing. N. C.
    ('Car. &', 'Car & P'),  -- 2,447 rows, 90% on pages <= 15; truncates Car. & P.
    ('L. T. N.', 'L.T. (n.s.)'),  -- 2,351 rows, 88% on pages <= 15; truncates L. T. N. S.
    ('A. &', 'L.R.A.E.'),  -- 2,267 rows, 82% on pages <= 15; truncates A. & E.
    ('L. J., Q.', 'L.J.K.B.'),  -- 2,156 rows, 96% on pages <= 15; truncates L. J., Q. B.
    ('R. P.', 'R.P.C.'),  -- 2,115 rows, 94% on pages <= 15; truncates R. P. C.
    ('C. B. N.', 'CB NS'),  -- 2,083 rows, 94% on pages <= 15; truncates C. B. N. S.
    ('Serg.', 'Serg. & Rawl.'),  -- 2,029 rows, 92% on pages <= 15; truncates Serg. & R.
    ('Y. &', 'Y & C Ex'),  -- 1,884 rows, 90% on pages <= 15; truncates Y. & C.
    ('L. J., C.', 'L.J.C.P.'),  -- 1,733 rows, 71% on pages <= 15; truncates L. J., C. P.
    ('W. &', 'Watts & Serg.'),  -- 1,478 rows, 89% on pages <= 15; truncates W. & S.
    ('Bro. C.', 'Bro CC'),  -- 1,393 rows, 76% on pages <= 15; truncates Bro. C. C.
    ('J. &', 'J & H'),  -- 1,278 rows, 85% on pages <= 15; truncates J. & H.
    ('Times L.', 'T.L.R.'),  -- 1,268 rows, 97% on pages <= 15; truncates Times L. R.
    ('Ad. &', 'Ad & E'),  -- 1,234 rows, 72% on pages <= 15; truncates Ad. & E.
    ('Moo. P.', 'Moo PC'),  -- 1,205 rows, 96% on pages <= 15; truncates Moo. P. C.
    ('V. &', 'Ves & Bea'),  -- 1,090 rows, 94% on pages <= 15; truncates V. & B.
    ('Abb. N.', 'Abb. Pr. (n.s.)'),  -- 1,033 rows, 94% on pages <= 15; truncates Abb. N. S.
    ('C. B., N.', 'CB NS'),  -- 983 rows, 94% on pages <= 15; truncates C. B., N. S.
    ('Serg. &', 'Serg. & Rawl.'),  -- 936 rows, 92% on pages <= 15; truncates Serg. & R.
    ('Cox, C.', 'Cox'),  -- 903 rows, 94% on pages <= 15; truncates Cox, C. C.
    ('L. J., M.', 'L.J.M.C.'),  -- 887 rows, 97% on pages <= 15; truncates L. J., M. C.
    ('De G. &', 'De G & J'),  -- 748 rows, 77% on pages <= 15; truncates De G. & J.
    ('Wash. C.', 'Wash. C. C.'),  -- 588 rows, 84% on pages <= 15; truncates Wash. C. C.
    ('Wall. (U.', 'Wall.'),  -- 580 rows, 84% on pages <= 15; truncates Wall. (U. S.)
    ('Watts &', 'Watts & Serg.'),  -- 549 rows, 93% on pages <= 15; truncates Watts & S.
    ('Jur. (N.', 'Jur. (n.s.)'),  -- 538 rows, 95% on pages <= 15; truncates Jur. (N. S.)
    ('De G. M. &', 'De G M & G'),  -- 474 rows, 77% on pages <= 15; truncates De G. M. & G.
    ('How. (U.', 'How.'),  -- 461 rows, 85% on pages <= 15; truncates How. (U. S.)
    ('Jur., N.', 'Jur. (n.s.)'),  -- 424 rows, 96% on pages <= 15; truncates Jur., N. S.
    ('L. T. (N.', 'L.T. (n.s.)'),  -- 422 rows, 90% on pages <= 15; truncates L. T. (N. S.)
    ('C. B. (N.', 'CB NS'),  -- 415 rows, 90% on pages <= 15; truncates C. B. (N. S.)
    ('Abb. Pr. N.', 'Abb. Pr. (n.s.)'),  -- 333 rows, 96% on pages <= 15; truncates Abb. Pr. N. S.
    ('Do G. &', 'De G & J'),  -- 320 rows, 73% on pages <= 15; truncates Do G. & J.
    ('D. M. &', 'De G M & G'),  -- 318 rows, 82% on pages <= 15; truncates D. M. & G.
    ('How. U.', 'How.'),  -- 305 rows, 97% on pages <= 15; truncates How. U. S.
    ('Cranch C.', 'Cranch, C. C.'),  -- 278 rows, 79% on pages <= 15; truncates Cranch C. C.
    ('Barb. S.', 'Barb.'),  -- 275 rows, 95% on pages <= 15; truncates Barb. S. C. R.
    ('De G. J. &', 'De G J & S'),  -- 249 rows, 86% on pages <= 15; truncates De G. J. & S.
    ('Dev. &', 'Dev. & Bat.'),  -- 239 rows, 90% on pages <= 15; truncates Dev. & Bat.
    ('Pet. (U.', 'Pet.'),  -- 233 rows, 86% on pages <= 15; truncates Pet. (U. S.)
    ('L. J., K.', 'L.J.K.B.'),  -- 211 rows, 99% on pages <= 15; truncates L. J., K. B.
    ('L. J., N.', 'L.J. (n.s.)'),  -- 200 rows, 89% on pages <= 15; truncates L. J., N. S.
    ('Fost. N.', 'Fost.'),  -- 192 rows, 99% on pages <= 15; truncates Fost. N. H.
    ('Foster (N.', 'Fost.'),  -- 178 rows, 98% on pages <= 15; truncates Foster (N. H.)
    ('Do G. M. &', 'De G M & G'),  -- 175 rows, 84% on pages <= 15; truncates Do G. M. & G.
    ('Har. &', 'H. & G.'),  -- 162 rows, 81% on pages <= 15; truncates Har. & G.
    ('Ohio, N.', 'Ohio C.C. (N.S.)'),  -- 160 rows, 99% on pages <= 15; truncates Ohio, N. S.
    ('Ohio N.', 'Ohio C.C. (N.S.)'),  -- 160 rows, 96% on pages <= 15; truncates Ohio N. S.
    ('Ct. of', 'Ct. Cl.'),  -- 159 rows, 90% on pages <= 15; truncates Ct. of Cl.
    ('Wheat. (U.', 'Wheat.'),  -- 144 rows, 88% on pages <= 15; truncates Wheat. (U. S.)
    ('Gill &', 'G. & J.'),  -- 141 rows, 81% on pages <= 15; truncates Gill & J.
    ('Scrg. &', 'Serg. & Rawl.'),  -- 136 rows, 96% on pages <= 15; truncates Scrg. & R.
    ('Tax', 'T.C.'),  -- 133 rows, 80% on pages <= 15; truncates Tax Cas.
    ('Nott &', 'Nott & McC.'),  -- 130 rows, 79% on pages <= 15; truncates Nott & McC.
    ('Stew. &', 'Stew. & P.'),  -- 119 rows, 90% on pages <= 15; truncates Stew. & P.
    ('WY.', 'Wyo.')   -- 105 rows, 90% on pages <= 15; truncates WY. R.
) AS v(reporter_found, reporter_standard)
 WHERE w.reporter_found = v.reporter_found
   AND w.reporter_standard = v.reporter_standard
   AND w.junk = false;

INSERT INTO legalhist.ocr_corrections (mistake, correction)
SELECT v.mistake, v.correction
FROM (VALUES
    -- 'S.' read as '8.': 73-79% of the '(N.' and '(U.' spellings in a random sample, and the leading reading of every 'N.' spelling
    ('(N. 8.)', '(N. S.)'),
    ('(U. 8.)', '(U. S.)'),
    ('Jur. N. 8.', 'Jur. N. S.'),
    ('Jur., N. 8.', 'Jur., N. S.'),
    ('L. T. N. 8.', 'L. T. N. S.'),
    ('C. B. N. 8.', 'C. B. N. S.'),
    ('C. B., N. 8.', 'C. B., N. S.'),
    ('Abb. Pr. N. 8.', 'Abb. Pr. N. S.'),
    ('L. J., N. 8.', 'L. J., N. S.'),
    ('Ohio, N. 8.', 'Ohio, N. S.'),
    ('Ohio N. 8.', 'Ohio N. S.'),
    ('How. U. 8.', 'How. U. S.'),
    ('M. & 8.', 'M. & S.'),
    ('W. & 8.', 'W. & S.'),
    ('Watts & 8.', 'Watts & S.'),
    ('De G. & 8.', 'De G. & S.'),
    ('De G. J. & 8.', 'De G. J. & S.'),
    -- 'C.' (or 'G.') read as '0.': 72% of 'L. J. M.', and the leading reading of the rest
    ('L. J. M. 0.', 'L. J. M. C.'),
    ('L. J., M. 0.', 'L. J., M. C.'),
    ('L. J. 0. P.', 'L. J. C. P.'),
    ('B. & 0.', 'B. & C.'),
    ('Y. & 0.', 'Y. & C.'),
    ('Bing. N. 0.', 'Bing. N. C.'),
    ('Bro. C. 0.', 'Bro. C. C.'),
    ('Cox, C. 0.', 'Cox, C. C.'),
    ('Cranch C. 0.', 'Cranch C. C.'),
    ('Wash. C. 0.', 'Wash. C. C.'),
    ('Barb. S. 0.', 'Barb. S. C.'),
    ('R. P. 0.', 'R. P. C.'),
    ('Moo. P. 0.', 'Moo. P. C.'),
    ('Abb. N. 0.', 'Abb. N. C.'),
    ('De G. M. & 0.', 'De G. M. & G.'),
    ('D. M. & 0.', 'D. M. & G.'),
    ('Do G. M. & 0.', 'Do G. M. & G.'),
    -- 'B.' read as '1B.', '13.', '11.' or '1.': together about half of 'L. J. Q.' and 'L. J. K.'
    ('L. J. Q. 1B.', 'L. J. Q. B.'),
    ('L. J. Q. 13.', 'L. J. Q. B.'),
    ('L. J. Q. 11.', 'L. J. Q. B.'),
    ('L. J. Q. 1.', 'L. J. Q. B.'),
    ('L. J., Q. 1B.', 'L. J., Q. B.'),
    ('L. J., Q. 13.', 'L. J., Q. B.'),
    ('L. J., Q. 11.', 'L. J., Q. B.'),
    ('L. J., Q. 1.', 'L. J., Q. B.'),
    ('L. J. K. 1B.', 'L. J. K. B.'),
    ('L. J. K. 13.', 'L. J. K. B.'),
    ('L. J. K. 11.', 'L. J. K. B.'),
    ('L. J. K. 1.', 'L. J. K. B.'),
    ('L. J., K. 1B.', 'L. J., K. B.'),
    ('L. J., K. 13.', 'L. J., K. B.'),
    ('L. J., K. 11.', 'L. J., K. B.'),
    ('L. J., K. 1.', 'L. J., K. B.'),
    ('E. & 1B.', 'E. & B.'),
    ('E. & 13.', 'E. & B.'),
    ('E. & 11.', 'E. & B.'),
    ('V. & 1B.', 'V. & B.'),
    ('V. & 13.', 'V. & B.'),
    ('Dev. & 1B.', 'Dev. & B.'),
    ('Dev. & 13.', 'Dev. & B.'),
    ('Dev. & 11.', 'Dev. & B.'),
    ('Am. 1). R.', 'Am. B. R.'),
    ('Am. 11. R.', 'Am. B. R.'),
    ('Am. 13. R.', 'Am. B. R.'),
    ('Am. 1B. R.', 'Am. B. R.'),
    -- 'P.' read as "1'.", '1P.' or '1.': 49-61% of the 'C.' spellings and 59-60% of 'Car. &' and 'Stew. &'
    ('L. J. C. 1''.', 'L. J. C. P.'),
    ('L. J. C. 1P.', 'L. J. C. P.'),
    ('L. J. C. 1.', 'L. J. C. P.'),
    ('L. J., C. 1''.', 'L. J., C. P.'),
    ('L. J., C. 1P.', 'L. J., C. P.'),
    ('L. J., C. 1.', 'L. J., C. P.'),
    ('Car. & 1''.', 'Car. & P.'),
    ('Car. & 1P.', 'Car. & P.'),
    ('Stew. & 1''.', 'Stew. & P.'),
    ('Stew. & 1P.', 'Stew. & P.'),
    ('Ohio N. 1''.', 'Ohio N. P.'),
    -- 'R.' read as '1t.', '1R.', '1:.', '11.' or '1.': together about 40% of 'T. L.' and 'Am. B.'
    ('T. L. 1t.', 'T. L. R.'),
    ('T. L. 1R.', 'T. L. R.'),
    ('T. L. 11.', 'T. L. R.'),
    ('T. L. 1.', 'T. L. R.'),
    ('Times L. 1t.', 'Times L. R.'),
    ('Times L. 1R.', 'Times L. R.'),
    ('Times L. 11.', 'Times L. R.'),
    ('Times L. 1.', 'Times L. R.'),
    ('Am. B. 1R.', 'Am. B. R.'),
    ('Am. B. 1:.', 'Am. B. R.'),
    ('Am. B. 1t.', 'Am. B. R.'),
    ('Am. B. 11.', 'Am. B. R.'),
    ('Am. B. 1.', 'Am. B. R.'),
    ('S. & 1t.', 'S. & R.'),
    ('S. & 1R.', 'S. & R.'),
    ('S. & 11.', 'S. & R.'),
    ('Serg. & 1t.', 'Serg. & R.'),
    ('Serg. & 1R.', 'Serg. & R.'),
    ('Serg. & 11.', 'Serg. & R.'),
    ('D. & 1t.', 'D. & R.'),
    -- 'H.' read as '11.' or '1I.'
    ('J. & 11.', 'J. & H.'),
    ('J. & 1I.', 'J. & H.'),
    ('Fost. N. 11.', 'Fost. N. H.'),
    ('Foster (N. 11.)', 'Foster (N. H.)'),
    -- and the rest: 'M.' read as '31.' or '3.', 'Cl.' as '1.', 'J.' as '3.', '&' as '4' (65% of bare 'Serg.')
    ('L. J. 31. C.', 'L. J. M. C.'),
    ('L. J. 3. C.', 'L. J. M. C.'),
    ('Ct. of 1.', 'Ct. of Cl.'),
    ('Gill & 3.', 'Gill & J.'),
    ('Serg. 4. R', 'Serg. & R'),
    ('Serg. 4 R', 'Serg. & R'),
    ('Serg. 4'' R', 'Serg. & R')
) AS v(mistake, correction)
WHERE NOT EXISTS (
    SELECT 1 FROM legalhist.ocr_corrections o WHERE o.mistake = v.mistake
);

-- migrate:down
SET ROLE = law_admin;

DELETE FROM legalhist.ocr_corrections o
 USING (VALUES
    -- 'S.' read as '8.': 73-79% of the '(N.' and '(U.' spellings in a random sample, and the leading reading of every 'N.' spelling
    ('(N. 8.)', '(N. S.)'),
    ('(U. 8.)', '(U. S.)'),
    ('Jur. N. 8.', 'Jur. N. S.'),
    ('Jur., N. 8.', 'Jur., N. S.'),
    ('L. T. N. 8.', 'L. T. N. S.'),
    ('C. B. N. 8.', 'C. B. N. S.'),
    ('C. B., N. 8.', 'C. B., N. S.'),
    ('Abb. Pr. N. 8.', 'Abb. Pr. N. S.'),
    ('L. J., N. 8.', 'L. J., N. S.'),
    ('Ohio, N. 8.', 'Ohio, N. S.'),
    ('Ohio N. 8.', 'Ohio N. S.'),
    ('How. U. 8.', 'How. U. S.'),
    ('M. & 8.', 'M. & S.'),
    ('W. & 8.', 'W. & S.'),
    ('Watts & 8.', 'Watts & S.'),
    ('De G. & 8.', 'De G. & S.'),
    ('De G. J. & 8.', 'De G. J. & S.'),
    -- 'C.' (or 'G.') read as '0.': 72% of 'L. J. M.', and the leading reading of the rest
    ('L. J. M. 0.', 'L. J. M. C.'),
    ('L. J., M. 0.', 'L. J., M. C.'),
    ('L. J. 0. P.', 'L. J. C. P.'),
    ('B. & 0.', 'B. & C.'),
    ('Y. & 0.', 'Y. & C.'),
    ('Bing. N. 0.', 'Bing. N. C.'),
    ('Bro. C. 0.', 'Bro. C. C.'),
    ('Cox, C. 0.', 'Cox, C. C.'),
    ('Cranch C. 0.', 'Cranch C. C.'),
    ('Wash. C. 0.', 'Wash. C. C.'),
    ('Barb. S. 0.', 'Barb. S. C.'),
    ('R. P. 0.', 'R. P. C.'),
    ('Moo. P. 0.', 'Moo. P. C.'),
    ('Abb. N. 0.', 'Abb. N. C.'),
    ('De G. M. & 0.', 'De G. M. & G.'),
    ('D. M. & 0.', 'D. M. & G.'),
    ('Do G. M. & 0.', 'Do G. M. & G.'),
    -- 'B.' read as '1B.', '13.', '11.' or '1.': together about half of 'L. J. Q.' and 'L. J. K.'
    ('L. J. Q. 1B.', 'L. J. Q. B.'),
    ('L. J. Q. 13.', 'L. J. Q. B.'),
    ('L. J. Q. 11.', 'L. J. Q. B.'),
    ('L. J. Q. 1.', 'L. J. Q. B.'),
    ('L. J., Q. 1B.', 'L. J., Q. B.'),
    ('L. J., Q. 13.', 'L. J., Q. B.'),
    ('L. J., Q. 11.', 'L. J., Q. B.'),
    ('L. J., Q. 1.', 'L. J., Q. B.'),
    ('L. J. K. 1B.', 'L. J. K. B.'),
    ('L. J. K. 13.', 'L. J. K. B.'),
    ('L. J. K. 11.', 'L. J. K. B.'),
    ('L. J. K. 1.', 'L. J. K. B.'),
    ('L. J., K. 1B.', 'L. J., K. B.'),
    ('L. J., K. 13.', 'L. J., K. B.'),
    ('L. J., K. 11.', 'L. J., K. B.'),
    ('L. J., K. 1.', 'L. J., K. B.'),
    ('E. & 1B.', 'E. & B.'),
    ('E. & 13.', 'E. & B.'),
    ('E. & 11.', 'E. & B.'),
    ('V. & 1B.', 'V. & B.'),
    ('V. & 13.', 'V. & B.'),
    ('Dev. & 1B.', 'Dev. & B.'),
    ('Dev. & 13.', 'Dev. & B.'),
    ('Dev. & 11.', 'Dev. & B.'),
    ('Am. 1). R.', 'Am. B. R.'),
    ('Am. 11. R.', 'Am. B. R.'),
    ('Am. 13. R.', 'Am. B. R.'),
    ('Am. 1B. R.', 'Am. B. R.'),
    -- 'P.' read as "1'.", '1P.' or '1.': 49-61% of the 'C.' spellings and 59-60% of 'Car. &' and 'Stew. &'
    ('L. J. C. 1''.', 'L. J. C. P.'),
    ('L. J. C. 1P.', 'L. J. C. P.'),
    ('L. J. C. 1.', 'L. J. C. P.'),
    ('L. J., C. 1''.', 'L. J., C. P.'),
    ('L. J., C. 1P.', 'L. J., C. P.'),
    ('L. J., C. 1.', 'L. J., C. P.'),
    ('Car. & 1''.', 'Car. & P.'),
    ('Car. & 1P.', 'Car. & P.'),
    ('Stew. & 1''.', 'Stew. & P.'),
    ('Stew. & 1P.', 'Stew. & P.'),
    ('Ohio N. 1''.', 'Ohio N. P.'),
    -- 'R.' read as '1t.', '1R.', '1:.', '11.' or '1.': together about 40% of 'T. L.' and 'Am. B.'
    ('T. L. 1t.', 'T. L. R.'),
    ('T. L. 1R.', 'T. L. R.'),
    ('T. L. 11.', 'T. L. R.'),
    ('T. L. 1.', 'T. L. R.'),
    ('Times L. 1t.', 'Times L. R.'),
    ('Times L. 1R.', 'Times L. R.'),
    ('Times L. 11.', 'Times L. R.'),
    ('Times L. 1.', 'Times L. R.'),
    ('Am. B. 1R.', 'Am. B. R.'),
    ('Am. B. 1:.', 'Am. B. R.'),
    ('Am. B. 1t.', 'Am. B. R.'),
    ('Am. B. 11.', 'Am. B. R.'),
    ('Am. B. 1.', 'Am. B. R.'),
    ('S. & 1t.', 'S. & R.'),
    ('S. & 1R.', 'S. & R.'),
    ('S. & 11.', 'S. & R.'),
    ('Serg. & 1t.', 'Serg. & R.'),
    ('Serg. & 1R.', 'Serg. & R.'),
    ('Serg. & 11.', 'Serg. & R.'),
    ('D. & 1t.', 'D. & R.'),
    -- 'H.' read as '11.' or '1I.'
    ('J. & 11.', 'J. & H.'),
    ('J. & 1I.', 'J. & H.'),
    ('Fost. N. 11.', 'Fost. N. H.'),
    ('Foster (N. 11.)', 'Foster (N. H.)'),
    -- and the rest: 'M.' read as '31.' or '3.', 'Cl.' as '1.', 'J.' as '3.', '&' as '4' (65% of bare 'Serg.')
    ('L. J. 31. C.', 'L. J. M. C.'),
    ('L. J. 3. C.', 'L. J. M. C.'),
    ('Ct. of 1.', 'Ct. of Cl.'),
    ('Gill & 3.', 'Gill & J.'),
    ('Serg. 4. R', 'Serg. & R'),
    ('Serg. 4 R', 'Serg. & R'),
    ('Serg. 4'' R', 'Serg. & R')
) AS v(mistake, correction)
 WHERE o.mistake = v.mistake AND o.correction = v.correction;

UPDATE legalhist.whitelist w
   SET junk = false,
       reporter_standard = v.reporter_standard
  FROM (VALUES
    ('Am.', 'A.B.R.'),  -- 20,999 rows, 61% on pages <= 15; the rest are Am. Rep. and Am. Dec. cites, so wrong in every reading
    ('B. &', 'B & A'),  -- 13,676 rows, 85% on pages <= 15; truncates B. & A.
    ('L. J. Q.', 'L.J.K.B.'),  -- 11,405 rows, 97% on pages <= 15; truncates L. J. Q. B.
    ('M. &', 'M & S'),  -- 9,012 rows, 79% on pages <= 15; truncates M. & S.
    ('T. L.', 'T.L.R.'),  -- 7,100 rows, 93% on pages <= 15; truncates T. L. R.
    ('Jur. N.', 'Jur. (n.s.)'),  -- 4,442 rows, 96% on pages <= 15; truncates Jur. N. S.
    ('E. &', 'El & Bl'),  -- 4,350 rows, 94% on pages <= 15; truncates E. & B.
    ('L. J. M.', 'L.J.M.C.'),  -- 3,794 rows, 93% on pages <= 15; truncates L. J. M. C.
    ('S. &', 'Serg. & Rawl.'),  -- 3,080 rows, 92% on pages <= 15; truncates S. & R.
    ('Am. B.', 'A.B.R.'),  -- 2,919 rows, 80% on pages <= 15; truncates Am. B. R.
    ('D. &', 'Dowl. & Ry.'),  -- 2,750 rows, 86% on pages <= 15; truncates D. & R.
    ('L. J. K.', 'L.J.K.B.'),  -- 2,690 rows, 99% on pages <= 15; truncates L. J. K. B.
    ('Bing. N.', 'Bing NC'),  -- 2,557 rows, 90% on pages <= 15; truncates Bing. N. C.
    ('Car. &', 'Car & P'),  -- 2,447 rows, 90% on pages <= 15; truncates Car. & P.
    ('L. T. N.', 'L.T. (n.s.)'),  -- 2,351 rows, 88% on pages <= 15; truncates L. T. N. S.
    ('A. &', 'L.R.A.E.'),  -- 2,267 rows, 82% on pages <= 15; truncates A. & E.
    ('L. J., Q.', 'L.J.K.B.'),  -- 2,156 rows, 96% on pages <= 15; truncates L. J., Q. B.
    ('R. P.', 'R.P.C.'),  -- 2,115 rows, 94% on pages <= 15; truncates R. P. C.
    ('C. B. N.', 'CB NS'),  -- 2,083 rows, 94% on pages <= 15; truncates C. B. N. S.
    ('Serg.', 'Serg. & Rawl.'),  -- 2,029 rows, 92% on pages <= 15; truncates Serg. & R.
    ('Y. &', 'Y & C Ex'),  -- 1,884 rows, 90% on pages <= 15; truncates Y. & C.
    ('L. J., C.', 'L.J.C.P.'),  -- 1,733 rows, 71% on pages <= 15; truncates L. J., C. P.
    ('W. &', 'Watts & Serg.'),  -- 1,478 rows, 89% on pages <= 15; truncates W. & S.
    ('Bro. C.', 'Bro CC'),  -- 1,393 rows, 76% on pages <= 15; truncates Bro. C. C.
    ('J. &', 'J & H'),  -- 1,278 rows, 85% on pages <= 15; truncates J. & H.
    ('Times L.', 'T.L.R.'),  -- 1,268 rows, 97% on pages <= 15; truncates Times L. R.
    ('Ad. &', 'Ad & E'),  -- 1,234 rows, 72% on pages <= 15; truncates Ad. & E.
    ('Moo. P.', 'Moo PC'),  -- 1,205 rows, 96% on pages <= 15; truncates Moo. P. C.
    ('V. &', 'Ves & Bea'),  -- 1,090 rows, 94% on pages <= 15; truncates V. & B.
    ('Abb. N.', 'Abb. Pr. (n.s.)'),  -- 1,033 rows, 94% on pages <= 15; truncates Abb. N. S.
    ('C. B., N.', 'CB NS'),  -- 983 rows, 94% on pages <= 15; truncates C. B., N. S.
    ('Serg. &', 'Serg. & Rawl.'),  -- 936 rows, 92% on pages <= 15; truncates Serg. & R.
    ('Cox, C.', 'Cox'),  -- 903 rows, 94% on pages <= 15; truncates Cox, C. C.
    ('L. J., M.', 'L.J.M.C.'),  -- 887 rows, 97% on pages <= 15; truncates L. J., M. C.
    ('De G. &', 'De G & J'),  -- 748 rows, 77% on pages <= 15; truncates De G. & J.
    ('Wash. C.', 'Wash. C. C.'),  -- 588 rows, 84% on pages <= 15; truncates Wash. C. C.
    ('Wall. (U.', 'Wall.'),  -- 580 rows, 84% on pages <= 15; truncates Wall. (U. S.)
    ('Watts &', 'Watts & Serg.'),  -- 549 rows, 93% on pages <= 15; truncates Watts & S.
    ('Jur. (N.', 'Jur. (n.s.)'),  -- 538 rows, 95% on pages <= 15; truncates Jur. (N. S.)
    ('De G. M. &', 'De G M & G'),  -- 474 rows, 77% on pages <= 15; truncates De G. M. & G.
    ('How. (U.', 'How.'),  -- 461 rows, 85% on pages <= 15; truncates How. (U. S.)
    ('Jur., N.', 'Jur. (n.s.)'),  -- 424 rows, 96% on pages <= 15; truncates Jur., N. S.
    ('L. T. (N.', 'L.T. (n.s.)'),  -- 422 rows, 90% on pages <= 15; truncates L. T. (N. S.)
    ('C. B. (N.', 'CB NS'),  -- 415 rows, 90% on pages <= 15; truncates C. B. (N. S.)
    ('Abb. Pr. N.', 'Abb. Pr. (n.s.)'),  -- 333 rows, 96% on pages <= 15; truncates Abb. Pr. N. S.
    ('Do G. &', 'De G & J'),  -- 320 rows, 73% on pages <= 15; truncates Do G. & J.
    ('D. M. &', 'De G M & G'),  -- 318 rows, 82% on pages <= 15; truncates D. M. & G.
    ('How. U.', 'How.'),  -- 305 rows, 97% on pages <= 15; truncates How. U. S.
    ('Cranch C.', 'Cranch, C. C.'),  -- 278 rows, 79% on pages <= 15; truncates Cranch C. C.
    ('Barb. S.', 'Barb.'),  -- 275 rows, 95% on pages <= 15; truncates Barb. S. C. R.
    ('De G. J. &', 'De G J & S'),  -- 249 rows, 86% on pages <= 15; truncates De G. J. & S.
    ('Dev. &', 'Dev. & Bat.'),  -- 239 rows, 90% on pages <= 15; truncates Dev. & Bat.
    ('Pet. (U.', 'Pet.'),  -- 233 rows, 86% on pages <= 15; truncates Pet. (U. S.)
    ('L. J., K.', 'L.J.K.B.'),  -- 211 rows, 99% on pages <= 15; truncates L. J., K. B.
    ('L. J., N.', 'L.J. (n.s.)'),  -- 200 rows, 89% on pages <= 15; truncates L. J., N. S.
    ('Fost. N.', 'Fost.'),  -- 192 rows, 99% on pages <= 15; truncates Fost. N. H.
    ('Foster (N.', 'Fost.'),  -- 178 rows, 98% on pages <= 15; truncates Foster (N. H.)
    ('Do G. M. &', 'De G M & G'),  -- 175 rows, 84% on pages <= 15; truncates Do G. M. & G.
    ('Har. &', 'H. & G.'),  -- 162 rows, 81% on pages <= 15; truncates Har. & G.
    ('Ohio, N.', 'Ohio C.C. (N.S.)'),  -- 160 rows, 99% on pages <= 15; truncates Ohio, N. S.
    ('Ohio N.', 'Ohio C.C. (N.S.)'),  -- 160 rows, 96% on pages <= 15; truncates Ohio N. S.
    ('Ct. of', 'Ct. Cl.'),  -- 159 rows, 90% on pages <= 15; truncates Ct. of Cl.
    ('Wheat. (U.', 'Wheat.'),  -- 144 rows, 88% on pages <= 15; truncates Wheat. (U. S.)
    ('Gill &', 'G. & J.'),  -- 141 rows, 81% on pages <= 15; truncates Gill & J.
    ('Scrg. &', 'Serg. & Rawl.'),  -- 136 rows, 96% on pages <= 15; truncates Scrg. & R.
    ('Tax', 'T.C.'),  -- 133 rows, 80% on pages <= 15; truncates Tax Cas.
    ('Nott &', 'Nott & McC.'),  -- 130 rows, 79% on pages <= 15; truncates Nott & McC.
    ('Stew. &', 'Stew. & P.'),  -- 119 rows, 90% on pages <= 15; truncates Stew. & P.
    ('WY.', 'Wyo.')   -- 105 rows, 90% on pages <= 15; truncates WY. R.
) AS v(reporter_found, reporter_standard)
 WHERE w.reporter_found = v.reporter_found
   AND w.junk = true
   AND w.reporter_standard IS NULL
   AND EXISTS (SELECT 1 FROM legalhist.reporters r WHERE r.reporter_standard = v.reporter_standard);
