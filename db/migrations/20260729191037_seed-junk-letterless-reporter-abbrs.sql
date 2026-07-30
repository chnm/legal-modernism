-- migrate:up
SET ROLE = law_admin;

-- Issue #254: junk the mechanical noise in the non-whitelisted tail by seeding
-- legalhist.whitelist, rather than by adding a rules table, a new link status,
-- or logic in the linker. No code change is needed: the linker's first step is
-- the whitelist lookup, so a junk row is all it takes for these to be recorded
-- as skipped_junk (cite-linker/main.go:359), and every consumer of that status
-- -- the dashboard matviews, chambers, ResetUnlinked -- already handles it.
--
-- What the noise is. GenericDetector's abbreviation pattern is abbrChar{3,16}
-- (go/citations/generic.go), and abbrChar is [\p{L}\s\.,&\(\)] -- whitespace
-- and punctuation, with no requirement that a letter appear at all. Two numbers
-- separated by nothing but leader dots, spaces, commas, or parentheses
-- therefore match the citation pattern exactly, and treatises are full of them:
-- tables of contents, indexes, and columns of page numbers.
--
--   "Of the nature of contracts . . . 12 . . . 456"  ->  abbr ". . ."
--   "Index entry 7 ......... 231"                    ->  abbr "........."
--   "Table row 5     88"                             ->  abbr ""
--
-- The empty abbreviation is the largest single spelling (17,212 citations): the
-- detector normalizes whitespace and trims, so a run of spaces between two
-- numbers records as ''. TestDetector_NoLetterAbbrs in
-- go/citations/detector_test.go pins these detections, so if abbrChar is ever
-- tightened to require a letter that test fails and these rows can be dropped.
--
-- Why the rule is safe. A reporter abbreviation names a person, court, or
-- jurisdiction, so it cannot be spelled without a letter. Verified against the
-- live database on 2026-07-29: zero rows lack an ASCII letter in
-- legalhist.reporters (reporter_standard), legalhist.reporters_abbreviations
-- (alt_abbr), legalhist.code_reporter (official_citation), or the non-junk part
-- of legalhist.whitelist (reporter_found). So "contains no letter" implies "not
-- a citation" with no judgment call to make -- which is what distinguishes this
-- from the other whitelist-seeding migrations (20260728162040 and friends),
-- where every row was a reading of what the OCR meant and had to be listed and
-- argued for one at a time.
--
-- [A-Za-z] and not the POSIX [[:alpha:]] class. The database is UTF-8, and
-- [[:alpha:]] counts the OCR's accented junk as letters: 832 citations have
-- abbreviations built from a stray 'o-acute' or a circumflex where the scan
-- mangled a leader dot ("o.", "(o)", ". o.", "^ ^"). Those are the same noise as
-- the dots around them, and no reporter is spelled with non-ASCII letters only
-- (verified above), so the ASCII class is both safe and the one that classifies
-- them honestly.
--
-- Why INSERT ... SELECT and not a VALUES list. The rule has no exceptions to
-- enumerate, so the predicate is the entire content of the change and a listing
-- would only invite a reviewer to re-derive it from 5,856 rows of dots. The set
-- is also open-ended -- abbrChar admits any permutation of six symbols up to 16
-- characters, so each new batch of scanned pages yields leader-dot spellings not
-- seen before. A literal list is stale the next time the detector runs; this
-- statement can simply be run again.
--
-- Junk rows carry reporter_standard IS NULL, per chk_whitelist_junk_no_standard.
--
-- Measured against the live database on 2026-07-29:
--   1,136,468 citations have a letterless reporter_abbr, in 6,170 distinct
--   spellings. 314 of those spellings are already in the whitelist as junk (the
--   highest-frequency leader-dot runs, junked by hand), which is why 1,076,842
--   of the citations are already skipped_junk. This migration inserts the
--   remaining 5,856 spellings and so reclassifies 59,626 citations from
--   skipped_not_whitelisted to skipped_junk. None of the 1,136,468 are linked or
--   even no_match, so no link can be lost.
--
-- The NOT EXISTS guard respects the reporter_found primary key and, more to the
-- point, leaves any pre-existing row for one of these spellings exactly as it
-- is rather than overwriting a human decision.
INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk)
SELECT DISTINCT cu.reporter_abbr, NULL::text, true
FROM moml_citations.citations_unlinked cu
WHERE cu.reporter_abbr !~ '[A-Za-z]'
  AND NOT EXISTS (
    SELECT 1 FROM legalhist.whitelist w
    WHERE w.reporter_found = cu.reporter_abbr
  );

-- migrate:down
SET ROLE = law_admin;

