-- Whitelisted spellings of single-volume reporters that no single-volume
-- detector finds when they appear without a volume (issue #275, item 2).
--
-- cite-detector-moml builds one detector per (single_vol reporter, spelling)
-- from legalhist.reporters.reporter_standard and reporters_abbreviations.alt_abbr
-- (GetSingleVolReporterAbbrs, go/citations/store_db.go). The regex is
--   \b + QuoteMeta(spelling) with each space replaced by [\s.]* + <stem>[.,]*\s+\d{1,4}
-- and is case-sensitive (NewSingleVolDetector, go/citations/detector.go). A
-- whitelist spelling that maps to a single-volume reporter but matches none of
-- those patterns is only ever detected when the generic detector sees a volume
-- in front of it, so a bare citation in that spelling is never recorded. The
-- volumed row count is therefore a lower bound on how often the spelling occurs.
--
-- <stem> is \w* only for spellings of stemMinAbbrLen (5) characters or more,
-- counting neither whitespace nor periods, and empty for shorter ones (issue
-- #283). This query has to reproduce that gate, because it is how the long forms
-- a short abbreviation no longer reaches get found and registered as alternates.
--
-- Coverage is tested against every detector's pattern, not only the patterns of
-- the reporter the spelling maps to, because the stem lets one reporter's
-- detector record another reporter's spelling ("Barnes" from a "Barnw" detector).
--
-- Read-only; run with LAW_CLAUDE. Takes a few minutes for the per-spelling counts.
SET statement_timeout = '600s';

WITH sv AS (
  SELECT reporter_standard FROM legalhist.reporters WHERE single_vol
),
patterns AS (
  SELECT reporter_standard AS abbr FROM sv
  UNION
  SELECT a.alt_abbr FROM legalhist.reporters_abbreviations a JOIN sv USING (reporter_standard)
),
rx AS (
  SELECT abbr,
         '^'
         || replace(regexp_replace(abbr, '([.^$*+?()\[\]{}|\\])', '\\\1', 'g'), ' ', '[\s.]*')
         -- abbrCoreLen: length without whitespace or periods, gated at
         -- stemMinAbbrLen = 5.
         || CASE WHEN length(regexp_replace(abbr, '[\s.]', '', 'g')) >= 5
                 THEN '\w*' ELSE '' END
         || '[.,]*$' AS re
  FROM patterns
),
wl AS (
  SELECT w.reporter_found, w.reporter_standard
  FROM legalhist.whitelist w JOIN sv USING (reporter_standard)
),
uncovered AS (
  SELECT wl.reporter_found, wl.reporter_standard
  FROM wl
  WHERE NOT EXISTS (SELECT 1 FROM rx WHERE wl.reporter_found ~ rx.re)
)
SELECT u.reporter_standard,
       u.reporter_found,
       (SELECT count(*) FROM moml_citations.citations_unlinked c
         WHERE c.reporter_abbr = u.reporter_found AND c.volume IS NOT NULL) AS volumed_rows,
       (SELECT count(*) FROM moml_citations.citations_unlinked c
         WHERE c.reporter_abbr = u.reporter_found AND c.volume IS NULL) AS bare_rows
FROM uncovered u
ORDER BY volumed_rows DESC, reporter_standard, reporter_found;
