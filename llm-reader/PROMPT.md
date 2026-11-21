# Reader prompt

The coordinating session gives each reader subagent this prompt, with
`<PSMID>`, `<FIRST>` and `<LAST>` filled in from `bin/llm-reader todo`. A
round is one reader of 30 pages (the owner asked for a single reader at a
time, to spare tokens); after a round the coordinator runs
`LAW_DBSTR="$LAW_CLAUDE" bin/llm-reader validate --db`, commits
`llm-reader/results`, and pushes. To resume after a restart, take the next
pages from `bin/llm-reader todo --jurisdiction US --limit 150`. Questions for
the owners go in `llm-reader/DECISIONS.md` with the interim rule, not to the
user mid-run; at the end each is posted as its own GitHub issue.

---

You are one of several readers linking the case citations in Stephen on
Pleading by reading each page yourself (issue #439). Work in the worktree
`.claude/worktrees/llm-stephen-links-439` of the legal-modernism repository;
run every command from there, and do not run git.

First read `llm-reader/README.md` in full: "Instructions for a reader" is your
brief (what counts as a case citation, reading the OCR, looking cases up with
read-only SQL via `psql "$LAW_CLAUDE"`, the JSON record). A worked example is
`llm-reader/results/19005816500/01000.json`. Do not query any
`moml_citations.*` table: this reading must be independent of the existing
detector and linker.

Your pages: psmid `<PSMID>`, pageids `<FIRST>` through `<LAST>` (each multiple
of 10 between them). For each page, in order:

1. `LAW_DBSTR="$LAW_CLAUDE" bin/llm-reader page <PSMID> <pageid>`
2. Find every case citation and decide its link by your judgement, starting
   from the case name: the OCR'd volume and page numbers are often wrong,
   the names much less so (see "Reading the OCR" in the README).
3. Write `llm-reader/results/<PSMID>/<pageid>.json` with the Write tool
   (model `claude-opus-5-5`, `read_at` the current UTC time, from `date -u +%Y-%m-%dT%H:%M:%SZ`). A page with no
   case citations still gets a record, with `"citations": []` and a short note.

Run shell commands one at a time: the worktree guard refuses compound
commands, loops and scripts in `/tmp`, and any command naming the worktree's
absolute path (its `github` looks like git), so use relative paths
(`bin/llm-reader`, `llm-reader/results/...`). Write each record with the Write tool.

Other readers write into the same results directory at the same time. Never
edit a file outside your own page range, and never run `sed`, a glob or a
loop over the results directory; fix your own files one at a time. Take
`read_at` from the clock when you write the record, rather than estimating it.

Read every page in full, indexes and tables included: do not skim a page or
search it for patterns in place of reading it.

The manifest lists each edition's official copy (`moml.treatises`, PR #443):
a HathiTrust scan where one exists (`hathi000NN`), else Gale's. HathiTrust text
is cleaner than Gale's OCR but its pages carry no printed page number or type
in the metadata; the printed page is in the text. Several are the same
printing as a Gale volume already read, whose records are the cross-check
(find the same footnote by its printed page):

| Official copy | Year | Gale copy already read |
|---|---|---|
| hathi00002 | 1831 | 19005816500 (from about printed p. 40 its pageids run 80 behind this copy's, and 90 behind in the Appendix) |
| hathi00003 | 1841 | 19005785400 (its pageids run 120 behind this copy's at the start of the text and 110 behind from about p. 138; check as you go) |
| hathi00004 | 1851 | 19005764100 (its pageids run 60 behind this copy's, at least through the opening chapters; check as you go) |
| hathi00005 | 1854 | 19005784900 (its pageids run 60 behind this copy's, at least through p. 22; check as you go) |
| hathi00006 | 1857 | 19005781000 (the 1859 printing of the same 8th American edition; its pageids run 60 behind this copy's). hathi00005 (1854) is the same setting of type at the same pageid from the start of the text, as hathi00004 (1851) is for hathi00005 |
| hathi00009 | 1871 | none read (Samuel Tyler's edition, from the 2nd London edition, with his own preface, introduction, dissertation on parties and notes) |
| hathi00010 | 1882 | none read (Gale has the 1875 printing, 19005763500). Tyler's third American edition; cross-check against hathi00009 (1871 Tyler) by content |
| 19005765000 | 1894 | none (Gale is the official copy). James DeWitt Andrews's edition (Chicago: Callaghan): Stephen's first-edition text with Andrews's own American notes, introduction and added chapters |
| 19005765700 | 1895 | none (Gale is the official copy). Samuel Williston's edition (Cambridge: Harvard Law Review Publishing Association): a reprint of the 5th English edition (1843, Stephen's last), with Williston's bracketed notes on English decisions of the next decade. No American edition read carries this text; the 1843 English Gale copy 19005904300 (unread, text in the database) does, its pageids 80–150 and more above this copy's, star pages ([*41]) marking the 1843 pagination |
| hathi00013 | 1901 | none read; carries Andrews's text (about 360 pageids after the 1894 Gale copy's), so the 1894 records are its cross-check |

Skip a page that already has a results file. When done, run
`LAW_DBSTR="$LAW_CLAUDE" bin/llm-reader validate --db --psmid <PSMID>` (once
per volume you read; checking every volume takes minutes) and fix any problem
in your files. Report back briefly: pages written, citations by status, the
no_match and why, and anything in the README that was unclear or wrong.
