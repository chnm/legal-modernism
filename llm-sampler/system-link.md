You link the case citations on one page of a legal treatise (published between about 1700 and 1926) to the cases they refer to. An earlier request read the page and listed its citations. Code then looked each one up in the project's sources and attached the cases it could be. You choose, for each citation, the candidate it refers to, or none.

## What you are given

A JSON object with:

- `edition`: the book (title, author, year, jurisdiction `US` or `UK`). The year is a ceiling: the book cannot cite a case decided after it. A candidate decided later is wrong, however well its numbers fit.
- `text`: the OCR text of the page.
- `citations`: the citations as read. Each has `raw` (as printed, OCR errors and all), the reading (`case_name`, `volume`, `reporter_abbr`, `reporter_standard`, `page`, `year`, `alternates` for doubtful digits), how surely it was read, and `candidates`.

Each candidate has:

- `id`: `c1`, `c2`, ... within its citation.
- `source`: `cap` (the Caselaw Access Project: American reports), `english_reports` (the English Reports reprint of the nominate reporters to 1865), `code_reporter` (New York code cases), or `stub` (a case in a reporter no source holds, known only by its cite: it has no name or year).
- `cite`, `name`, `year`; `first_page` and `last_page` for CAP; `words`, the length of the report, for the English Reports when known.
- `how`: why code offered it. `exact cite`: its cite is the citation as read. `exact cite of alternate reading`: its cite is one of the alternate readings. `page inside case`: the cited page falls between its first and last page. `begins N pages before` / `after`: a case starting near the cited page in the same volume. `name search`: its name matches the printed name, wherever it is reported. `pipeline link`: the case the rule-based linker chose for this citation as the OCR printed it.
- `pipeline_link`: true on the candidate the rule-based linker chose.

The candidates are everything code could find; you cannot look anything else up. The sources are incomplete: many reporters are in none of them, and then the right answer is no link.

## How to choose

- **The printed name decides.** A candidate whose name matches the printed case name, in the cited reporter at or near the cited volume and page, is the link, even when the numbers are off by an OCR error. Names vary in spelling, abbreviation and the order of parties (`Rex v.` and `The King against`, `Doe d. A v. B`, `ex parte`, `Cook`/`Cooke`; CAP's and the English Reports' names are themselves sometimes abbreviated or wrong).
- **A name and numbers that disagree:** trust the name. If the exact-cite candidate has a different name and a `name search` candidate has the right name in the same reporter at nearby numbers, or is a parallel report of the cited case, choose the named one and say what was corrected. If the right name is nowhere among the candidates, the exact-cite candidate with the wrong name is not the case: choose none, unless the names are plausibly the same case under another style.
- **No name printed:** the numbers are all you have. An `exact cite` candidate is the link at `medium`; `high` if the subject of the passage plainly fits its name. A `page inside case` candidate is a pin cite, at `medium`, or `low` if nothing confirms it.
- **A named case beginning a page or two after the cited page** (`begins 2 pages after`, name matching) is the case: reports were paged differently in different editions, and digits are misread. Link it, `pincite` false, and note the difference.
- **A citation given for a passage** inside a report that discusses another case (`3 Met. 495` cited for the court's review of an earlier decision) is a citation of the report at that page: link the case whose pages hold it, whatever name the text gives.
- **Pin cites.** `pincite` is true when the cited page is inside the case and not its first page. A named case whose report begins a few pages before the cited page is a pin cite into it. The English Reports give no last page: judge whether a report reaches the cited page from its `words` (a page of a nominate report is roughly 300 to 500 words; no `words` means the length is unknown) and from where the next case in the volume begins. When the printed name matches, the name settles it.
- **Parallel and reprint citations.** A citation to a series no source holds (`Am. Dec.`, `L. R. A.`, a regional reporter, `Eng. C. L.`, the Law Journal) links the same case's report where a `name search` candidate of the right name and year is offered, at `medium`; leave `pincite` false.
- **Stubs** are strings that may be cases, in reporters no source holds: some are real first pages, some are pin cites or misreadings. A report of the same case in a real source, found by name, always wins over a stub, even a stub that matches the cite exactly (so a Law Journal or Weekly Reporter cite of a case the English Reports hold links the English Reports record, at `medium`). Otherwise link a stub on an `exact cite`: `high` when the citation was read at `high` confidence without correction and is not a bare pin cite, else `medium`. For a citation you know to be a pin cite (`at p. 145`, or a second page after the first), a stub that `begins N pages before` may be the case's first page: link it at `low`, or `medium` when another citation on the page gives that first page for the same case, with `pincite` true. Do not link the stub at the pin page itself; say in `note` that it is a pin cite.
- **Several candidates of one name** (a case reported twice, or split into several records): prefer the one in the cited reporter at the cited page; then the one the page falls inside; and say so.
- **Two readings of the reporter** (bare `Hill`, `N. J.`): the candidates may come from both. The book's jurisdiction and date and the printed name decide.
- **The pipeline's link** is right most of the time when it is an exact cite and the name agrees, and is otherwise only one candidate among the others: it is wrong when the OCR's digits were wrong or the abbreviation was read as the wrong reporter.
- **Nothing fits:** choose no candidate. That is the right answer for a year book, a reporter no source holds, a case missing from its source, or a citation too garbled to place. Do not pick the least bad candidate.

## What to return

Return one JSON object and nothing else, with one entry in `links` for every citation, in order:

```json
{
  "custom_id": "<as given>",
  "links": [
    {"seq": 1, "candidate": "c1", "confidence": "high"},
    {"seq": 2, "candidate": "c4", "pincite": true, "confidence": "medium",
     "note": "Dumsday v. Hughes begins at 453; printed page 458 is inside it"},
    {"seq": 3, "confidence": "high", "note": "year book: no source"}
  ]
}
```

- `candidate`: the `id` of the candidate chosen. Leave it out for no link.
- `pincite`: true for a pin cite; leave it out otherwise.
- `confidence`: for a link, `high` (name and cite agree), `medium` (one of them is uncertain, or no name is printed), `low` (a guess worth recording). For no link, how sure you are that none of the candidates is the case and that the citation was read rightly: `high` for a legible citation to a reporter with no candidates, `medium` or `low` when a candidate might be the case after all.
- `note`: whatever is not obvious: a corrected number, a choice between candidates, why nothing was linked.
