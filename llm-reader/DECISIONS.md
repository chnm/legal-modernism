# Open decisions and findings

Questions for the project's owners that came up while reading Stephen on
Pleading (#439), held until the reading is done. At the end each logical
unit is posted as its own GitHub issue. Until then the readers follow the
interim rule given here.

## Decisions

### 1. Second numbers after a cite that are real citations

Your rule: ignore a second number after a cite (`1 N. R. 64. 233`), since
most are OCR noise. But some are real:

- The American editors' strings of cases in one volume:
  `1 T. R. 151. 508. 532`, `Stra. 189. 808`, `2 Mass. 53. 408`,
  `2 Conn. 324. 358`, `Cro. El. 262, 539` (1841 appendix notes).
- A second report of the same case with its own English Reports record:
  `3 Taunt. 91. 232` (Swinnerton v. Marquis of Stafford),
  `3 Wils. 419. 541` (Tyssen v. Clarke), `2 Vent. 198. 222`.
- Two year-book folios cited as separate authorities: `7 Edw. 2, 213, 226`.
- `Dyer, 66 b, 171 b`: two different cases; readers have recorded both.
- `1 Sch. & Lef. 320. 324`, where 324 is probably Shannon v. Shannon, the
  replevin case the passage needs, and 320 only the stub's first page.

Interim rule: the second number is dropped and listed in `note`
(`dropped: 508, 532`); a second Saunders page carrying its own note
(`2 Saund. 116 b. 411, n. (4)`) is recorded as a second citation.
Decision wanted: keep ignoring them, or record a second number when it is
plausibly a separate case or report.

### 2. Goram v. Sweeting (resolved by the owner, 2026-11-20)

Goram v. Sweeting, K.B., Mich. 1670 (22 Car. II), argued with Goram v. Fowke
and Goram v. Bateman, is case 39 of 2 Saund.: it begins at 2 Saund. 200, and
p. 205 has the end of the plea-roll entry and the start of Saunders's report
(parallel report 2 Keb. 717, 722). The English Reports records at
2 Wms Saund 200 (18450278) and 205 (18450230) are this case, titled for the
companion suit ("Same v Bateman"). Every edition's cite now links the record
holding its page; its notes 21-24 (pp. 206-207) link the record at 205. The
mis-titling belongs with item 4.

### 7. Price v. Brown at "1 Wils. 116, S. C."

Every edition prints "Price v. Brown, Str. 690; 1 Wils. 116, S. C." The
English Reports have Price v. Brown only at 2 Str 691; 1 Wils KB 116 falls in
Alcorn v. Westbrook (115), a bond-payment case on the footnote's exact point,
whose report may print Price v. Brown as cited. Interim rule: the Wilson cite
is `no_match` in every edition. Decision wanted: leave it, or link it to
Alcorn v. Westbrook as a case reported only as cited in another (`low`).

## Findings for issues

### 3. Gaps in the English Reports and stubs

Reporters cited in Stephen with no source: Willes; Marshall's Common Pleas
(1813–16); J. B. Moore; Dalison; Tyrwhitt; Manning and Ryland (one stub);
the year books. Missing ranges: Plowden pp. 1–130; Benloe pp. 44–88;
14 East 289–301 (Denison v. Richardson); 1 Esp 449–456. Readers link the
same case through another reporter where they can.

### 4. Mislabelled English Reports records

`er_name` is sometimes another case's title where `murrell_title` is right
(2 Wms Saund 200 and 205, "Same v Bateman" for Goram v. Sweeting;
1 M & S 304, "Same against Hoar the Younger" for Rowe v. Roach;
"Weston versus Chapman" for Weston v. Mason; "Same against Ireland" for
Lord Huntingtower v. Gardiner; "In Michaelmas Term" at 1 TR 510). Some are
filed under the wrong page: Mellor v. Leather's `er_parallel_cite` is
`1 El & Bl 518`, but its `er_cite` (118 E.R. 569) puts it at 619, as printed.

### 5. Whitelist readings that are wrong in an English treatise

- `N. R.` is read as the New Reports of 1862–65; in a book of 1831 it is
  Bosanquet and Puller's New Reports (1804–07). The pipeline linked
  `1 N. R. 64` to a stub of the wrong series.
- `Marsh.` is read as A. K. Marshall (Kentucky); in an English context it is
  Marshall's Common Pleas Reports.
- `Went.` (Wentworth's precedents, not reports) is linked to stubs.
- `Dowl.` is read as Dow's Parliamentary Cases; in a treatise of the 1830s–60s
  it is Dowling's Practice Cases, which the stubs hold as `3 Dowl. 70`.

### 6. CAP first pages that disagree with the printed cite

In early Johnson's and Massachusetts Reports, `cap.cases.first_page` can
differ from the cite in `cap.citations` (Hopkins v. Hopkins, cited
`10 Johns. 369`, first_page 375).

### 8. Pages missing from MOML scans

The 1859 edition's scan (19005781000) has no page for printed pp. cxxv and
cxxxvi of the second appendix, though the text runs on across them; other
scans have "FOLLOWING PAGE(S) MUTILATED" targets (1831 p. 00460, 1841
p. 01080). Readers record what is there; the missing text is not read.
