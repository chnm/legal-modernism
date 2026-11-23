You read one page of a legal treatise published between about 1700 and 1926, as OCR text, and return every citation of a reported **case** printed on it. A second request, which you do not see, will link each citation you return to the case it refers to, so read the citations as a legal historian would: carefully, and correcting what the OCR got wrong.

## What you are given

A JSON object with:

- `edition`: the book (title, author, year, jurisdiction `US` or `UK`, subjects). The year is a ceiling: nothing on the page cites a case decided later.
- `page`: the volume and page identifiers, and the printed page number when known.
- `text`: the OCR text of the page. This is the only page you report on.
- `previous_page_tail` and `next_page_head`: the end of the page before and the start of the page after, for context only: to complete a citation cut off at the page break, to resolve an `Ibid.` at the top of the page, or to see the sentence a footnote supports.
- `pipeline_detections`: what a regular-expression detector found on this page, and what a rule-based linker made of each: its reading of the reporter, whether it linked a case (`status`, `tier`, `linked`). Treat it as a careful but literal-minded assistant. It finds most volume-reporter-page strings, including some that are not cases (statutes, treatises, noise); it reads digits exactly as the OCR printed them; it misses citations with unusual spacing, citations broken across lines, year books, `Ibid.` and `Id.`, and reporters it does not know. A detection that linked to a case whose name matches the name printed on the page is strong evidence that the numbers were read correctly; a `no_match` on a legible citation often means a misread digit or a reporter the sources do not hold.

The table of reporters at the end of these instructions lists every reporter the project knows: its standard abbreviation, title, jurisdiction and years.

## What to return

Return one JSON object and nothing else:

```json
{
  "custom_id": "<as given>",
  "note": "optional: anything about the page as a whole",
  "citations": [
    {"seq": 1, "raw": "Rush v. Seymour, 10. Mod.. 88", "case_name": "Rush v. Seymour",
     "volume": 10, "reporter_abbr": "Mod.", "reporter_standard": "Mod", "page": 88,
     "pipeline": "same", "confidence": "high"},
    {"seq": 2, "raw": "Sackrider v. M'Donald, IO Johns. 2S3", "case_name": "Sackrider v. M'Donald",
     "volume": 10, "reporter_abbr": "Johns.", "reporter_standard": "Johns.", "page": 253,
     "alternates": [{"volume": 10, "page": 283}],
     "pipeline": "new", "confidence": "medium", "note": "OCR: IO for 10, 2S3 for 253 or 283"}
  ],
  "pipeline_rejected": [
    {"raw": "13 Eliz. c. 5", "reason": "statute"}
  ]
}
```

For each citation:

- `seq`: 1, 2, 3 in the order printed: the text, then the footnotes in their order.
- `raw`: the citation exactly as the OCR prints it, errors and all, with its case name if one is printed, and line breaks collapsed to single spaces.
- `case_name`: the name as you read it, corrected (`Sackrider v. M'Donald`), normalizing `vs.`, `versus`, `against` and `ads.` to `v.`. Leave it out when no name is printed for the citation. A name printed once at the head of a string of parallel reports, or before `S. C.`, belongs to every citation of that string.
- `volume`, `page`, `year`: your reading of the numbers, corrected. `year` only when a year is printed with the citation. Leave `volume` out for a reporter cited without one. A page with a folio letter (`62 a`) is page 62.
- `reporter_abbr`: the reporter as printed, with OCR errors corrected (`Johns.`, `B. & C.`, `Ld. Raym.`).
- `reporter_standard`: the `standard` abbreviation from the table of reporters for the reporter you judge this to be, copied exactly. Leave it out if the reporter is not in the table. Abbreviations are ambiguous and the reporter must fit the book: `Hill` in a New York book is Hill's New York Reports; `N. R.` in an English book of 1831 is Bosanquet and Puller's New Reports, not the New Reports of 1862. Weigh the edition's jurisdiction and year and the reporter's years.
- `parallel_of`: when this citation is another report of the same case as an earlier citation on the page (a parallel report, `S. C.`, a reprint), the `seq` of that earlier citation. Give it the same `case_name` too. A decision in the same litigation at another stage (`affirming 50 Barb. 100`, `reversed, 12 Wall. 1`) is a separate decision with its own report: record it with the same `case_name` but without `parallel_of`. In a `see also` string, or a string of authorities for one proposition, each citation is a different case: a name does not carry forward to them.
- `alternates`: up to three other readings of the numbers, when a digit is doubtful (`3`/`8`, `1`/`I`/`l`, `5`/`S`, `0`/`O`, a dropped or doubled digit). Each is a `volume` and `page`. Leave it out when the numbers are clear.
- `pipeline`: `same` if a pipeline detection has this volume, reporter and page; `corrected` if the pipeline detected this citation but you read its numbers or reporter differently; `new` if the pipeline did not detect it.
- `confidence`: how surely you read the citation: `high` (legible, and you are sure it cites a case), `medium` (a number or the reporter is uncertain), `low` (a guess worth recording).
- `note`: anything not obvious: an OCR correction, an `Ibid.` you expanded, why a reporter was chosen.

`pipeline_rejected` lists each pipeline detection you did not record, with a short reason (`statute`, `treatise`, `not a citation`, `duplicate of seq 3`, and the like). Every pipeline detection must end up either matched by a citation or in this list.

## What counts

Record:

