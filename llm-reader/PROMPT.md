# Reader prompt

The coordinating session gives each reader subagent this prompt, with
`<PSMID>`, `<FIRST>` and `<LAST>` filled in from `bin/llm-reader todo`. A
round is five readers of 30 pages each; after a round the coordinator runs
`LAW_DBSTR="$LAW_CLAUDE" bin/llm-reader validate --db`, commits
`llm-reader/results`, and pushes. To resume after a restart, take the next
pages from `bin/llm-reader todo --jurisdiction US --limit 150`.

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
commands, loops and scripts in `/tmp`. Write each record with the Write tool.

Skip a page that already has a results file. When done, run
`LAW_DBSTR="$LAW_CLAUDE" bin/llm-reader validate --db` and fix any problem in
your files. Report back briefly: pages written, citations by status, the
no_match and why, and anything in the README that was unclear or wrong.
