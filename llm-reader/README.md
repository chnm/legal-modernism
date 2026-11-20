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

```sh
go build -o bin/llm-reader ./llm-reader/          # from the repository root
export LAW_DBSTR="$LAW_CLAUDE"                    # read-only is enough except for load
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
- Not cases: treatises and abridgments (Tidd, Co. Litt., Com. Dig., Bac. Abr.,
  Vin. Abr., Arch. Pract., Chit. Pl., Selw. N. P., 2 Inst.), statutes
  (`4 Ann. c. 16`), year books may be recorded as `no_match`, cross-references
  (`supra, p. 32`). Skip them.
- Tables of cases and indexes list case names with the treatise's own page
  numbers, not citations: record the page with `"citations": []` and a note.
  So too title pages, advertisements, blank pages.
- Footnote markers (`(q)`, `(r)`) and running heads are not citations.
- `Ib.`, `Id.`, `S. C.` (same case) and a bare `Ibid. 233` refer back to the
  previous cite: resolve them to the volume and reporter they stand for and
  record them, noting the expansion in `note`.
- When a citation is cut off at the bottom of the page and continues on the
  next, record it on the page where the reporter and page number are printed;
  run `bin/llm-reader page` on the neighbouring page if you need to see it.

### Reading the OCR

Expect errors: `I` or `l` for 1, `S` for 5, `O` for 0, broken words, stray
punctuation, hyphenation across lines. Correct obvious errors, record `raw`
exactly as printed, and say what you corrected in `note`. When a cite cannot
be read with reasonable confidence, record it as `no_match` with a low
confidence and a note.

### Looking the case up

Query with `psql "$LAW_CLAUDE"`, read-only. The case name is the check: link
only when the case found is plausibly the one named (spelling, abbreviation
and order of parties vary; `Rex v.` = `The King against`, `Doe d. A v. B`,
`ex parte`). When the page gives no name, a hit at the exact first page is
enough at `medium` confidence.

**Identify the reporter.** `legalhist.whitelist` maps printed spellings
(`reporter_found`) to a `reporter_standard`, and `legalhist.reporters` gives
its title, jurisdiction (`us:ny`, `uk:kb`, ...) and years. These are help,
not authority: a reporter must fit the period. `N. R.` in a book of 1831 is
Bosanquet and Puller's New Reports (1804–07), not the New Reports of 1862–65.
The edition's year is a ceiling: no case it cites is later.

**English Reports** (`english_reports.cases`). `er_parallel_cite` is the
nominate cite without punctuation: `10 Mod 88`, `2 B & C 871`, `4 Taunt 572`,
`3 Bos & Pul 453`, `2 TR 1`, `1 Ld Raym 716`, `1 Salk 180`, `4 Co Rep 62` (folio letters `a`/`b` are
dropped), `Dick 500` (reporters paged as one run have no volume). Bosanquet
and Puller's New Reports are mostly stored under `Bos & Pul` too, so that
`1 Bos & Pul 64` may be the 1797 volume or the 1804 New Reports: tell them
apart by `er_year` and the name. Look across reporters by name when a cite
misses: the same case is often reported in several.

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
SELECT c.id, c.name_abbreviation, c.decision_year, c.first_page, c.last_page
  FROM cap.cases c JOIN cap.reporters r ON r.id = c.reporter
 WHERE r.short_name = 'Johns.' AND c.volume = '5'
   AND c.first_page <= 130 AND coalesce(c.last_page, c.first_page) >= 130;
```

**Code reporter** (`legalhist.code_reporter`, New York code cases): match
`official_citation` or `parallel_citation`, and `name`.

**Stubs** (`legalhist.stub_cases`): reporters no source covers, known only
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
  printed) are your reading of it.
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