- Every citation of a reported case, named or not: `Rush v. Seymour, 10 Mod. 88`; a bare `4 Taunt. 572`; `Herlakenden's Case, 4 Rep. 62 a`; a citation to a reporter's note on a case (`1 Saund. 28, n. (3)`).
- Each citation of a string separately, and each printed parallel report separately (`5 Johns. 123; S. C. 2 Am. Dec. 40` is two).
- A citation each time it is printed, even when the same case is cited twice on the page.
- `Ib.`, `Ibid.`, `Id.`, `S. C.`, `ubi supra` and a name followed by `supra` when they stand for a case report and you can tell which: resolve them to the volume and reporter they stand for (`Ibid. 233` after `4 Taunt. 572` is `4 Taunt. 233`), and say so in `note`. `110 id. 588` after a citation to a reporter is that reporter.
- Year books and the Liber Assisarum (`2 Ed. 3. 1`, `Yearb. 21 H. 6. 9`, `22 Ass. 41`): `reporter_abbr` `Y.B.`, the regnal year as `volume`, the folio as `page` (left out when cited by plea), and the reign in `note` (`21 Hen. VI`).
- A citation that prints only a page inside the case (`6 Ch. at p. 10`, `37 L. J. at 75`, `4 Wheat. 316, 407` cited for 407 alone): `page` is the page printed, not the case's first page, even if you know it. The lookup that follows finds the case a page falls in; say `pin cite` in `note`.
- A citation whose page number the OCR lost or never printed (`Herring v. Woodhull, 29 Ill.`), or a reporter cited by name alone: record it with `page` left out, and say so in `note`. Do not supply a number from memory; the name will find the case.
- A citation to a reporter's or editor's note on a case (`1 Wms. Saund. 333 a, n. 8`) is a citation of the case it annotates, at that page.
- A citation split across the page break is recorded once, on the page where its page number is printed: a citation whose name and reporter end this page but whose page number is in `next_page_head` belongs to the next page, and one whose page number opens this page (its name in `previous_page_tail`) belongs here.

Do not record:

- Statutes (`13 Eliz. c. 5`, `2 R. S. 301`), constitutions, codes, session laws and rules of court.
- Treatises, abridgments, digests, dictionaries, books of entries and collections of leading cases (`Co. Litt.`, `Bac. Abr.`, `Com. Dig.`, `Vin. Abr.`, `2 Inst.`, `Tidd`, `Chit. Pl.`, `2 Kent Com. 340`, `Story Eq. Jur. § 12`, `1 Greenl. Ev.`, `Smith's Leading Cases`), and periodical articles. Beware abbreviations that are a treatise in one book and a reporter in another. Collections of cases that are in the table of reporters are reports, whatever their titles: Equity Cases Abridged (`Eq. Cas. Abr.`), Select Cases in Chancery, Cases in Chancery.
- A case named with no report anywhere on the page (`as in Slade's case`), an unreported or manuscript case, and cross-references within the book (`supra, p. 32`, `ante, § 4`).
- A court and year given in place of a report (`Vt., 1909`, `1919 (Mass.)`): it names no report. But keep it in the `raw` and `year` of the citation it accompanies.
- Footnote markers, running heads, section numbers, dates, sums of money.
- Entries of a table of cases or an index, which pair case names with the book's own pages: return `"citations": []` with a `note` saying what the page is. So too a title page, an advertisement, a blank or illegible page, and a page with no citations.

## Reading the OCR

The numbers are where OCR fails most: `I`, `l` for 1; `S` for 5 or 8; `O` for 0; `3` for 8; dropped or doubled digits; a semicolon read as a digit (`5415` for `545;`). Case names survive better. Weigh the whole page: the same case cited twice, a string of citations to one reporter in volume order, the reporter's period against the book's date. Give the reading you think is right in `volume` and `page`, the others in `alternates`, and keep `raw` as printed. Where the OCR has interleaved the lines of two columns or two footnotes, so that a citation's parts are scattered (`2 P. Wms. ... 495`), put the citation back together: `raw` is its parts as printed, joined with ` ... `, and `note` says so. A printed variant the table does not list (`Pr.` for Price, `Br.` for Brown's Chancery Cases, `Mad.` for Maddock) is matched to its reporter by your own knowledge, and the reporter must suit the book's subject and court as well as its date. A second number after a citation (`2 Ld. Raym. 882, 883`) is usually a further page of the same case or noise: record the citation once, at its first page, and list what you dropped in `note`. The exception is a run of numbers each of which plainly cites a different case, as a digest or a note listing a reporter's cases does (`3 C. H. R. 3, 70, 118; 4 Ib. 33, 52`): record each as its own citation. If the edition's year is contradicted by the page itself (it cites cases with later dates printed), say so in the page `note` and do not use the year as a ceiling.

Reprints and unofficial series are cited as reports and are recorded like any other: `Am. Dec.`, `Am. Rep.`, `Am. St. Rep.`, `L. R. A.`, `Eng. C. L.`, the regional reporters (`N. E.`, `N. W.`, `Atl.`, `Pac.`, `S. W.`, `S. E.`, `So.`), the Law Journal, the Jurist, the Weekly Reporter, the Law Times.

## The table of reporters

One reporter per line: standard abbreviation | the spellings it is most often printed under in the treatises, commonest first | title | jurisdiction | years. The printed spellings are a guide, not a rule: several reporters share a spelling (`Moo.` is Moody's Crown Cases, J. B. Moore's Common Pleas Reports or Sir Francis Moore's King's Bench Reports), and the book's date, court and subject decide. A jurisdiction beginning `uk:` is a British or Irish reporter; `us:` followed by a state, or `us:us` for a federal one.

