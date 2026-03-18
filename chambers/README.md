# Chambers

Internal web app for inspecting citation data from the Legal Modernism project. Named after [judicial chambers](https://en.wikipedia.org/wiki/Chambers_(law)).

## Running

```bash
go run ./chambers/            # default port 4567
go run ./chambers/ --port 8080
gow run ./chambers/           # auto-restart on file changes
```

Requires a database connection configured via `LAW_DBSTR` or the individual `LAW_DB_*` environment variables (see the repository [README](../README.md#configuration)).

## Architecture

Single-binary Go web server using `net/http` (no external router), `html/template` for rendering, and `embed.FS` for bundling templates and static files into the binary.

### Files

| File | Purpose |
|------|---------|
| `main.go` | Server setup, route registration, HTTP handlers, template function map |
| `queries.go` | Structs, SQL queries, and database access functions |
| `templates.go` | `embed.FS` declarations for `templates/` and `static/` |
| `logger.go` | `slog` JSON logger initialization (standard project pattern) |

### Routes

| Route | Handler | Template | Description |
|-------|---------|----------|-------------|
| `GET /` | `handleHome` | `home.html` | Landing page with nav links |
| `GET /cite` | `handleCiteLookup` | `cite-lookup.html` | UUID input form |
| `GET /cite?id={uuid}` | `handleCiteLookup` | `detail.html` | Full citation detail page |
| `GET /reporters` | `handleReporters` | `reporters.html` | List all standard reporters |
| `GET /reporters/check?r={name}` | `handleReporterCites` | `reporter-cites.html` | Color-coded citations for a reporter |
| `GET /static/...` | `http.FileServer` | — | Embedded static files (portrait image) |

### Templates and static files

All templates live in `templates/*.html` and are embedded via `//go:embed templates/*.html`. They are parsed once at startup in `main()` with a `funcMap` that provides:

- `deref` — dereference `*int` or `*string` pointers
- `derefStr` — dereference `*string` to string (empty string if nil)
- `ptrOr` — dereference `*string` or return an HTML fallback (e.g., `&mdash;`), returns `template.HTML`
- `highlightRaw` — HTML-escape OCR text then wrap the raw citation string in `<mark>` tags, returns `template.HTML`

Static files in `static/` are embedded via `//go:embed static/*` and served at `/static/`.

Templates use inline `<style>` blocks (no shared CSS framework). Each page is self-contained.

### Database queries

**`getCitationDetail`** — The main query. A single SELECT joining 12 tables across 6 schemas to fetch everything about one citation: the raw detected cite, linking pipeline results, matched case info (from CAP, Code Reporter, or English Reports), and the MOML treatise page metadata and OCR text. Returns a `CitationDetail` struct with ~30 fields, most of which are nullable (`*string`, `*int`).

Key joins:
- `moml_citations.citations_unlinked` — the raw detected citation
- `moml_citations.citation_links` — linking result (status, cleaned/normalized/linked forms)
- `cap.cases` + `cap.reporters` + `cap.courts` + `cap.jurisdictions` — CAP case details
- `legalhist.code_reporter` — code reporter cases
- `english_reports.cases` — English Reports cases
- `moml.book_info` + `moml.book_citation` — treatise metadata (title, author, productlink)
- `moml.page` — page metadata (sourcepage number); joined on BOTH `psmid` and `pageid`
- `moml.page_ocrtext` — full OCR text; joined on BOTH `psmid` and `pageid`

The `moml.page` and `moml.page_ocrtext` joins must include `psmid` (treatise ID) in addition to `pageid`, because `pageid` values (e.g., `06870`) are reused across different treatises.

**`getReporterStandards`** — Distinct `reporter_standard` values with variant counts from the `legalhist.reporters_citation_to_cap` whitelist.

**`getReporterVariants`** — All `reporter_found` abbreviations for a given `reporter_standard`.

**`getCitesForReporter`** — Up to 10,000 raw citations whose `reporter_abbr` maps to a given `reporter_standard` via the whitelist, with linking status. Ordered by `cu.id` to get a mix of variants (not alphabetical, which would cluster one variant before the LIMIT).

### CitationDetail methods

- `HasLink()` — true if status is any `linked_*` value
- `IsCAP()` / `IsCodeReporter()` / `IsEnglishReports()` — which case source was matched
- `MomlVolumeURL()` — Gale MOML URL for the volume (transforms the stored `productlink` from old `link.galegroup.com` domain to `link.gale.com`, adds `u=viva_gmu`)
- `MomlPageURL()` — Same as volume URL but appends `&pg=N` where N is derived from `pageid` by stripping the trailing `0` and leading `0`s (e.g., `06870` becomes `687`)

### ReporterCite.StatusClass

Maps linking status to CSS classes for color-coded display:
- `status-linked` (green) — status starts with `linked`
- `status-nomatch` (red) — `no_match`
- `status-skip` (blue) — `skipped_junk` or `skipped_statute`
- `status-unprocessed` (yellow) — nil status or unknown

## Adding a new page

1. Add a query function and any needed structs to `queries.go`
2. Create a template in `templates/` (it will be auto-embedded)
3. Add a handler function in `main.go`
4. Register the route on the `mux` in `main()`
5. Add a nav link on `home.html`

Each page should include an explanatory `<p class="about">` paragraph and a collapsible `<details class="query">` element showing the SQL query used.
