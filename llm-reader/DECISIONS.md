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

The records are not yet consistent on `7 Edw. 2, 213, 226`: hathi00002
and hathi00009 (appendix) record two citations; the other editions drop
226. Align them once this is decided.

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

### 9. Parallel cites to reprint series in the American notes

Andrews (1894) and later American editors print, beside almost every
American case, its reprint in American Decisions, American Reports,
American State Reports or L.R.A. (and, for English cases, Eng. C. L. or
English Ruling Cases). None of these series is in CAP or the English
Reports. Interim rule: each is recorded as its own citation, linked to the
case's own report at `medium`, as "S. C." reports are. That roughly doubles
the citations from these notes. Decision wanted: keep recording them, or
leave reprint parallels out. The 1894 readers of pageids 01070–01360 left
them out (noted per page); align once decided.

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

### 10. Cases identified but with no stub

Readers link a stub only when one exists at the case's first page (they
cannot create stubs: `llm_links.stub_cite` is a foreign key onto
`legalhist.stub_cases`, which `make db-stubs` derives from the pipeline's
misses). A case the reader identified (name, volume, reporter, corrected
page) in a reporter with no source and no stub at that page is recorded as
`no_match` with the reading kept: Lutwyche pages with no stub, Root,
Weekly Notes of Cases, the Kentucky Law Reporter, Law Reports of the 1890s,
and others. `SELECT ... FROM moml_citations.llm_links WHERE status =
'no_match' AND volume IS NOT NULL` lists them; they could seed stubs or a
review list.

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
   The Liber Assisarum (`22 Ass. 41`) is by plea: empty `page`, plea in
   `note` (records to 2026-11-21 put it in `page`).
2. **Pin cites on split English Reports records:** a cite in a later record
   of a case that begins earlier is `pincite: true`, even at that record's
   first page. Early records often lack it: Peacock v. Bell (1 Saund. 73),
   Hayman v. Gerrard (1 Saund. 102), Cutler v. Southern (1 Saund. 116),
   Craft v. Boite (1 Saund. 246), Osborne v. Rogers (1 Saund. 267), Greene v.
   Jones (1 Saund. 298), William v. Gwyn (2 Saund. 45), Manser's Case
   (2 Rep. 3), Collins v. Blantern, Bridgwater v. Bythway (3 Lev. 113, check
   whether one case or two). Conversely a folio-lettered first page
   (Dyer 171 b, 1 Saund. 295 a n. 1 in hathi00002) is not a pin cite.
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
   at 76 in some records). Bridgwater v. Bythway, 3 Lev. 113, is 17970039
   without `pincite`, at `medium` (settled 2026-11-21; drop it from item
   2's list; hathi00006/01780 has it at `high`). Grade every `no_match`
   by the README's rule (`high` legible and named, `medium` legible
   unnamed); earlier records mix them (Denison v. Richardson, Holding v.
   Haling, Hickman v. Walker). "1 Saund. 374, n. 1" (1831, 1871) is 274,
   The King v. Sutton, as the English editions print it; hathi00002 links
   Toomes v. Etherington. Smith v. Goldsworthy, 1 Dowl. N.S. 288
   (19005765700/00610–00900) kept an exact stub although the English
   Reports have the case (2 QB 717, 4 QB 430): pick the record by year and
   link it, per the same-case rule; likewise Paddock v. Forrester,
   1 Dowl. N.S. 527 (19005765700/00910–01200), at `low`. "Plow. Com. 231"
   (Willion v. Berkley, 1 Plowden 222, pin cite) is recorded in the 1895
   records but skipped in the 1831 record of the same footnote: align.
   "Anon., 2 Salk. 519" is 17950161 everywhere (19005765700/01510–01800
   picked 17950162 on two pages). A bare `Lutw.` page matching a stub is
   `high` (the volume follows from the page); many records have `medium`.
   Bradford v. Pickles, printed "H. L. Cas." with no page (hathi00013/01910),
   is identified as [1895] A.C. 587, which has a stub: link it at `low`
   with the page supplied rather than `no_match`. 1894 Andrews pages the
   1901 reader read more clearly (19005765000): 02880 Kellogg v. Larkin is
   56 Am. Dec. 164 and Bennett v. McIntire 6 L.R.A. 736; 02990 Williams v.
   Preston Am. Dec. 179; 03030 McKinley v. McGregor 522; 03040 Wadhams v.
   Swan 54 (pin cite); 02860 Illinois Fire Ins. v. Stanton 359 (pin cite);
   seq order on 02830, 02890, 02920, 02990 (interleaved columns).
   "Holt's Inst. 562" (1894 records, e.g. 19005765000/03340) is 561, no
   `pincite`. 19005765000/03690: the year book is 1 Hen. 7, fo. 16, not 10. Remove `case_name` where no name is printed
   (e.g. hathi00009/02580 and hathi00010/02600, "Bennet v. Holbech" on a
   bare Saund. 319 n. 6). The hathi00010 (1882) records apply the current
   grading rules; bring the matching hathi00009 (1871) records into line
   (e.g. Holding v. Haling `high`, Ld. Ray. 333 S. C. `low`, Blockley v.
   Slater `medium`, no `case_name` on 1 Ld. Raym. 202). Buckley v. Rice
   Thomas, Plowd. 128 a, is Bulkeley v Thomas, 2 Dy 113 (17941482), pin
   cite, `medium`, in every edition; several records left it `no_match`.
   Hartford v. Jones, 2 Salk. 654: relink every record to the duplicate
   with a `word_count`. Likewise Colt v. Bishop of Coventry (Hob.): relink
   17921247 to 17920644 in hathi00002, hathi00006, hathi00009 and the Gale
   records.
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
   Findings unit above as its own GitHub issue, linked from #439.
   The owner first decided (2026-11-21) not to read the UK editions, then
   reversed it the same day: after the last U.S. edition, read the UK
   editions (7 official copies, 4,219 pages) with ONE reader at a time,
   slow and steady. The progress page shows both (no `--jurisdiction`).
   Order, as the owner confirmed: (a) finish the U.S. reading; (b) read the
   UK editions; (c) the consistency pass above, over U.S. and UK records;
   (d) post the decisions and findings as issues; (e) the stubs (item 10).
10. The owner wants the identified `no_match` cases (finding 10) added to
    `legalhist.stub_cases` (asked 2026-11-21). Draft it (a migration, or a
    change to `db/stub_cases.sql` so `make db-stubs` keeps them) and remind
    the owner when the reading is done; they run it with write access.

Already aligned across editions (recorded in the README's "choices already
made"): 1 Saund. 228 n. (1), 1 Saund. 318 n. (3), 2 Saund. 291 n., 2 Saund.
319 n. (6), 2 Saund. 62 b n. (5), 1 Saund. 284 n. (4), 12 Mod. 508, Salk. 544,
Goram v. Sweeting, Harmer v. Rowe, King v. Lyme Regis, 6 Mass. 304, 2 Bay 204.
