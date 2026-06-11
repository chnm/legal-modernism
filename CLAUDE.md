# CLAUDE.md

## Project overview

Legal Modernism studies American legal history through computational methods. Data is drawn from the Making of Modern Law, the Caselaw Access Project, and custom datasets. Data is stored in PostgreSQL. Code is written in Go. The project website is built with Hugo.

## Repository structure

Programs (Go binaries):

- `adj2edge/` — Convert network adjacency list to edge list
- `cap-import/` — Import data from the Caselaw Access Project
- `cite-detector-moml/` — Detect citations in the *Making of Modern Law* treatises
- `cite-predictor/` — Augment citation detection using generative AI
- `chambers/` — Internal web server for inspecting citation data
- `cite-linker/` — Link detected citations to a database of caselaw

Support directories:

- `db/` — Database migrations, schema, and queries
- `go/` — Shared Go packages (citations, db, sources)
- `scripts/` — One-off R and shell scripts for data manipulation
- `slurm/` — Slurm batch job scripts for running programs on the HPC cluster
- `test-data/` — Sample data for development and testing
- `website/` — Hugo static website
- `notebooks/` — Analytical notebooks (R, Python, Quarto)
- `doi/` — DOI metadata records

## Go development

- Module: `github.com/lmullen/legal-modernism`
- Go version: 1.24
- Build: `go build ./cmd-name/`
- Run: `go run ./cmd-name/`
- Test: `go test ./...`
- Tests use `stretchr/testify` with table-driven test patterns
- Key dependencies: `jackc/pgx/v4` (PostgreSQL), `gammazero/workerpool` (concurrency), `schollz/progressbar` (CLI progress), `stretchr/testify` (testing)

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
slog.Info("reset complete", "deleted", n)          // final result → INFO
slog.Error("batch failed", batch.LogID("error", err)...)
```

**Repository pattern:** Database access uses interface-based `Store` types (see `go/citations/store.go`, `go/sources/store.go`). Implementations wrap `*pgxpool.Pool`.

**Error handling:** Wrap errors with context using `fmt.Errorf("context: %w", err)`. Define sentinel errors in `errors.go` files (see `go/sources/errors.go`).

**Concurrency:** Use `gammazero/workerpool` for parallel processing. Use `context.Context` with timeouts for database operations and graceful shutdown via signal handling.

## Database

- PostgreSQL 17.x, connected via `go/db/db.go` using pgx v4 connection pooling
- Connection string from `LAW_DBSTR` environment variable
- Migrations managed by [dbmate](https://github.com/amacneil/dbmate) in `db/migrations/`
- Migration naming: `YYYYMMDDHHMMSS_description.sql`
- Full schema: `db/schema.sql`
- Schemas: `cap`, `cap_citations`, `english_reports`, `legalhist`, `moml`, `moml_citations`, `stats`, `sys_admin`, `textbooks`
- Migrations should be idempotent: use `IF NOT EXISTS` / `IF EXISTS` guards on `CREATE INDEX`, `CREATE TABLE`, `ADD CONSTRAINT`, `DROP CONSTRAINT`, etc.
- Don't update `db/schema.sql`: this file is auto-generated.

### Writing a migration

1. Create `db/migrations/YYYYMMDDHHMMSS_description.sql` with `-- migrate:up` / `-- migrate:down` sections; the `down` must fully reverse the `up`. Start the `migrate:up` with `SET ROLE = law_admin;`, as the existing migrations do.
2. Before adding a constraint, verify existing rows won't violate it (query with the read-only `LAW_CLAUDE` connection); a constraint that current data violates fails to apply.
3. Apply with `make db-up`, then regenerate the schema with `make db-schema`. Both need write access via `LAW_DBSTR`, so Claude cannot run them — ask the user to.
4. Commit **both** the migration file and the regenerated `db/schema.sql`. "Don't update `db/schema.sql`" means don't hand-edit it, not don't commit it.

## Environment variables

- `LAW_DBSTR` — PostgreSQL connection string
- `LAW_CLAUDE` — Read-only PostgreSQL connection string for Claude
- `LAW_DEBUG` — Set to `debug` or `true` for debug-level logging

## Claude database access

- **Never use `LAW_DBSTR`.** When connecting to the database, always use the `LAW_CLAUDE` environment variable, which connects with a read-only user.
- **Read-only access only.** Only run `SELECT` queries. Never run `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `TRUNCATE`, `CREATE`, or any other command that writes to or modifies the database.

## Website

Hugo static site in `website/`. Uses Bootstrap 5 via CDN, no external Hugo theme.

- `make preview` — Dev server on port 54321
- `make build` — Production build with minification
- `make deploy` — Build and rsync to production

## Chambers

Internal web app for browsing and inspecting citation data. Located in `chambers/`.

- Build: `go build ./chambers/`
- Run: `go run ./chambers/` (default port 4567)
- Dev with live reload: `air` (configured via `chambers/.air.toml`)
- Templates and static files are embedded via `//go:embed`
- Routes registered on `http.NewServeMux()` in `main.go`
- To add a new page: add query in `queries.go`, create template in `templates/`, add handler and register route in `main.go`, add nav link to `home.html`

**Frontend:** Use the latest version of Bootstrap CSS via CDN (no Bootstrap JavaScript). Use Observable Plot (`@observablehq/plot`) for data visualizations when possible, and D3.js otherwise.

## GitHub

Use the `gh` CLI for GitHub operations — creating pull requests, viewing issues, checking CI status, and commenting. It is authenticated via the `GH_TOKEN` environment variable.

- Open a PR: `gh pr create --base main --head <branch> --title "..." --body "..."`
- Reference the issue a PR closes in the commit or PR body with `Closes #<n>` so it auto-closes on merge.
- View an issue: `gh issue view <n>`
- Check CI on a PR: `gh pr checks <n>`

Do not hand-build GitHub API calls with `curl`; prefer `gh`.

## CI

GitHub Actions (`.github/workflows/go.yml`) runs on push/PR to main:

- `go build -v ./...`
- `go test -v ./...`
