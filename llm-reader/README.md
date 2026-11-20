# llm-reader

Claude reads MOML pages one at a time and links the cases they cite, by its
own judgement (issue #439). It starts with the editions of Stephen on Pleading
(work 1107): the twelve U.S. editions, then the six UK ones. Nothing here uses
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
- A case cited by name with no reporter (`as in Slade's case`) is not
  recorded unless a reporter cite accompanies it somewhere on the page.
- Year books (cited directly or through an abridgment, `Vin. Ab. ... cites
  15 Hen. 7. 10`), e.g. (`2 Ed. 3. 1`, `Yearb. 21 H. 6. 9`, `22 Ass. 41`) are reports
  of cases with no source here: always record them, as `no_match`, leaving
  out `page` when the folio cannot be read. Write `reporter_abbr` as `Y.B.`
  and the regnal year as `volume`, with the reign in `note` (`10 Edw. III`).
  A year book cited only by an editor's page (`Mayn. 109`) leaves `volume`
  empty. A year book named with no folio (`the Year-books, viz. 1 Ed. 2`) is not
  recorded. Records of pleas (Placitorum Abbreviatio) are not reports.
- Reports that are also books of entries (`Lutw.`) are reports: record them.
  Books of precedents only (Lilly's Entries, Modern Entries, Rastall,
  Coke's Entries, Wentworth) are not.
- Not cases: treatises and abridgments (Tidd, Co. Litt., Com. Dig., Bac. Abr.,
  Vin. Abr., Arch. Pract., Chit. Pl., Selw. N. P., 2 Inst., Booth, and the
  practice books Richardson's K. B. and Manning's Exchequer), statutes
  (`4 Ann. c. 16`), rules of court even when printed in a report
  (`6 Bing. 802`), cross-references (`supra, p. 32`). Skip them.
- Tables of cases and indexes list case names with the treatise's own page
  numbers, not citations: record the page with `"citations": []` and a note,
  even when an entry carries a report to tell Anonymous cases apart
  (`Anonymous, (2 Salk. 519)`).
  So too title pages, advertisements, blank pages.
- Footnote markers (`(q)`, `(r)`) and running heads are not citations.
- `Ib.`, `Id.`, `ubi supra`, `S. C.` (same case) and a bare `Ibid. 233` refer back to the
  previous cite: resolve them to the volume and reporter they stand for and
  record them, noting the expansion in `note`.
- When a citation is cut off at the bottom of the page and continues on the
  next, record it on the page where the reporter and page number are printed;
  run `bin/llm-reader page` on the neighbouring page if you need to see it.

### Reading the OCR: the name leads, the numbers are suspect

This is OCR of old print, and the numbers are where it fails most: volumes and
pages are misread (`I`, `l` for 1; `S` for 5 or 8; `O` for 0; `3` for 8;
dropped or doubled digits), and a stray number after a cite is as likely to be
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

Record `raw` exactly as printed, OCR errors and all.

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
page, unless its subject plainly belongs to the one beginning there. Choices
already made: 1 Saund. 318 n. (3) is Redman v. Edolph; 1 Saund. 228 n. (1)
is Stennel v. Hogg. When several cases begin at a cited page and nothing
tells them apart, the pick is always `low`. Ignore a second
number printed after a cite (`2 Ld. Raym. 882, 883`, `1 N. R. 64. 233`):
record the citation once, at its first page; such numbers are often OCR
noise. Always list the numbers you dropped in `note` (`dropped: 508, 532`),
so they can be recovered. The exception is a second page carrying its own note
(`2 Saund. 116 b. 411, n. (4)`): that is a second citation. When a page
number is cut off and printed on no page you have, supply it from the case
found by name and say so. A cite repeated on a page, or the same case cited in two footnotes, is
recorded each time it is printed. When
the English Reports split one case into several records, link the record the
cited page falls in.

**Identify the reporter.** `legalhist.whitelist` maps printed spellings
(`reporter_found`) to a `reporter_standard`, and `legalhist.reporters` gives
its title, jurisdiction (`us:ny`, `uk:kb`, ...) and years. These are help,
not authority: a reporter must fit the period. `N. R.` in a book of 1831 is
Bosanquet and Puller's New Reports (1804–07), not the New Reports of 1862–65.
The edition's year is a ceiling: no case it cites is later.

Some English reporters have no source here, so their cites are `no_match`
unless the same case is cited through another reporter: Willes's Reports
(`Willes`), Marshall's Common Pleas Reports (`Marsh.`, 1813–16; the
whitelist's Kentucky reading is wrong in an English context), and the year
books. Lutwyche (`Lutw.`) and Chitty's King's Bench Reports (`Chit.`) are in
the stubs only. A bare `Ibid.` with no page repeats the previous cite.
When the cited reporter has no source but the same case is reported in one
that does (Willes's case found in Barnes or Strange by name), link that
report at `medium` and say so in `note`: the link is to the case. The same
goes when the reporter is in a source but the cited pages are missing
(Plowden 1–130, Benloe 44–88). This
covers J. B. Moore (found in Taunton), Tyrwhitt (in Crompton and Meeson, or
Crompton and Jervis), Marshall (Taunton) and Willes (often in Barnes's
Notes, Comyns or Kenyon). Benloe has no records for pages 44–88. A case known only as cited in
another report (`Johnson v. Picket, cited Grimwood v. Barritt, 6 T. R. 463`)
is one record, linked to the report that prints it.

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
are missing. Keilwey and Benloe
are stored as groups of cases by term: link the group whose pages contain
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

**Code reporter** (`legalhist.code_reporter`, New York code cases): match
`official_citation` or `parallel_citation`, and `name`.

**Stubs** (`legalhist.stub_cases`): Lutwyche, for one, is only here, split
into `1 Lutw.` and `2 Lutw.` though the book is paged as one run.
Stubs are cases in reporters no source covers, known only
by their cite (`cite` is `<volume> <reporter_standard> <page>`, e.g.
`11 W.R. 813`). Link a stub only when it is exactly the case's first page; a
pin cite into a stub reporter is `no_match`.

Prefer, in order: CAP or the English Reports (whichever is the reporter's
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

- `seq` numbers the citations 1, 2, 3 in the order printed.
- `raw` is the citation as printed (with the case name if printed); `case_name`,
  `volume`, `reporter_abbr` (as printed, corrected), `page` and `year` (when
  printed) are your reading of it. A volume you supplied goes in `volume`, and
  `note` says so.
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
