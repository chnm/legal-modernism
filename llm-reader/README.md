# llm-reader

Claude reads MOML pages one at a time and links the cases they cite, by its
own judgement (issue #439). It starts with the editions of Stephen on Pleading
(work 1107): the twelve U.S. editions, then the seven UK ones, each read in
its official copy (`moml.treatises`: a HathiTrust scan where there is one,
since PR #443). Gale copies read before that change keep their records. Nothing here uses
the regex detector or linker: a reader sees a page's OCR text and looks the
cases up itself, so the result can be compared with `moml_citations.citation_links`.

- `manifests/work-1107.tsv` lists every page, in reading order (U.S. editions
  by year, then UK).
- `results/<psmid>/<pageid>.json` is the record of one page read. A page with
  a record is done; the work resumes from the first page without one.
- `moml_citations.llm_pages` and `moml_citations.llm_links` hold the records
  once loaded (`db/migrations/20261120020229_llm-links.sql`).

Set `LAW_DBSTR` on the command line each time (`LAW_DBSTR="$LAW_CLAUDE"
bin/llm-reader ...`): an `export` does not carry over between Claude's shell
calls. Read-only access is enough for everything but `load`.

```sh
go build -o bin/llm-reader ./llm-reader/          # from the repository root
bin/llm-reader todo --limit 25 --jurisdiction US      # next pages to read
bin/llm-reader page <psmid> <pageid>                  # a page's OCR text
bin/llm-reader validate --db                          # check every record and its targets
bin/llm-reader serve --port 4314 --jurisdiction US    # progress bar
LAW_DBSTR=<write access> bin/llm-reader load          # replace each page's rows with its record
```

`load` deletes and reinserts each page's rows, so it can be run again at any
time as more pages are read. Run `validate --db` first: a target that does not
exist would fail the load on a foreign key.

## Instructions for a reader

You are reading pages of a nineteenth-century legal treatise, OCR'd, and
recording every citation of a **case** with the case it refers to. Use your
judgement as a legal historian would: the printed case name, the reporter's
period and jurisdiction, the edition's date, and the surrounding text all
count. Do not look at `moml_citations.*`: the point is an independent reading.

### What to record

- Every citation of a reported case: `Rush v. Seymour, 10 Mod. 88`,
  `4 Taunt. 572`, `Herlakenden's Case, 4 Rep. 62 a`, `1 Saund. 28, n. (3)`
  (Saunders's notes are cited as a case report: record it, and link the case
  at that page if there is one). A string of cites is one record each.
- An unreported manuscript case (`Carter v. Tams, D. Ct. Philad. 1836, MS.`)
  is not recorded: it has no report to link.
- An unnamed case cited only inside another report (`Cited ibid.`) is not
  recorded: it has neither a name nor a cite of its own.
- A case cited by name with no reporter (`as in Slade's case`) is not
  recorded; where its report is cited elsewhere on the page, that cite is the
  one record.
- Year books (cited directly or through an abridgment, `Vin. Ab. ... cites
  15 Hen. 7. 10`), e.g. (`2 Ed. 3. 1`, `Yearb. 21 H. 6. 9`, `22 Ass. 41`) are reports
  of cases with no source here: always record them, as `no_match`, leaving
  out `page` when the folio cannot be read. Its `confidence` is how surely you read the
  cite: `high` when legible, `low` when garbled. Write `reporter_abbr` as `Y.B.`
  and the regnal year as `volume`, with the reign in `note` (`10 Edw. III`).
  A year book cited only by an editor's page (`Mayn. 109`) leaves `volume`
  empty; one that prints its regnal year too (`18 E. 2. 614`) keeps it. A year book named with no folio (`the Year-books, viz. 1 Ed. 2`) is not
  recorded; one cited by plea number (`8 Edw. 3, pl. 20`) is, with `page`
  left out and the plea in `note`. Records of pleas (Placitorum Abbreviatio,
  Ryley's Placita Parliamentaria) and maxims (Heath's Maxims) are not reports.
- Reports that are also books of entries (`Lutw.`) are reports: record them.
  Books of precedents only (Lilly's Entries, Modern Entries, Rastall,
  Coke's Entries, East's Entries, Wentworth, Upper Bench Precedents, Liber Placitandi) are
  not. A year book garbled in the OCR but read surely from another
  edition's text is `medium`. A bare `Ibid.`
  or `see same cases` pointing back to a whole earlier footnote of several
  cites is not recorded again.
- Not cases: treatises and abridgments (Tidd, Co. Litt., Com. Dig., Bac. Abr.,
  Vin. Abr., Arch. Pract., Chit. Pl., Selw. N. P., 2 Inst., Booth, Smith's Leading
  Cases (a collection, not a report), Chitty on Pleading (`1 Chitty, 498,
  1st edit.`, not Chitty's K.B. Reports), Gilbert on Replevin (`Gilb. Rep. (Hunt's Edit.)`,
  not Gilbert's Reports), and the practice books Richardson's K. B., Manning's Exchequer, `Mann. Exch.` or `1 Manning`, and the Practical Register, `Pract. Reg.`), statutes
  (`4 Ann. c. 16`), an abridgment entry that names no year-book folio
  (`Fitz. Abr. tit. Eject. firm. 2`), a reporter's preface (`Co. Pref. to
  10 Rep.`), rules of court even when printed in a report
  (`6 Bing. 802`), cross-references (`supra, p. 32`). Skip them.
- Tables of cases and indexes list case names with the treatise's own page
  numbers, not citations: record the page with `"citations": []` and a note,
  even when an entry carries a report to tell Anonymous cases apart
  (`Anonymous, (2 Salk. 519)`).
  So too title pages, advertisements, blank pages.
- Footnote markers (`(q)`, `(r)`) and running heads are not citations.
- `Ib.`, `Id.`, `ubi supra`, a case name followed by `sup.` or `supra`
  (`Martin v. Kesterton, sup.`), `S. C.` (same case) and a bare `Ibid. 233` refer back to the
  previous cite: resolve them to the volume and reporter they stand for and
  record them, noting the expansion in `note`.
- When a citation is cut off at the bottom of the page and continues on the
  next, record it on the page where its page number is printed (even if the
  name and reporter end the page before);
  run `bin/llm-reader page` on the neighbouring page if you need to see it.

### Reading the OCR: the name leads, the numbers are suspect

This is OCR of old print, and the numbers are where it fails most: volumes and
pages are misread (`I`, `l` for 1; `S` for 5 or 8; `O` for 0; `3` for 8;
dropped or doubled digits; a semicolon after a page often comes through as
an extra digit, `5415` for `545;`), and a stray number after a cite is as likely to be
noise as a second page. The case names survive much better. So read every
citation as evidence to be weighed, not a key to be looked up:

- **Start from the case name when one is printed.** Find the case by name
  (`er_name ILIKE '%dumsday%'`, `name_abbreviation ILIKE '%seymour%'`), then
  check that the reporter, volume and page are a plausible reading of what is
  printed. A case of that name in that reporter, near that volume and page,
  is the link even when the printed numbers are off: say what you corrected
  in `note`.
- **Use everything on the page:** the sentence the footnote supports, the
  other cites in the string, an `S. C.` or `Ib.` pointing back, the same case
  cited elsewhere on the page or a neighbouring one, the reporter's period,
  and the edition's year (no case it cites is later).
- **When no name is printed**, the numbers are all you have: a case beginning
  at the exact page is a link at `medium` confidence; a page inside a case
  whose subject fits the text is a pin cite at `medium` or `low`.
- **A name and numbers that point to different cases**: trust the name and
  look for it nearby (another volume, a transposed digit, the same case in
  another reporter). Record `no_match` only when you cannot find the named
  case at all, and say in `note` what you tried and what candidates you saw.

- **Other editions help.** The same footnote usually appears in other
  editions of the treatise, often legible where this one is not: search
  `moml.page_ocrtext` for the other volumes of the work (`moml.volumes`
  joined to `moml.editions` on `work_id`; searching all of MOML times out):

  ```sql
  SELECT v.year, o.psmid, o.pageid FROM moml.page_ocrtext o
    JOIN moml.volumes v USING (psmid) JOIN moml.editions e USING (bibliographicid)
   WHERE e.work_id = 1107 AND o.ocrtext ILIKE '%Dumsday%' ORDER BY v.year;
  ```

  That is page text, not the
  pipeline's results, so it is fair to use. So are the records other readers
  have written for the same footnote in another edition
  (`llm-reader/results/`): use them as a cross-check and keep the editions
  consistent, but still read the page and check what is doubtful, since an
  earlier reader can be wrong.

When this page's OCR has lost a line that every other edition prints, record
the cite from the other editions at `medium`, with `raw` as they print it and
a note saying so.

Record `raw` exactly as printed, OCR errors and all, but with line breaks
inside a citation collapsed to single spaces.

### Looking the case up

Query with `psql "$LAW_CLAUDE"`, read-only (the role cannot create
temporary tables; use a CTE). In the English Reports, check `murrell_title`
as well as `er_name`, which is sometimes wrong; a record can even carry the
wrong case's title, so the printed name and the subject of the passage decide.
`english_reports.cases.id` is text: quote it (`id IN ('17953003')`). A page with a folio letter
(`228 a`) is page 228, and a pin cite only if 228 is not the case's first page. Several `Anonymous` cases at one page are told apart
by subject; when they cannot be, pick one at `low` and say so. Names vary in spelling,
abbreviation and order of parties (`Rex v.` = `The King against`,
`Doe d. A v. B`, `ex parte`, `Cook`/`Cooke`, `Garret`/`Garrett`). When a cite
prints no volume (`Ld. Ray. 883`), find the volume from the case name and say
in `note` that you supplied it. When a pin cite falls on a page where one
case ends and another begins, the printed name decides which. `S. C.` (same
case) points to the case just named: find that case in the reporter given.
Some reporters were paged differently in different editions (Douglas,
Strange): a named case found a page or two from the printed page is linked,
with a note. A note to a case (`1 Saund. 318, n. (3)`), or any editor's note (`the
editor of Yelverton, p. 195 b`), belongs to the case
it annotates, which may begin a page or two before the note. On a page where
one case ends and another begins, a note goes to the case running onto the
page, unless its subject plainly belongs to the one beginning there.
Williams numbers each case's notes from (1), so a high-numbered note on the
page where a new case begins belongs to the case running onto it. Choices
already made: 2 Saund. 319 n. (6) is Bennet v. Holbech (whose record begins at
317), even though the record that begins at 319 is Hoskins v. Robins; 1 Saund. 318 n. (3)
is Redman v. Edolph; 1 Saund. 228 n. (1)
is Stennel v. Hogg; Goram v. Sweeting (2 Saund. 200–207, notes 21–24) is the
English Reports records at 2 Wms Saund 200 and 205, titled "Same v Bateman". When several cases begin at a cited page and nothing
tells them apart, the pick is always `low`; but a name printed in another
edition's text for the same footnote does tell them apart, at `medium`.
(`english_reports.cases` holds no case text, so a subject can only be judged
from the name and the passage.) `english_reports.cases.word_count`
helps: a case whose record is a hundred words cannot reach a page several
pages on, and a long record starting a page earlier usually holds the notes
printed on the next. Choices already made (2 Saund. 319 n. (6) covers
319 b and 319 c printed without a note number, and 1 Saund. 228 n. (1)
covers 228 a, b and c): 2 Saund. 291 n. (the videlicet)
is Dakin's Case; 12 Mod. 508 is Pell v. Garlick; Salk. 544 is
Stephen v. Arthur; 2 Saund. 62 b, n. (5) is Hodsden v. Harridge; King v. Lyme Regis, Doug. 148, is
the record at 1 Doug 149; Harmer v. Rowe, 2 Chit. Rep. 334 and its
"2 Stark. 36, S. C.", are both 6 M & S 146. "Holt's Inst. 561, S. C." (Hatton v.
Morse) is Holt's K.B. Reports, Holt KB 561, at `medium`; a nearby page
printed or OCR'd (562, 563, 661) is a misprint of 561, not a pin cite;
the choice holds with or without "S. C.". Bridgwater v.
Bythway, 3 Lev. 113, is the record at 3 Lev 113 (17970039), a separate
report of the suit, not a pin cite into 3 Lev 109. Colt v. Bishop of
Coventry (Hob. 140–164) is 17920644, the record with text (titled "De
Termino Sancti Mich", with the case's name in `murrell_title`), not the
empty 17921247. "Anon., 2 Salk. 519" (three Anonymous cases begin there)
is 17950161, at `low`. Buckley v. Rice Thomas (Plowd. 118–128, in Plowden's
missing pages) is Dyer's report, "Bulkeley v Thomas", 2 Dy 113 (17941482),
linked at `medium`: Plow. 118, the report's first page, is not a pin
cite; a later Plowden page (123, 128 a) is. Record each of
these choices at `medium` (unless a pin-cite or name fault makes it `low`),
except where the printed name alone picks the case, which is `high`
(Stephen v. Arthur, Salk. 544).
A second report of a case found only inside a different case's report
(Saunders v. Hussey, "Ld. Ray. 333, S. C.", inside Silly v. Dally; Price
v. Brown, "1 Wils. 116, S. C.", inside Alcorn v. Westbrook) is `no_match`
at `low`. Ignore a second
number printed after a cite (`2 Ld. Raym. 882, 883`, `1 N. R. 64. 233`):
record the citation once, at its first page; such numbers are often OCR
noise. Always list the numbers you dropped in `note` (`dropped: 508, 532`),
so they can be recovered. But where another edition prints a case name at
that second number ("8 Rep. 39. 59": Griesley's Case and Beecher's Case),
it is a second citation; a second number that is only another record of the
same case (2 Vent. 198, 222, Humphreys v. Bethily) stays one citation. A note on the same page (`2 Saund. 410, and note (3)`, or `97, and 97 b,
n. (2)`: a folio letter is the same page) is one record. The
exception is a second page carrying its own note
(`2 Saund. 116 b. 411, n. (4)`): that is a second citation. When a page
number is cut off and printed on no page you have, supply it from the case
found by name and say so. A cite repeated on a page, or the same case cited in two footnotes, is
recorded each time it is printed. When
the English Reports split one case into several records, link the record the
cited page falls in; when the case begins in an earlier record, the cite is
`pincite: true`, even at the later record's first page (Craft v. Boite at
1 Saund. 246, Osborne v. Rogers at 267).

**Identify the reporter.** `legalhist.whitelist` maps printed spellings
(`reporter_found`) to a `reporter_standard`, and `legalhist.reporters` gives
its title, jurisdiction (`us:ny`, `uk:kb`, ...) and years. These are help,
not authority: a reporter must fit the period. `N. R.` in a book of 1831 is
Bosanquet and Puller's New Reports (1804–07), not the New Reports of 1862–65.
The edition's year is a ceiling: no case it cites is later.

Some English reporters have no source here, so their cites are `no_match`
unless the same case is cited through another reporter: Willes's Reports
(`Willes`), Marshall's Common Pleas Reports (`Marsh.`, 1813–16; the
whitelist's Kentucky reading is wrong in an English context; likewise its
`Dowl.`, read as Dow's Parliamentary Cases, is usually Dowling's Practice
Cases, which are in the stubs as `3 Dowl. 70`), and the year
books. Lutwyche (`Lutw.`) and Chitty's King's Bench Reports (`Chit.`) are in
the stubs only, and so, at most, are the practice and later reporters common
in the 1867 and later editions' notes: Dowling (`Dowl.`, `Dowl. N.S.`),
Dowling and Lowndes, Dowling and Ryland (in Barnewall and Cresswell),
Perry and Davison, Gale and Davison, Scott (C.P.) and Scott N.R.,
Harrison and Wollaston, Moore and Payne,
Hodges, Nevile and Perry, the Law Journal and the Jurist. Find the same case
in the English Reports by name first; it wins over a stub even when the
stub matches the cite exactly. If two English Reports records of that name
fit, pick one by year and subject and link it at `low`; do not fall back to
the stub. Willmore, Wollaston and Davison (`Will. W.
& D.`, 1837–38) and Gale's Exchequer Reports (`Gale`, 1835–36) have no
source and no stubs. A bare `Ibid.` with no page repeats the previous cite, and is not recorded
when that cite is a treatise. When it follows a footnote holding a single
case, it is recorded as a citation of that case. A bare `Id.` is read the
same way.
When the cited reporter has no source but the same case is reported in one
that does (Willes's case found in Barnes or Strange by name), link that
report at `medium` and say so in `note`: the link is to the case. The same
goes when the reporter is in a source but the cited pages are missing
(Plowden 1–130, Benloe 44–88). This
covers J. B. Moore (found in Taunton), Tyrwhitt (in Crompton and Meeson, or
Crompton and Jervis), Marshall (Taunton) and Willes (often in Barnes's
Notes, Comyns or Kenyon). Benloe has no records for pages 44–88. A case known only as cited in
another report (`Johnson v. Picket, cited Grimwood v. Barritt, 6 T. R. 463`)
is one record, linked to the report that prints it; but a case cited in a
note of another report that also has a report of its own is linked to its
own report.

**English Reports** (`english_reports.cases`). `er_parallel_cite` is the
nominate cite without punctuation: `10 Mod 88`, `2 B & C 871`, `4 Taunt 572`,
`3 Bos & Pul 453`, `2 TR 1`, `1 Ld Raym 716`, `1 Salk 180`, `4 Co Rep 62` (folio letters `a`/`b` are
dropped), `Dick 500` (reporters paged as one run have no volume). Bosanquet
and Puller's New Reports are mostly stored under `Bos & Pul` too, so that
`1 Bos & Pul 64` may be the 1797 volume or the 1804 New Reports: tell them
apart by `er_year` and the name. Look across reporters by name when a cite
misses: the same case is often reported in several. Some reporters carry a
court suffix: `3 Wils KB 558`, Saunders is `1 Wms Saund 73`, Yelverton
`Yel`, Hardres `Hard`, Barnewall and Alderson `B & A`, and some carry a
volume the printed cite never shows (`2 Cowp`, `3 Dy`, `1 Plowden`); Rep.
temp. Hard. is `Cas T H`, Sir T. Raymond `T Raym`, Leonard `3 Leo 205`, Holt's Nisi Prius plain `Holt` (King's
Bench is `Holt KB`). Plowden's first 130 pages
are missing. Dalison is stored as `2 Ben & D`
(the 1689 Benloe and Dalison, 123 E.R. 225–315; `1 Ben & D` is that edition's
Benloe, paged differently from the 1661 `Benl` Stephen cites). Keilwey,
Benloe and Dalison are stored as groups of cases by term: link the group whose pages contain
the cite, at `medium`, `Show KB`, `Freem KB`, `Holt KB`, `Moo KB`,
`Barn KB`; when an exact cite misses, match the volume and the start of the
reporter's name (`er_parallel_cite ~ '^3 Wils'`) to see how it is stored.

```sql
-- exact cite, then the reporter-volume to find the case a pin cite falls in
SELECT id, er_parallel_cite, er_name, er_year FROM english_reports.cases
 WHERE er_parallel_cite = '4 Taunt 572';
SELECT id, er_parallel_cite, er_name, er_year FROM english_reports.cases
 WHERE er_parallel_cite ~ '^3 Bos & Pul [0-9]+$'
   AND (regexp_match(er_parallel_cite, '([0-9]+)[a-z]?$'))[1]::int BETWEEN 440 AND 458
 ORDER BY (regexp_match(er_parallel_cite, '([0-9]+)[a-z]?$'))[1]::int;
SELECT id, er_parallel_cite, er_name, er_year FROM english_reports.cases
 WHERE er_name ILIKE '%dumsday%';          -- by name, when the cite is garbled
```

**CAP** (`cap.cases`, reporters in `cap.reporters`, cites in `cap.citations`).

```sql
SELECT c.id, c.name_abbreviation, c.decision_year, c.first_page, c.last_page
  FROM cap.citations ci JOIN cap.cases c ON c.id = ci."case"
 WHERE ci.cite = '5 Johns. 123';
-- a pin cite: the case in that volume whose pages contain it
-- (cap.cases.volume is a barcode; the volume number is in cap.volumes)
SELECT c.id, c.name_abbreviation, c.decision_year, c.first_page, c.last_page
  FROM cap.cases c
  JOIN cap.volumes v ON v.barcode = c.volume
  JOIN cap.reporters r ON r.id = c.reporter
 WHERE r.short_name = 'Wend.' AND v.volume_number = 10
   AND c.first_page <= 112 AND coalesce(c.last_page, c.first_page) >= 112;
-- by name, across a reporter
SELECT c.id, c.name_abbreviation, c.decision_year, ci.cite
  FROM cap.cases c JOIN cap.citations ci ON ci."case" = c.id
 WHERE c.name_abbreviation ILIKE '%sackrider%' AND c.decision_year <= 1841;
```

A citation that falls in a later record of a case the English Reports split
in two is a `pincite` when the case itself begins earlier, even if the page is
that record's first.

CAP's `first_page` sometimes disagrees with the printed cite, notably in
early Johnson's and Massachusetts Reports (Hopkins v. Hopkins is cited
`10 Johns. 369` but has `first_page` 375): when a pin-cite lookup by
`first_page`/`last_page` misses, look in `cap.citations` for cites near the
page, and by name.

American cases in the later editions (Andrews, 1894, and after):

- Each printed parallel cite is its own citation. One to a series CAP
  does not hold as a reporter (American Decisions, American Reports,
  American State Reports, L.R.A., the Lawyers' Edition (`L. ed.`), a
  regional reporter not in
  `cap.citations` for that case) links the same case's official report in
  CAP at `medium`, as a report with no source does; one CAP holds links
  directly. A reprint in a periodical (American Law Register) is treated
  the same way. Reprints of English cases (Eng. C. L., English Ruling Cases)
  likewise link the case's own report in the English Reports at `medium`.
- A recent case cited only by regional reporter (`37 N. E. Rep. 136`) is
  found by name and year in CAP, usually under its official report. One CAP
  does not hold at all (an unofficially reported Texas Civil Appeals case)
  is `no_match`, `high` when legible and named.
- To find the same passage in another edition when pageids don't line up,
  search `moml.page_ocrtext` for that psmid with a distinctive case name.
- When the volume-join pin-cite query misses (some reporters' volumes are
  not joined), search `cap.citations.cite` with a pattern on the reporter
  and volume, then check pages.
- A cite one page past CAP's `first_page` (common in Ill. App.) is a pin
  cite into that case.
- CAP's own cite strings are sometimes wrong (Allen v. Patterson stored as
  `7 N.Y.3d 476`): trust the name, year and pages, and say so in `note`.
- Where CAP holds one case twice (69 Hun 489 as its own record and as a
  cite on an N.Y.S. record), link the record of the cited reporter; where
  two records share one cite (Steinmetz v. Currey, 1 Dall. 234), link the
  one whose `first_page` matches.
- `id.` carries across the two columns of an editor's notes: it is the
  reporter last cited in the notes' reading order.
- American treatises, casebooks and dictionaries are not reports: Bouvier,
  Burrill, Ames's Cases, Leading Criminal Cases, Puterbaugh, Thompson on
  Trials, and the like.
- `id.` standing for a reporter (`110 id. 588` after a U.S. cite) is that
  reporter. "Heretofore cited" and "stated ante" pointing to a cite pages
  away are read like `supra`, at `medium`.
- Not recorded: an annotation cited without its case ("48 L. R. A. 177,
  note"), and a case cited only to this book's own Appendix or "supra" with
  no report printed anywhere on the page.
- A reprint's "own report" may be a stub when the English Reports lack the
  case (Harrop v. Hirst, `4 L.R. Exch. 43`).
- CAP files some series under other names: Kernan is N.Y. 11–14 (2 Kern. =
  12 N.Y.); Hun is "N.Y. Sup. Ct." (2 Hun =
  9 N.Y. Sup. Ct.); the circuit-court reporters (Story, Paine, Washington
  C.C.) are in F. Cas., with cites such as `1 Paine, 422`. A case cited
  only in L.R.A. links its official report at `medium`.
- When the printed name and CAP's differ (Jacobson v. Miller is CAP's
  Gunzberg v. Miller), search by volume and page; the name still decides
  between cases there.
- Smith's Leading Cases is not recorded, even when cited under a case name
  ("Cutter v. Powell, 2 Smith's L. C.").
- When several cases begin at one CAP cite (19 How. 393), the name decides.
  When a printed cite is to a different real case of the same name (Wilson
  v. N.Y. & N.H. Ry., 18 R.I. 491 for 598), link the one the passage
  describes, at `medium`, with the printed numbers in `note`.
- In some CAP volumes (16 N.Y., 3 Mass.) `first_page` is offset from the
  cites throughout: judge pin cites against `cap.citations` there.
- A case named in the text with its cite in a footnote is one citation.
  Cases cited inside a quoted opinion are recorded.
- CAP's `first_page` can disagree with the printed cite in any reporter
  (Gardner v. Clark, 21 N.Y. 399, `first_page` 428), not only early Johnson's
  and Massachusetts: look in `cap.citations` and by name.
- The edition's own table of cases is a good source for a page number the
  OCR lost or garbled; say so in `note`.
- A number a later edition misprints where earlier editions print it
  right (2 Wils. 145 for 143) follows the earlier editions: `high` when
  the name is printed and unambiguous, else `medium`.
- A printed page that falls inside the named case is kept as printed, a pin
  cite, whether every edition prints it or a later editor changed it
  (Coare v. Giblett, 4 East 90 for 85). `page` is corrected only when the
  printed number lies outside the named case.
- A reprint cite (Eng. C. L., Am. Dec.) keeps the reprint's own page in
  `page`; the link goes to the case's own report.

**Code reporter** (`legalhist.code_reporter`, New York code cases): match
`official_citation` or `parallel_citation`, and `name`.

**Stubs** (`legalhist.stub_cases`): Lutwyche, for one, is only here, split
into `1 Lutw.` and `2 Lutw.` though the book is paged as one run.
Stubs are cases in reporters no source covers, known only
by their cite (`cite` is `<volume> <reporter_standard> <page>`, e.g.
`11 W.R. 813`). Link a stub only when it is exactly the case's first page; a
pin cite into a stub reporter is `no_match`.

Prefer, in order (and an English Reports report of the same case, found by
name, over an exact stub in another reporter: Tyrwhitt, the Law Journal
(`L. J.`), the Jurist (`Jur.`), Perry and Davison and the like): CAP or the English Reports (whichever is the reporter's
source), the code reporter, a stub. If nothing fits, `no_match`, saying in
`note` what you tried.

### The record

Write one file per page to `llm-reader/results/<psmid>/<pageid>.json`:

```json
{
  "psmid": "...", "pageid": "01000",
  "model": "claude-opus-5-5", "read_at": "2026-11-20T02:30:00Z",
  "note": "optional, e.g. table of cases",
  "citations": [
    {"seq": 1, "raw": "Rush v. Seymour, 10. Mod.. 88", "case_name": "Rush v. Seymour",
     "volume": 10, "reporter_abbr": "Mod.", "page": 88,
     "status": "linked_english_reports", "er_case_id": "17953003", "confidence": "high"},
    {"seq": 2, "raw": "Dumsday v. Hughes, 3 Bos. & Pul. 458", "case_name": "Dumsday v. Hughes",
     "volume": 3, "reporter_abbr": "Bos. & Pul.", "page": 458,
     "status": "linked_english_reports", "er_case_id": "18030628", "pincite": true,
     "confidence": "high", "note": "case begins at 453"}
  ]
}
```

- `seq` numbers the citations 1, 2, 3 in the order printed: text, then
  footnotes by their letter, then an editor's numbered notes (a note
  continued from the previous page included). Where the scan's text interleaves the lines of
  two footnotes, or puts whole footnotes out of letter order, follow the
  letters and say so in `note`.
- A case found by its printed name at a corrected page (Richley v. Proone,
  printed 386, at 286) is `high` when the name is unambiguous: the name
  decides, and `page` holds the corrected number.
- Earlier editions' records are a cross-check, but some predate the rules
  here; where they differ from this README, follow the README.
- For a parallel cite linked to another reporter's record, leave
  `pincite` unset when you cannot tell whether the cited page is the first
  of its own report.
- `pincite` is false when the cite is the case's first page in another
  report and the link goes to its record in the English Reports' reporter
  (Harmer v. Rowe, 2 Chit. Rep. 334, linked to 6 M & S 146).
- An Anonymous is an unambiguous name when it is the only Anonymous
  beginning at that page.
- A `no_match` to a reporter with no source (Willes, Tyrwhitt; a reporter
  held only as stubs, like Lutwyche, counts as one), or to a
  case missing from one that has a source (Denison v. Richardson,
  14 East 291), is graded by
  how surely the cite was read: `high` when legible with a name printed,
  `medium` when legible without one. Year books, which are cited without
  names, are `high` whenever legible. When another edition prints a
  different regnal year, reign or folio, keep this edition's reading, at
  `medium` unless most editions agree with it, and note the variant. A
  reading that is only this scan's OCR garbling (46 for 45) is not a
  variant: take the legible text of another copy, at `medium`. A cite
  marked "S. P." (same point) is recorded like any other. A
  supplied volume or reporter makes a `no_match` at most `medium` (a rule
  that sets `low` still holds; a reporter carried over from the cite
  before counts as supplied); the volume of a single-volume reporter
  (Plowden, stored as `1 Plowden`) is not "supplied".
- Where the English Reports hold two records of one case at one cite
  (Hartford v. Jones, 2 Salk. 654), link the one with a `word_count`; if
  both have one, the longer, at `medium`.
- A series cited as numbered straight through ("17 Martin" for 5 Mart.
  (n.s.)) takes the series' own volume in `volume`, with the printed one in
  `note`. A page supplied for a cite printed without one is `high` when the
  name is unique in that volume, `medium` otherwise.
- Lutwyche is paged as one run, so the stub's `1 Lutw.` or `2 Lutw.` follows
  from the page and is not "supplied": a bare `Lutw. 1592` matching
  `2 Lutw. 1592` is an exact match.
- A cite printed without a name is `medium` even when it matches a stub
  exactly; another edition's or a reprint's name for it may decide among
  several cases at one page.
- A stub link is `high` when the printed cite matches the stub exactly,
  `medium` when you supplied the volume or corrected a number.
- A year book printed with a folio and a plea number (`10 Hen. 6, 7, pl.
  21`) keeps the folio as `page` and the plea in `note`. The Liber
  Assisarum (`22 Ass. 41`) is cited by plea: `page` is empty and the plea
  goes in `note`.
- `raw` is the citation as printed (with the case name if printed); `case_name`,
  `volume`, `reporter_abbr` (as printed, corrected), `page` and `year` (when
  printed) are your reading of it. `case_name` is left out when no name is
  printed for the cite; a name printed at the head of a string of parallel
  reports, or before `S. C.`, counts for every cite of that string. A volume you supplied goes in `volume`, and
  `note` says so; likewise `volume` and `page` hold the corrected reading when
  the named case is found at other numbers than those printed (which stay in
  `raw`).
- `status` is `linked_cap`, `linked_english_reports`, `linked_code_reporter`,
  `linked_stub` or `no_match`, with exactly the matching one of
  `cap_case_id` (number), `er_case_id` (string), `code_reporter_id` (number),
  `stub_cite` (string), and none for `no_match`.
- `pincite` is true when the cited page is inside the case, not its first page.
- `confidence` is `high` (name and cite agree), `medium` (one is uncertain, or
  no name is printed), or `low` (a guess worth recording).
- `note` explains anything not obvious: an OCR correction, a choice between
  candidates, why nothing was found.

Write valid JSON and nothing else in the file. Check it with
`bin/llm-reader validate --db` before reporting back.
