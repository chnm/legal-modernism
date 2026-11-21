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
(1813–16); J. B. Moore; Tyrwhitt; Manning and Ryland (one stub);
the year books. Missing ranges: Plowden pp. 1–130; Benloe pp. 44–88;
14 East 289–301 (Denison v. Richardson, probably the missing id
18110402 between Bowring v. Pritchard, 18110401, and Bateman v. Smith, 18110403); 1 Esp 449–456. Readers link the
same case through another reporter where they can. Dalison, thought to be
missing, is in the English Reports as `2 Ben & D` (123 E.R. 225–315), which a
search for `Dal` misses; its records are untitled groups of notes.

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
cxxxvi of the second appendix, and the 1867 scan (19005773800) none for
p. 388 (between pageids 03280 and 03290), though the text runs on across them; other
scans have "FOLLOWING PAGE(S) MUTILATED" targets (1831 p. 00460, 1841
p. 01080). Readers record what is there; the missing text is not read.

## Coordinator's to-do (not for the owners)

Kept here so it survives a restart or a compacted conversation.

### Running the reading

- One reader at a time, 30 pages per round (the owner's instruction,
  2026-11-20); the prompt is `llm-reader/PROMPT.md`, with the table of
  HathiTrust copies, their Gale copies and pageid offsets.
- Next pages: `bin/llm-reader todo --jurisdiction US --limit 30`. After each
  round: `LAW_DBSTR="$LAW_CLAUDE" bin/llm-reader validate --db --psmid <psmid>`,
  commit `llm-reader/`, push, and fold readers' questions into the README.
- Progress page: `bin/llm-reader serve --port 4314 --jurisdiction US --title
  "Reading Stephen on Pleading: U.S. editions (official copies)"` (shows
  pages per hour over ten minutes, time in New York).
- When a usage limit is near, stop the readers, validate, commit and push;
  a restarted session resumes from the first page without a record.

### Consistency pass, at the end of the U.S. editions

The Gale records read early (1831, 1841, 1845, and parts of 1851–1867)
predate several rules. Bring every U.S. record, Gale and HathiTrust, into line:

1. **Year books:** `reporter_abbr` `Y.B.`, regnal year as `volume`, reign in
   `note`, `page` empty when cited by plea number only; confidence `high`
   when legible. Early records use `Ed. 3.`, `Hen. 6`, `Ass.` and low/medium.
2. **Pin cites on split English Reports records:** a cite in a later record
   of a case that begins earlier is `pincite: true`, even at that record's
   first page. Early records often lack it: Peacock v. Bell (1 Saund. 73),
   Hayman v. Gerrard (1 Saund. 102), Cutler v. Southern (1 Saund. 116),
   Craft v. Boite (1 Saund. 246), Osborne v. Rogers (1 Saund. 267), Greene v.
   Jones (1 Saund. 298), William v. Gwyn (2 Saund. 45), Manser's Case
   (2 Rep. 3), Collins v. Blantern, Bridgwater v. Bythway (3 Lev. 113, check
   whether one case or two). Conversely a folio-lettered first page
   (Dyer 171 b) is not a pin cite.
3. **`no_match` read before the same-case rule:** compare each Gale
   `no_match` with other editions' records of the same footnote and link
   the same case through another report where they did: Willes (Eaton v.
   Southby, Greenhow v. Ilsley, Moravia v. Sloper, Titley v. Foxall),
   J. B. Moore (Bray v. Freeman, Corporation of Arundel v. Bowman, via
   Taunton), Marshall (Jackson v. Wickes, via Taunton), Plowden pp. 1–130
   via Dyer (Partridge v. Strange, Buckley v. Rice Thomas, Fulmerston v.
   Steward), Benloe pp. 44–88 (Thorn v. Rolfe via 2 Dy 185), Dy. 264 a
   (Horneby v. Clifton, low), Tyrwhitt and Dowling cites. Already done:
   Topping v. Fuge (1831), Dal. 67 (all editions).
4. **Unnamed cites** are `medium`, not `high` (e.g. 4 Wend. 211, 12 Wend. 249
   in the early records).
5. **`raw`:** collapse line breaks inside a citation to single spaces
   (HathiTrust records).
6. **`page` holds the corrected reading** where a named case is found at
   other numbers than printed (e.g. Jones v. Kitchin, 1 Bos. & Pul. 77, is
   at 76 in some records).
7. **`sup.` after a case name** (Martin v. Kesterton, sup.) is now recorded
   as a back-reference; earlier records left it out.
8. **A bare `Ibid.` after a footnote mixing a case and a treatise**
   (Le Bret v. Papillon, 4 East 502; 2 Chitty 421) is not recorded; the
   Gale 1851 record (19005764100, the page 60 below hathi00004 02740) has
   it. Also align that copy's "2 Saund. 410, and note (3), ibid." and
   "2 Saund. 97, and 97 b, n. (2)" to one record each, and 1 Saund. 23 n. (2)
   to Birks v. Trippet at 28. In the 1851 appendix: 2 Mass. 53 is a pin
   cite into Benson v. Swift (50), and 2 Saund. 380 n. (14) has `low` in some
   records and `medium` in others; pick one. Every "choice already made"
   in the README (12 Mod. 508, Doug. 148, ...) is `medium`; earlier records
   have some at `low`. A stub matching the printed cite exactly is `high`,
   and a case found by an unambiguous printed name at a corrected page is
   `high` (rules of 2026-11-21; earlier records vary). `pincite` is false
   for a first page in another report linked through the same-case rule
   (Harmer v. Rowe, Lucas v. Nockells); earlier records set it.
9. After the pass: validate everything, commit, and post each Decisions and
   Findings unit above as its own GitHub issue, linked from #439. Then start
   the UK editions (7 official copies, 4,219 pages; restart the progress page
   with `--jurisdiction UK` or none).

Already aligned across editions (recorded in the README's "choices already
made"): 1 Saund. 228 n. (1), 1 Saund. 318 n. (3), 2 Saund. 291 n., 2 Saund.
319 n. (6), 2 Saund. 62 b n. (5), 1 Saund. 284 n. (4), 12 Mod. 508, Salk. 544,
Goram v. Sweeting, Harmer v. Rowe, King v. Lyme Regis, 6 Mass. 304, 2 Bay 204.
