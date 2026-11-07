# CLAUDE.md

## Project overview

Legal Modernism studies American legal history through computational methods. Data is drawn from the Making of Modern Law, the Caselaw Access Project, and custom datasets. Data is stored in PostgreSQL. Code is written in Go. The project website is built with Hugo.

## Repository structure

Programs (Go binaries):

- `adj2edge/` — Convert network adjacency list to edge list
- `cap-import/` — Import data from the Caselaw Access Project
- `cite-detector-moml/` — Detect citations in the *Making of Modern Law* treatises
- `cite-detector-cap/` — Detect citations in the opinions of the Caselaw Access Project (issue #74)
- `cite-predictor/` — Augment citation detection using generative AI
- `chambers/` — Internal web server for inspecting citation data
- `cite-linker/` — Link the citations detected in the treatises to a database of caselaw
- `cite-linker-cap/` — Link the citations detected in the CAP opinions the same way

Support directories:

- `db/` — Database migrations, schema, and queries
- `go/` — Shared Go packages (citations, db, linker, sources)
- `scripts/` — One-off R and shell scripts for data manipulation
- `slurm/` — Slurm batch job scripts for running programs on the HPC cluster
- `test-data/` — Sample data for development and testing
- `website/` — Hugo static website
- `notebooks/` — Analytical notebooks (R, Python, Quarto)
- `doi/` — DOI metadata records

## Go development

- Module: `github.com/lmullen/legal-modernism`
- Go version: 1.25
- Build: `go build ./cmd-name/`
- Run: `go run ./cmd-name/`
- Test: `go test ./...`
- Tests use `stretchr/testify` with table-driven test patterns
- Key dependencies: `jackc/pgx/v5` (PostgreSQL), `gammazero/workerpool` (concurrency), `schollz/progressbar` (CLI progress), `stretchr/testify` (testing)

Write code in Go unless instructed otherwise.

## Code conventions

Follow idiomatic Go patterns. Specific conventions used in this project:

**Logging:** Use the `slog` package for structured logging. Output JSON to stderr. Control log level with the `LAW_DEBUG` environment variable. Domain objects should provide a `LogID()` method returning `[]any` key-value pairs for consistent structured log context.

Log levels:

- **DEBUG** — intermittent, per-iteration progress (e.g. per-batch counts inside a loop). Gated behind `LAW_DEBUG` so a normal run isn't noisy.
- **INFO** — phase starts and final results (e.g. `deleted X rows`, `loaded N entries`, `done linking citations`). For a long operation, log the outcome once when it finishes — not per chunk.
- **WARN** — recoverable problems that don't stop the run.
- **ERROR** — failures, typically right before exiting.

```go
slog.Debug("saved batch", batch.LogID()...)        // per-iteration progress → DEBUG
slog.Info("done linking citations", "processed", n) // final result → INFO
slog.Error("batch failed", batch.LogID("error", err)...)
```

**Repository pattern:** Database access uses interface-based `Store` types (see `go/citations/store.go`, `go/sources/store.go`). Implementations wrap `*pgxpool.Pool`.

**Error handling:** Wrap errors with context using `fmt.Errorf("context: %w", err)`. Define sentinel errors in `errors.go` files (see `go/sources/errors.go`).

**Concurrency:** Use `gammazero/workerpool` for parallel processing. Use `context.Context` with timeouts for database operations and graceful shutdown via signal handling.

## Database

- PostgreSQL 17.x, connected via `go/db/db.go` using pgx v5 connection pooling
- Connection string from `LAW_DBSTR` environment variable
- Migrations managed by [dbmate](https://github.com/amacneil/dbmate) in `db/migrations/`
- Migration naming: `YYYYMMDDHHMMSS_description.sql`, the version taken from the clock (`date -u +%Y%m%d%H%M%S`), never a round or hand-picked time. dbmate records an applied migration by its version alone, so two files sharing one are one migration to it and the second is silently skipped; `go/db/migrations_test.go` fails on a duplicate.
- Full schema: `db/schema.sql`
- Schemas: `cap`, `cap_citations`, `english_reports`, `legalhist`, `moml`, `moml_archive`, `moml_citations`, `opinion_citations`, `stats`, `sys_admin`, `textbooks`
- MOML's `psmid` identifies a volume (`moml.volumes`), `bibliographicid` an edition of one or more volumes (`moml.editions`), and `editions.work_id` its work (`moml.works`, issue #335): a treatise across all its editions, taken broadly to include later editors' revisions, translations, and what was made from it (abridgments, analyses, question books, supplements), which `editions.derivative` marks. Every edition has a work, most of them a work of one edition. The works were built from three title-and-author rules and a reviewed list of links in `db/migrations/20260925170000_moml-works.sql`; `db/queries/work-candidates.sql` lists pairs that may still belong together, and a correction is a migration that moves `work_id`. A column lives on `editions` only if every volume of the edition shares it (author, collation, total volumes); title, edition statement, imprint and dates stay on `volumes`, because a few editions mix volumes of different printings. `current_volume` 0 means not part of a numbered set, and is not unique within an edition. Subjects and LoC headings belong to the edition (`edition_subjects`, `edition_loc_subjects`), in catalogue order by `position`; a LoC heading is one row per MARC subfield and subfield `a` begins one. `volume_sets` records Gale's sibling volumes, a few of which cross editions. The side-corpus volumes (`momlextra*`) have no Gale id and are each their own edition. `volumes.year` is the volume's own year, not its set's first year. Editions MOML scanned twice were reduced to one copy, recorded in `moml.duplicate_copies`. `moml.treatises` is the editions that count as treatises, with `jurisdiction` `US` or `UK` taken from the edition's subjects; join `moml.volumes` for the volumes. It leaves out pamphlets under 50 pages, public documents and arguments in a case (by title), biographies and speeches (by Gale subject or LoC heading), collected essays, trials and commemorative pieces, but never an edition whose title calls it a treatise; digests, casebooks, dictionaries and form books stay. `db/queries/treatises-excluded.sql` lists what is left out and why. The original tables, the four catalog records with no volume, and everything the duplicate removal took are in `moml_archive`, which a later migration drops.
- Migrations should be idempotent: use `IF NOT EXISTS` / `IF EXISTS` guards on `CREATE INDEX`, `CREATE TABLE`, `ADD CONSTRAINT`, `DROP CONSTRAINT`, etc.
- `legalhist.reporters.cited_by_year_from` is the first year from which a reporter is cited by year (K.B. 1901, the Irish Reports 1894, QB and the Appeal Cases 1891). The detector records the year in `citations_unlinked.year` and drops the year-less reading of the same citation; the linker and `db/stub_cases.sql` key a citation from that year on by it (`[1905] 2 K.B. 1`), and treat an earlier year as decoration on a volume-cited citation.
- The Law Reports of 1865–1875 are cited with the series ahead of the volume, `L. R. 5 Ch. 100`. The detector moves the prefix behind the volume before detecting, so the spelling recorded is `L. R. Ch.`; the whitelist maps those spellings to the series rows.
- A standard spelling always trumps an alternative one. A spelling that is some reporter's `legalhist.reporters.reporter_standard` belongs to that reporter, so a `reporters_abbreviations.alt_abbr` equal to any `reporter_standard` is skipped by both loaders (`LoadReporterAltAbbrs` for the linker, `GetSingleVolReporterAbbrs` for the detector) and refused by the seed migrations' `NOT EXISTS` guard; the rows stay in the table (issue #289).
- The linker refuses an anachronistic link (issue #319): a hit on a case decided in a later year than the citing document, which is the treatise volume's year (`moml.volumes.year`) or, for a citation in a CAP opinion, the citing case's `decision_year`; the corpus store puts it on the citation as `SourceYear`. The case's year is `cap.cases.decision_year`, `legalhist.code_reporter.decision_year`, or for the English Reports `murrell_year`, falling back to `er_year`. The same year is not anachronistic, and an unknown year keeps the link. A refused hit is passed over and the cascade continues; if nothing else links, the row is `no_match` with tier `us_anachronistic` or `uk_anachronistic`. `db/queries/anachronistic-links.sql` measures it.
- A whitelist spelling can have a second reading, `legalhist.whitelist.fallback_standard` (issue #358: bare `Hill` is Hill's S.C. Law Reports, falling back to Hill's N.Y. Reports), which the linker tries after a miss that says the reporter was wrong (`volume_absent`, `diffvols_missing`, `anachronistic`). For a CAP opinion the two readings trade places when the citing case's state is the fallback reporter's and not the spelling's own (issue #401): the corpus store puts the state on the citation as `SourceJurisdiction`, through `legalhist.cap_jurisdiction_codes`, which maps `cap.jurisdictions` onto the `us:xx` codes of `legalhist.reporters.jurisdiction`. A treatise has no citing state and is never reordered.
- `moml_citations.edition_case_citations` (issue #213) is one row for each MOML edition (`bibliographicid`) and each case its linked citations reach: a CAP, English Reports, code reporter or stub case, in the same typed columns as `citation_links`, with `source` naming which. `cite_count` is the number of citations. Join `moml.editions` for the work and `moml.treatises` for the jurisdiction. `make db-maintenance` refreshes it.
- `opinion_citations` (issue #74) is the CAP twin of `moml_citations`: `citations_unlinked` holds what `cite-detector-cap` finds in the text of `cap.opinions` for cases decided in or before its `--max-year` (1920 in `slurm/cite-detector-cap.sh`; nothing in the tables records the cutoff, join `cap.cases` for the citing case's year), and `citation_links` holds `cite-linker-cap`'s results, with the same columns, statuses and tiers as the MOML tables and the citing key `cap_case`, `cap_opinion` in place of `moml_treatise`, `moml_page`. `cap.opinions.id` is a surrogate key added for this (`("case", type)` is not unique), and the detections have foreign keys onto `cap.cases` and `cap.opinions`, so a `TRUNCATE` of either needs `CASCADE` and empties them. The unique key is per opinion, so a cite repeated in one opinion is one row. An opinion's citation of its own case links (same year), and is kept: `case_case_citations`, the twin of `edition_case_citations` at the grain of citing case and cited case, marks it `self_cite`, and every reader filters on that. The stub registry is shared: `make db-stubs` builds it from both ledgers' misses (issue #346), so a `stubs` rebuild in either corpus's run can leave the other ledger's `linked_stub` rows pointing at pruned stubs until that corpus is relinked (`--from truncate-links`); `make db-stubs` prints the stale links per ledger. The two `chk_citation_links_match_tier` constraints are kept in step by hand, and `go/citations/linker_tier_constraint_test.go` checks that every tier is in a migration naming each table. The linker does not resolve pin cites: a citation to an interior page of a case is `no_match` at `us_page_absent` / `uk_page_absent`. The page-range matching that once linked them (`cap_page_interior`, `er_page_interior`, issues #242 and #243) was removed in issue #393 because most of its links were to the wrong case, and the `pincite_only` columns went with it. `cap_citations` is something else: CAP's own citation graph, which `opinion_citations.cap_graph_edges` sets these links against (issue #349): `cap_graph_scope` is the citing cases the comparison covers (decided by 1920, with an opinion, in `cap_citations.metadata`), `cap_graph_edges` every edge between two CAP cases in either graph, self-cites left out, and `cap_graph_agreement` and `cap_graph_case_agreement` its aggregates, which chambers' `/linking/cap` and `db/queries/opinion-citations-vs-cap-graph.sql` read. Most edges only CAP's graph has cite a case none of whose cites occurs in the opinion text, so CAP read text `cap.opinions` does not hold. The CAP linking dashboard's views (`opinion_citations.linking_dashboard_*`, `top_reporters`) are the twins of MOML's.
- `citations_unlinked.case_name` (both ledgers, issue #387) is the case name printed before the citation as the detector read it, `Plaintiff v. Defendant` with the versus normalized to `v.`, `In re X`, or a bare name, NULL when none was found; `case_name_source` says how: `direct`, `parallel` (inherited from the preceding parallel cite across `;`), or `table` (plaintiff taken from the row above in a table of cases). `go/citations/casename.go` is eyecite's backward scan with the treatises' conventions added; it runs in `DetectDocument` after `RemoveShadows`, so it needs the #367 and #373 filters. Neither column is in the unique index. Nothing reads them yet; the linker is the first customer (#359, #323).
- `legalhist.stub_cases` is a derived registry that `make db-stubs` rebuilds from the misses of both linkers, judging a reporter's eligibility and counting a string's citations across the treatises and the CAP opinions together (issue #346); it records a case's identity only (cite, reporter, vol_year, volume, page), never statistics; `vol_year` is the citation year that keys a year-cited reporter, not the year decided. `legalhist.stub_case_metadata` (party names, year decided, jurisdiction) is curated, and its foreign key onto `stub_cases` means a stub with metadata cannot be pruned, deleted, or truncated. A migration that removes or renames stubs must move their metadata first (an `UPDATE` of `stub_cases.cite` cascades) or delete it deliberately.
- Don't update `db/schema.sql`: this file is auto-generated.

### Writing a migration

1. Create `db/migrations/YYYYMMDDHHMMSS_description.sql`, naming it with the current UTC time from `date -u +%Y%m%d%H%M%S` (not a rounded or invented one: parallel branches that both guess "noon" collide, and dbmate then applies only one of them). The version must sort after every migration on `origin/main`, not just on the branch. It has `-- migrate:up` / `-- migrate:down` sections; the `down` must fully reverse the `up`. Start the `migrate:up` with `SET ROLE = law_admin;`, as the existing migrations do.
2. After merging `main` into a branch that adds a migration, check that no version is shared (`ls db/migrations | cut -c1-14 | sort | uniq -d` prints nothing) and that the branch's migration still sorts last; if not, rename it to the current time. `go test ./go/db/` checks the first.
3. Before adding a constraint, verify existing rows won't violate it (query with the read-only `LAW_CLAUDE` connection); a constraint that current data violates fails to apply.
4. Apply with `make db-up`. dbmate dumps the schema after `up`, so that one command also regenerates `db/schema.sql`; `make db-schema` only redoes the dump and is not needed after `db-up`. It needs write access via `LAW_DBSTR`, so Claude cannot run it — ask the user to, giving first the command that puts them in the branch's worktree (see Claude database access).
5. Commit **both** the migration file and the regenerated `db/schema.sql`. "Don't update `db/schema.sql`" means don't hand-edit it, not don't commit it.

## Pipeline run order

`scripts/pipeline.sh` runs the whole rebuild from a workstation with ssh access to hopper (issue #321): `make sync-hopper`, the truncates, the detector and linker Slurm jobs, `make db-stubs`, the second linker pass, and `make db-maintenance`, in the order below. It waits for a job by polling `squeue` over fresh ssh connections rather than holding one open, so a laptop that sleeps resumes where it left off; run it under `caffeinate -i` anyway, since the local steps wait for the laptop to wake. It asks once, up front, before the TRUNCATE statements (`--yes` skips the prompt), fails loudly naming the phase and the `--from PHASE` command that resumes the run, and never cancels a Slurm job: Ctrl-C prints `--from PHASE --job ID` to reattach. Pass or fail comes from the Slurm job state, not the exit code, because the detector exits 1 on a wall-time SIGTERM as well as on a real failure; a job that hits its wall time is resubmitted once. `--dry-run` prints every command without running any. Logs go to `logs/pipeline/<timestamp>/`. `scripts/README.md` documents it in full.

The phases, and the manual steps they replace. After a change to the detector, the whitelist, or the reporter tables:

1. Before running: `make db-up` (which also dumps `db/schema.sql`) and commit `db/schema.sql`. The script's preflight refuses to start while migrations are pending.
2. `truncate-citations`, `detect`: if the detector or its inputs (`legalhist.reporters.single_vol`, `reporters_abbreviations`) changed: `TRUNCATE moml_citations.citations_unlinked CASCADE` (this also empties `citation_links`), then run `cite-detector-moml` (`slurm/cite-detector-moml.sh`). The detector always rescans every page, so it is only worth running after the truncate. The 2026-09-25 run took **21 minutes** from submission to completion on a bigmem node with 64 CPUs, 12 of them scanning pages, and saved 55.9M citations; since 2026-10-23 the job asks for 24 CPUs, 16 GB and two hours on the normal partition, sized to what a busy partition has free on one node (a 64-CPU request queued for over five hours on bigmem and was estimated eighteen on normal), so expect about 50 minutes of scanning.
3. `link`: run `cite-linker` (`slurm/cite-linker.sh`). There is no `--reset` flag: to re-derive existing rows after a whitelist or linking change, `TRUNCATE moml_citations.citation_links` first and then run it unchanged, which is `./scripts/pipeline.sh --from truncate-links`. A full rebuild takes about ten minutes.
4. `stubs`, `truncate-links`, `relink`: `make db-stubs` to rebuild `legalhist.stub_cases` from the linker's misses (issue #248), then `TRUNCATE moml_citations.citation_links` and run `cite-linker` again so that citations to reporters no source covers link to the stubs (status `linked_stub`). A routine incremental run (`./scripts/pipeline.sh --from link`) needs neither step: the linker links new citations to the existing registry. The threshold is `STUB_THRESHOLD` (default 5 citations); `db/queries/stub-case-threshold.sql` is how it was sized.
5. `maintenance`: `make db-maintenance` to vacuum the churned tables and refresh the materialized views that read them, which the chambers dashboard reads. `db/maintenance.sh` finds the views and their dependencies in the catalogs, skips the views over static source data (CAP, FreeLaw, the MOML page text; `make db-maintenance-all` refreshes those too, after a reimport), and runs the refreshes a few at a time, costliest chain first, so each gets its parallel workers. `--dry-run` with `LAW_CLAUDE` prints the plan without writing.

The CAP corpus (issue #74) runs through the same script with `--corpus cap`: `preflight`, `sync`, `truncate-citations` (`TRUNCATE opinion_citations.citations_unlinked CASCADE`), `detect` (`cite-detector-cap`, `slurm/cite-detector-cap.sh`), `link` (`cite-linker-cap`, `slurm/cite-linker-cap.sh`), `stubs`, `truncate-links`, `relink` and `maintenance`, the same phases as MOML, since CAP misses feed the stub registry too (issue #346). Because the registry is shared, follow a run that rebuilt the stubs with a relink of the other corpus (`--corpus cap --from truncate-links` after a MOML run, `--from truncate-links` after a CAP one). `--corpus cap --from truncate-links` relinks after a whitelist or linker change, and `--corpus cap --from link` links new rows. The preflight refuses to submit while any of the four jobs is queued, because a detector's 64 connections beside a linker's 34 exceed the 97 the server allows, so a CAP job never runs beside a MOML job. A cap run's `maintenance` refreshes every materialized view, MOML's included. Before the first run the user applies the three migrations (`make db-up`; the first holds `cap.opinions` under an exclusive lock for about ten minutes while it backfills the id, and is followed by a hand `VACUUM (ANALYZE, PROCESS_TOAST false) cap.opinions;`), and commits the `db/schema.sql` it dumps.

## Environment variables

- `LAW_DBSTR` — PostgreSQL connection string
- `LAW_CLAUDE` — Read-only PostgreSQL connection string for Claude
- `LAW_DEBUG` — Set to `debug` or `true` for debug-level logging

## Claude database access

- **Never use `LAW_DBSTR`.** When connecting to the database, always use the `LAW_CLAUDE` environment variable, which connects with a read-only user.
- **Read-only access only.** Only run `SELECT` queries. Never run `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `TRUNCATE`, `CREATE`, or any other command that writes to or modifies the database.
- **Commands for the user on a branch.** When you need the user to run a database command for work on a branch (`make db-up`, `make db-schema`, `make db-stubs`, `make db-maintenance`, a `TRUNCATE`, `scripts/pipeline.sh`, …), first give the command that puts them in that branch's worktree, e.g. `cd .worktrees/<branch-name>` from the project root, and only then the database command, so it runs against the branch's migrations and code rather than whatever the main checkout has checked out.

## Website

Hugo static site in `website/`. Uses Bootstrap 5 via CDN, no external Hugo theme.

- `make preview` — Dev server on port 54321
- `make build` — Production build with minification
- `make deploy` — Build and rsync to production

## Chambers

Internal web app for browsing and inspecting citation data. Located in `chambers/`.

- Build: `go build ./chambers/`
- Run: `go run ./chambers/` (default port 4567)
- To show the user a running server from the sandbox, listen on one of ports 4313–4316 (`go run ./chambers/ --port 4313`): those four ports are exposed from the container to the same ports on the user's machine, so the server is at `http://localhost:4313` for them too. Connect it with the read-only user, `LAW_DBSTR="$LAW_CLAUDE"`, since chambers only reads.
- Dev with live reload: `air` (configured via `chambers/.air.toml`)
- Templates and static files are embedded via `//go:embed`
- Routes registered on `http.NewServeMux()` in `main.go`
- To add a new page: add query in `queries.go`, create template in `templates/`, add handler and register route in `main.go`, add nav link to `home.html`

**Frontend:** Use the latest version of Bootstrap CSS via CDN (no Bootstrap JavaScript). Use Observable Plot (`@observablehq/plot`) for data visualizations when possible, and D3.js otherwise.

**Public visualizations:** visualizations meant for readers outside the project (articles, talks, eventually the website) follow `chambers/VISUALIZATIONS.md`: anatomy, typography, colour, interactivity, accessibility, sizing, and how they consume JSON. Read it before building one, ask whether it is full screen or in the viewport before starting, and record every colour assigned to a category in its colour registry. They live under "Draft public visualizations" on the home page, at `/viz/<name>` with their JSON at `/api/viz/<name>`; the first is `/viz/jurisdictions` (issue #216), which reads `opinion_citations.jurisdiction_citations`, our CAP links rolled up by the citing case's jurisdiction and year and the cited case's jurisdiction, with the English Reports as `uk`; a citation is one opinion citing one case, counted once however many cite strings link it (`case_case_citations.opinion_count`). The second is `/viz/field-code` (issue #117), figure 5 of Funk and Mullen's "The Spine of American Law" (AHR 2018) as a D3 network: the codes of civil procedure connected by the sections they borrowed, laid out where the published figure put them, with the codes of the corpus the figure left out beside it. Its data is not in the database but two JSON arrays in `chambers/data/`, embedded and served at `/api/viz/field-code/...`, which `scripts/field-code-network` derives from the article's research compendium and the hand-edited SVG of the figure (neither in the repository); moving them into the database is a later task.

## Branches and worktrees

Whenever the user asks you to work on a separate branch, use a git worktree instead of switching branches in the current checkout.

- Never run `git switch` or `git checkout <branch>` in the main working directory. Leave it on whatever branch it's on, with any uncommitted changes untouched.
- Create worktrees inside the project at `.worktrees/<branch-name>` (replace `/` in branch names with `-`).
- Before creating the first worktree, make sure `.worktrees/` is listed in `.gitignore`. If it isn't, add it and tell the user.
- For a new branch, base it on `main` unless the user says otherwise:
  `git worktree add -b <branch> .worktrees/<branch> main`
- For an existing branch: `git worktree add .worktrees/<branch> <branch>`
- If a worktree for that branch already exists (check `git worktree list`), reuse it rather than creating another.
- After creating a worktree, do all reading, editing, building, and testing from inside that directory. Install dependencies there if needed (e.g., `go mod download`, `npm install`).
- Tell the user the worktree path when you create it.
- Commit work in the worktree's branch. Don't merge into `main` or push unless the user asks.
- Don't remove a worktree or delete its branch unless the user asks or its PR has been merged (see After a PR is merged). When they do, use `git worktree remove <path>` followed by `git branch -d <branch>`, and run `git worktree prune` if needed.
- When the user needs to run a database command for the branch, give the command to switch to its worktree (`cd .worktrees/<branch-name>`) before the database command itself (see Claude database access).

### After a PR is merged

When the user says a PR has been merged, or you notice it yourself (`gh pr view <n> --json state,mergedAt` says `MERGED`, or the branch's commits are already in `origin/main`), clean up without being asked: the branch and its worktree have done their job, and a stale `main` is what makes the next branch start from the wrong place.

1. `git fetch --prune origin`, then confirm the PR really is merged before removing anything.
2. Remove the worktree: `git worktree remove .worktrees/<branch>` (a sandbox session's worktree lives at `.claude/worktrees/<name>` instead; remove it the same way). If the worktree has uncommitted changes, stop and ask rather than forcing it.
3. Delete the local branch with `git branch -d <branch>`. Because the project merges with a merge commit, `-d` succeeds once `main` is synced; if it refuses, check that the PR shows as merged before using `-D`.
4. Sync `main` with the remote. From the main checkout, if `main` is checked out and the tree is clean, `git pull --ff-only origin main`; if another branch is checked out there, `git fetch origin main:main` updates it without switching. If neither fast-forwards, stop and tell the user rather than merging or resetting.
5. `git worktree prune`, then report what was removed and the commit `main` now points at.

Do this for every branch of the merged PR, not just the one this session created, and leave the remote branch to GitHub's own cleanup.

## GitHub

Use the `gh` CLI for GitHub operations — creating pull requests, viewing issues, checking CI status, and commenting. In the sandbox it is authenticated as the machine account `lmullen-agent` (see Attribution).

- Open a PR: `gh pr create --base main --head <branch> --title "..." --body "..."`
- Reference the issue a PR closes in the commit or PR body with `Closes #<n>` so it auto-closes on merge.
- View an issue: `gh issue view <n>`
- Check CI on a PR: `gh pr checks <n>`

Do not hand-build GitHub API calls with `curl`; prefer `gh`.

The project status board is https://github.com/orgs/chnm/projects/21/views/1.

### Attribution

Work that Claude does is attributed to the machine account `lmullen-agent`, not to the
repository owner `lmullen`.

- **Commits:** in the sandbox, `GIT_AUTHOR_*` and `GIT_COMMITTER_*` in
  `/etc/sandbox-persistent.sh` make `lmullen-agent <lincoln+agent@lincolnmullen.com>` both
  author and committer, so commit without `--author`. Do **not** set `user.name` or
  `user.email` in the repository's git config: the sandbox mounts the host's checkout, so
  that would relabel the user's own commits too. If `git var GIT_AUTHOR_IDENT` names
  anyone else, stop and tell the user rather than committing.
- **Trailer:** keep the usual `Co-Authored-By: Claude ... <noreply@anthropic.com>` trailer
  at the end of the commit message.
- **Comments, PR bodies, and issue text:** the sandbox's GitHub token belongs to
  `lmullen-agent`, so `gh` posts as that account and no attribution line is needed. If
  `gh api user --jq .login` names anyone else, stop and tell the user rather than posting.
- **Reviews:** because Claude's PRs are opened by `lmullen-agent`, `lmullen` can approve
  them, which the ruleset on `main` requires. Merge only when the user asks, after their
  approval, with a merge commit (not squash); don't reach for `--admin`.

## CI

GitHub Actions (`.github/workflows/go.yml`) runs on push/PR to main:

- `go build -v ./...`
- `go test -v ./...`
