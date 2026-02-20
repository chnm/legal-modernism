# CLAUDE.md

## Project overview

Legal Modernism studies American legal history through computational methods. Data is drawn from the Making of Modern Law, the Caselaw Access Project, and custom datasets. Data is stored in PostgreSQL. Code is written in Go. The project website is built with Hugo.

## Repository structure

Programs (Go binaries):

- `adj2edge/` — Convert network adjacency list to edge list
- `cap-import/` — Import data from the Caselaw Access Project
- `cite-detector-moml/` — Detect citations in the *Making of Modern Law* treatises
- `cite-predictor/` — Augment citation detection using generative AI
- `cite-linker/` — Link detected citations to a database of caselaw

Support directories:

- `db/` — Database migrations, schema, and queries
- `go/` — Shared Go packages (citations, db, sources)
- `scripts/` — One-off R and shell scripts for data manipulation
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

**Logging:** Use the `slog` package for structured logging. Output JSON to stderr. Control log level with the `LAW_DEBUG` environment variable. Domain objects should provide a `LogID()` method returning `[]any` key-value pairs for consistent structured log context. Example:

```go
slog.Info("processed batch", batch.LogID()...)
slog.Error("batch failed", batch.LogID("error", err)...)
```

**Repository pattern:** Database access uses interface-based `Store` types (see `go/citations/store.go`, `go/sources/store.go`). Implementations wrap `*pgxpool.Pool`.

**Error handling:** Wrap errors with context using `fmt.Errorf("context: %w", err)`. Define sentinel errors in `errors.go` files (see `go/sources/errors.go`).

**Concurrency:** Use `gammazero/workerpool` for parallel processing. Use `context.Context` with timeouts for database operations and graceful shutdown via signal handling.

## Database

- PostgreSQL, connected via `go/db/db.go` using pgx v4 connection pooling
- Connection string from `LAW_DBSTR` environment variable
- Migrations managed by [dbmate](https://github.com/amacneil/dbmate) in `db/migrations/`
- Migration naming: `YYYYMMDDHHMMSS_description.sql`
- Full schema: `db/schema.sql`
- Schemas: `cap`, `cap_citations`, `english_reports`, `legalhist`, `moml`, `moml_citations`, `stats`, `sys_admin`, `textbooks`

## Environment variables

- `LAW_DBSTR` — PostgreSQL connection string
- `LAW_DEBUG` — Set to `debug` or `true` for debug-level logging

## Website

Hugo static site in `website/`. Uses Bootstrap 5 via CDN, no external Hugo theme.

- `make preview` — Dev server on port 54321
- `make build` — Production build with minification
- `make deploy` — Build and rsync to production

## CI

GitHub Actions (`.github/workflows/go.yml`) runs on push/PR to main:

- `go build -v ./...`
- `go test -v ./...`