-- Delete exactly the rows the up inserted. The predicate alone is not enough:
-- legalhist.whitelist already held 341 letterless junk rows before this
-- migration (all junk, none with a standard reporter), which the up's NOT EXISTS
-- guard skipped and which must survive the down -- deleting them would push the
-- 1,076,842 citations they cover back to skipped_not_whitelisted on the next
-- linker run. They are therefore listed here and excluded. NOT IN is safe with a
-- literal list, which cannot yield the NULL that would make it match nothing.
DELETE FROM legalhist.whitelist
WHERE junk
  AND reporter_standard IS NULL
  AND reporter_found !~ '[A-Za-z]'
  AND reporter_found NOT IN (
    '&', '& .', '(', '( )', '( .', '()', '(),', '().', '(,)', '(,.', '(. &',
    '(. .', '(.)', '(..', ')', ') (', ') .', ').', ',', ', (', ', ,',
    ', , ,', ', ,,', ', ,, ,,', ', .', ', . .', ', . . .', ', ..',
    ', ........', ',,', ',, ,', ',, ,,', ',, ,, ,', ',, ,, ,,',
    ',, ,, ,, ,,', ',, ,, .', ',, .', ',, . .', ',, ..', ',, ...',
    ',, ........', ',,,', ',,,,', ',,.', ',,........', ',.', ',. .', ',.,',
    ',..', ',........', '.', '. &', '. & .', '. (', '. (.', '. )', '. ).',
    '. ,', '. , .', '. ,,', '. ,.', '. .', '. . ,', '. . .', '. . . ,',
    '. . . .', '. . . . ,', '. . . . .', '. . . . . .', '. . . . . . .',
    '. . . . . . . .', '. . . . . . ..', '. . . . . . ...', '. . . . . ..',
    '. . . . . .. .', '. . . . . ...', '. . . . ..', '. . . . .. .',
    '. . . . ...', '. . . ..', '. . . .. .', '. . . .. . .', '. . . ...',
    '. . .,', '. . ..', '. . .. .', '. . .. . .', '. . .. . . .',
    '. . .. ..', '. . ...', '. . ... .', '. . ....', '. . .....',
    '. . ......', '. . .......', '. . ........', '. .,', '. ..', '. .. .',
    '. .. . .', '. .. . . .', '. .. . . . .', '. .. . ..', '. .. ..',
    '. .. .. .', '. .. .. ..', '. .. .. .. ..', '. .. ...', '. .. ... ...',
    '. .. ......', '. ...', '. ... .', '. ... . .', '. ... ..', '. ... ...',
    '. ... ... ...', '. ....', '. .... .', '. .... ..', '. .... ...',
    '. .....', '. ..... .', '. ..... ...', '. ......', '. ...... .',
    '. .......', '. ....... .', '. ....... ...', '. ........',
    '. ........ .', '. .........', '. ......... .', '. ..........',
    '. .......... .', '. ...........', '. ............', '. .............',
    '. ..............', '.)', '.,', '., .', '.,,', '.,.', '..', '.. ,',
    '.. .', '.. . .', '.. . . .', '.. . . . .', '.. . . . . .',
    '.. . . . . . .', '.. . . ..', '.. . ..', '.. . .. .', '.. . .. ..',
    '.. . .. .. ..', '.. . ...', '.. . ... ...', '.. . ..... .',
    '.. . ...... .', '.. . ....... .', '.. ..', '.. .. .', '.. .. . .',
    '.. .. . ..', '.. .. . .. ..', '.. .. ..', '.. .. .. .', '.. .. .. . .',
    '.. .. .. . ..', '.. .. .. ..', '.. .. .. .. .', '.. .. .. .. ..',
    '.. .. .. .. .. .', '.. .. .. ...', '.. .. .. ....', '.. .. ...',
    '.. .. ....', '.. ...', '.. ... .', '.. ... ..', '.. ... ...',
    '.. ... ... ...', '.. ....', '.. .... .', '.. .... ..', '.. .... ...',
    '.. .....', '.. ..... .', '.. ......', '.. ...... .', '.. .......',
    '.. ....... .', '.. ........', '.. .........', '.. ..........',
    '.. ...........', '.. ............', '..,', '...', '... ,', '... .',
    '... . .', '... . . .', '... . . . .', '... . . . . .',
    '... . . . . . .', '... . ..', '... . .. ...', '... . ...',
    '... . ... ...', '... ..', '... .. .', '... .. . ...', '... .. ..',
    '... .. .. ...', '... .. ...', '... .. ... ..', '... .. ... ...',
    '... ...', '... ... .', '... ... . .', '... ... . . ..', '... ... . ..',
    '... ... . ...', '... ... . ....', '... ... ..', '... ... .. .',
    '... ... .. ..', '... ... .. ...', '... ... ...', '... ... ... .',
    '... ... ... ..', '... ... ... ...', '... ... ....', '... ... .... ...',
    '... ... .....', '... ... ......', '... ... .......', '... ....',
    '... .... ..', '... .... ...', '... .....', '... ..... .',
    '... ..... ...', '... ......', '... ...... .', '... ...... ..',
    '... ...... ...', '... .......', '... ....... ...', '... ........',
    '... ........ ...', '... .........', '... ..........',
    '... ...........', '... ............', '....', '.... .', '.... . .',
    '.... . . .', '.... . ..', '.... ..', '.... .. .', '.... .. ..',
    '.... .. .. ..', '.... .. ...', '.... ...', '.... ... ...', '.... ....',
    '.... .....', '.... ......', '.... .......', '.... ........',
    '.... .........', '.... ..........', '.....', '..... .', '..... . .',
    '..... ..', '..... ...', '..... ... ...', '..... ....', '..... .....',
    '..... ......', '..... .......', '..... ........', '..... .........',
    '......', '...... .', '...... . .', '...... ..', '...... ...',
    '...... ... ...', '...... ....', '...... .....', '...... ......',
    '...... .......', '...... ........', '.......', '....... .',
    '....... . .', '....... ..', '....... ...', '....... ....',
    '....... .....', '....... ......', '....... .......', '........',
    '........ .', '........ ..', '........ ...', '........ ....',
    '........ .....', '........ ......', '........ .......', '.........',
    '......... .', '......... ..', '......... ...', '......... ....',
    '......... .....', '..........', '.......... .', '.......... ..',
    '.......... ...', '.......... ....', '.......... .....', '...........',
    '........... .', '........... ..', '........... ...', '............',
    '............ .', '............ ..', '............ ...',
    '.............', '............. .', '..............', '...............',
    '................'
  );
