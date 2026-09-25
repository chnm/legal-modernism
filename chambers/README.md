# Chambers

Internal web app for browsing the citation data of the Legal Modernism project. Named after [judicial chambers](https://en.wikipedia.org/wiki/Chambers_(law)). It runs at chambers.legalmodernism.org.

Chambers shows the citations that the treatises of the *Making of Modern Law* make to reported cases, by the entities they connect:

- **Treatises.** A *work* (`moml.works`) is a treatise across all its editions, later editors' revisions, translations and derivatives included. An *edition* (`moml.editions`, keyed by `bibliographicid`) is one printing, in one or more *volumes* (`moml.volumes`, keyed by `psmid`), each of which is a sequence of scanned *pages* (`moml.page`, keyed by `psmid` and `pageid`). Only the editions in the treatise view (`moml.treatises`, US and UK) are listed and counted; a work is shown when at least one of its editions is in the view.
- **Cases.** A case is whatever a linked citation reached: a record in the Caselaw Access Project (`cap.cases`), the English Reports (`english_reports.cases`), the Code Reporter (`legalhist.code_reporter`), or a *stub case* (`legalhist.stub_cases`): a cite string no source holds, recorded because it recurs across the corpus. Cases are addressed by source and id, `cap:6754004` or `stub:1 Ch. 1`.
- **Reporters.** The series of law reports the treatises cite (`legalhist.reporters`, keyed by `reporter_standard`). A reporter connects to cases only through citations: the spelling a treatise used is mapped to a standard by `legalhist.whitelist`, and the linker resolves the cite.
- **Citations.** A *citation* is one string the detector found on a page (`moml_citations.citations_unlinked`); its *link* (`moml_citations.citation_links`) is what the linker made of it: a status, a match tier, and at most one case. Aggregated, a citation is an edge from an edition to a case with a count (`moml_citations.edition_case_citations`, "this edition cites this case N times"), which is what the rankings and the top-cases tables are built from.

## Running

```bash
go run ./chambers/            # default port 4567
go run ./chambers/ --port 8080
make chambers                 # air, with live reload (chambers/.air.toml)
```

Requires a database connection configured via `LAW_DBSTR` or the individual `LAW_DB_*` variables (see the repository [README](../README.md#configuration)). Every page only reads, so the read-only connection works for a local check: `LAW_DBSTR="$LAW_CLAUDE" go run ./chambers/`.

Deployment: `Dockerfile` builds the binary into a distroless image; `.github/workflows/cicd--chambers.yml` builds and deploys it on a push to `main` that touches `chambers/`, `go/` or the module files.

## Pages

| Route | Page |
|---|---|
| `/` | The five sections. Reads nothing from the database. |
| `/works` | Works with an edition in the treatise view: title, author, editions, years, US/UK, citations, linked share, cases. `q` searches title and author; `jur` is `us`, `uk` or `both`; `sort` is `cites`, `cases`, `editions`, `earliest` or `title`. |
| `/works/{id}` | A work: its editions (those outside the treatise view muted, derivatives flagged), a chart of citations per edition, and the cases cited most across its editions, with how many editions cite each. `pincite=exclude` leaves out editions that reach a case only through pin cites. |
| `/editions/{bibliographicid}` | An edition: the catalogue record, its subjects and LoC headings, links to Gale, its citation totals, the reporters it cites and their US/UK share, the cases it cites most, and every page on which a citation was found, volume by volume, with the page's type and section. |
| `/editions/{bibliographicid}/cases` | Every case the edition cites, paginated; `sort` is `cites`, `year` or `name`. |
| `/pages/{psmid}/{pageid}` | A page: the text as the detector read it, with every citation it found marked, and the citations in the order they appear with their links; `?text=original` shows the scan's OCR text unmarked. Previous and next page, and previous and next page with citations. |
| `/cases` | Cases ranked by the treatise editions that cite them, with works and citations alongside; `source` filters (`cap`, `er`, `code`, `stub`); `sort` is `editions`, `works` or `cites`; `pincite=exclude` ranks by editions that reach the case other than only through pin cites. `q` is looked up as an exact cite in every source, then searched as a name. |
| `/cases/{source}/{id}` | A case: its record, how many editions and works cite it, citing editions by decade, the spellings it was cited under with their tiers and the share found on index and other non-body pages, and the citing editions, paginated. |
| `/reporters` | The reporters with their spellings, citations, linked share and no-match count; `jur`, `type`, `q`, `sort`. |
| `/reporters/{standard}` | A reporter: its record, its spellings with how often each was found, its alternate abbreviations and CAP renumbering, its citations by status and tier (each linking to the citations), the cases reached through it, the editions citing it most, and its unmatched cites that recur. |
| `/citations` | The citation list. See the parameters below. |
| `/citations/{uuid}` | One citation: what was detected, what the linker did, the case, and the page it was found on with the citation marked. |
| `/linking` | The linking dashboard: what was skipped, what linked and to which source, by reporter, and the match tiers. |
| `/linking/tiers` | The match tiers charted over the corpus and by reporter. |
| `/linking/whitelist` | The whitelist extender: the spellings the detector found most often that the whitelist lacks, with candidate standards, producing CSVs for the seed migrations. It writes nothing. |
| `/api/linking-dashboard`, `/api/tiers`, `/api/whitelist-extender` | The JSON behind the three linking pages, cached for an hour. |

The routes of the app before the redesign (`/treatises`, `/treatise`, `/case`, `/cite`, `/reporters/check`, `/unmatched`, `/normalized`, `/linking-dashboard`, `/tiers`, `/whitelist-extender`) redirect permanently to their new homes; `redirects.go` has the table.

### The citation list

`/citations` runs one of four query shapes, each served by an index, and shows the first 500 rows:

- `cite=<normalized cite>` finds the citations whose normalized form (the whitelist's standard spelling of the reporter, with the volume and page, after any renumbering into CAP: `2 Mass. 420`) is that string. `reporter=`, `volume=`, `page=` and optionally `year=` (for a reporter cited by year) are turned into the same lookup, including the renumbered form of a reporter in `reporters_diffvols`.
- `reporter=<standard>` alone returns a sample of the reporter's citations, in no particular order, through every spelling the whitelist maps to it.
- `edition=<bibliographicid>` returns the citations in an edition, in page order; add `case=<source:id>` for the citations from that edition to that case.
- `case=<source:id>` returns the citations linked to a case.

`status=` and `tier=` narrow any shape (`status=unprocessed` is a citation with no link row). `id=<uuid>` redirects to the citation's page.

## Data freshness

Two kinds of query serve the pages. Live queries read a page, a citation, a volume's pages or a case's citations, each bounded by an index: the unique index on `citations_unlinked (moml_treatise, moml_page, …)` for a volume or page, the partial indexes on the case-id columns of `citation_links` for a case, `cite_normalized` for a cite string, `reporter_abbr` for a reporter's spellings. Everything aggregated comes from materialized views, refreshed by `make db-maintenance` and so as of the last refresh, not the last linker run:

| View | Grain | Used by |
|---|---|---|
| `moml_citations.edition_citation_counts` | one treatise edition: the treatise view materialized, with its work, counts and cases | every list and per-case join |
| `moml_citations.work_citation_counts` | one work with a treatise edition | `/works`, `/works/{id}` |
| `moml_citations.edition_case_citations` | one edition and one case it cites (issue #213) | top cases, citing editions |
| `moml_citations.case_edition_counts` | one case, with its name, year and cite copied in, ranked by citing editions | `/cases`, case summaries |
| `moml_citations.reporter_case_citations` | one reporter and one case reached through it | reporter pages |
| `moml_citations.edition_reporter_citations` | one edition and one reporter it cites | edition and reporter pages |
| `moml_citations.treatise_citation_counts` | one volume's citation totals | (through `edition_citation_counts`) |
| `moml_citations.citations_unmatched_top` | one unmatched cite string that recurs five times or more | reporter pages |
| `moml_citations.linking_dashboard_summary`, `_reporters`, `_tiers` | the corpus, a reporter, a reporter's status and tier | linking pages, reporter pages |
| `legalhist.top_reporters` | one detected spelling and its count | reporter spellings, whitelist extender |

A section that reads a view which does not exist yet or has not been populated (the states between deploying, applying a migration and running `make db-maintenance`) renders empty with a notice naming the view, instead of failing the page; `isUnavailable` in `server.go` recognizes the two SQLSTATEs.

`moml.treatises` itself is a view over regexes and subject joins that costs about a second to scan, which is why the lists read the materialized copy; looking one edition up in it by id is fast, and the edition, page and citation pages do that to say whether the edition is in the view.

## Things to keep in mind when reading the pages

- **Page text.** The page view shows the text the detector read: the scan's OCR text with the corrections of `legalhist.ocr_corrections` applied and the Law Reports series prefix moved behind the volume ("L. R. 5 Ch. 100" to "5 L. R. Ch. 100"), in that order, as `cite-detector-moml` does. A citation's raw string was cut from that text, so it can be marked. The corrections are loaded once per process; a change to the table needs a restart. A raw string that no longer occurs verbatim is looked for with its whitespace relaxed, and one still not found is listed after the located ones, unmarked.
- **Page identity.** `pageid` values repeat across volumes; every join of `moml.page`, `moml.page_ocrtext` and `moml.page_content` is on `psmid` and `pageid` together.
- **Rankings are led by noise.** Tables of cases in the treatises' indexes read "Taylor, 1", "Taylor, 2", and the detector for a single-volume reporter such as Taylor's North Carolina Reports fires on them, so a page-one case like *Bradberry v. Hooks* tops the ranking with thousands of citing editions. The case page makes this visible: the spellings it was cited under, the tiers they linked through, and the share of the citations found on index and other non-body pages. `pincite=exclude` and the works count are there to look past it; the fix belongs to the detector.
- **Pin cites.** An edition that reaches a case only through pin cites to its interior pages (`edition_case_citations.pincite_only`) is marked *pin*; issue #242 found many such links to be OCR noise rather than pin cites.
- **Statutes and junk.** Statute series and junk spellings are turned away before the linker probes anything, so they carry no tier and are not counted among a reporter's citations.

## Architecture

Single-binary Go web server using `net/http` (Go 1.22 route patterns, no router), `html/template`, and `embed.FS` for the templates and static files.

| File | Purpose |
|---|---|
| `main.go` | Flags, logger, connection pool, server with timeouts, signal shutdown |
| `server.go` | The `server` type every handler hangs off: rendering, error pages, timeouts, the unavailable-view check, the OCR replacer, and `collect`, the generic row scanner |
| `routes.go` | Every route, the static files, the legacy redirects, and the home page |
| `works.go`, `editions.go`, `pages.go`, `cases.go`, `reporters.go`, `citations.go` | One entity each: its types, its queries and its handlers |
| `linking.go`, `whitelist.go` | The Detecting & linking section and its JSON APIs |
| `casemeta.go` | `CaseRef` and the SQL fragments that join the four case sources the same way everywhere |
| `vocabulary.go` | The statuses and tiers: labels, glosses, colours and order, keyed by the constants in `go/citations`, and handed to the linking pages' scripts |
| `links.go` | Every URL Chambers builds for itself, and the Gale links |
| `format.go`, `pagination.go`, `templates.go`, `redirects.go`, `logger.go` | Template functions, paging, template parsing, the redirect table, the JSON logger |

Templates live in `templates/`. `baseof.html` is the layout (navbar, breadcrumbs, notices, shared CSS); the files whose names begin with an underscore are partials (`pagination`, `chip`, `case`, `top_cases`, `notice`, `linking_nav`); every other file is a page and is parsed with the layout and the partials, discovered rather than listed. Charts use Observable Plot from esm.sh; the work and case pages embed their data in the page, the linking pages fetch theirs from the APIs.

## Tests

`go test ./chambers/` needs no database. The tests parse every template and render the database-free pages, check that every `Tier*` and `Status*` constant in `go/citations` has a vocabulary entry, exercise the citation locating and highlighting, the LoC heading grouping, the URL builders, the citation-filter shapes, pagination and formatting, and request the home page, the static files and every legacy redirect through `httptest`.

To check a page against the database, run the app with the read-only connection on a spare port and `curl` it; the queries that matter for speed are bounded as described above, and the largest edition (seven volumes, 285K citations) renders in about three seconds.

## Adding a page

1. Put its types, queries and handler in the entity's file (or a new one). Query through `collect`; read aggregates from a view and wrap the call in `optional` so an unpopulated view becomes a notice.
2. Add the template to `templates/`; it is parsed automatically. Give the handler's data a `Page` (title, section, crumbs).
3. Register the route in `routes.go`, and build its URL in `links.go` if other pages link to it.
4. Add a test if there is logic that does not need the database.
