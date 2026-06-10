# freelaw-import

Load [CourtListener](https://www.courtlistener.com/) (Free Law Project) bulk data into the
`freelaw` schema of the database. This data improves the citation linker: many reporter
citations refer to the same decision (parallel citations), and CourtListener groups them under
a single *cluster*, which can in turn be mapped to a Caselaw Access Project (CAP) case. Together
these tables let the linker resolve a detected citation to a CAP case even when the exact
citation string is not present in `cap.citations` (see issue #197).

## What it loads

The program populates two tables from two CourtListener bulk exports:

- **`freelaw.citations`** — the parallel-citation crosswalk, from `citations-YYYY-MM-DD.csv.bz2`.
  One row per reporter citation (`volume`, `reporter`, `page`, a concatenated `cite`, the citation
  `type`, and the `cluster_id`). Rows that share a `cluster_id` are parallel citations for the
  same decision. The source `type` integer (1–9) is stored as its lowercased CourtListener
  constant name (`federal`, `state`, `state_regional`, `specialty`, `scotus_early`, `lexis`,
  `west`, `neutral`, `journal`).

- **`freelaw.clusters_to_cap`** — a crosswalk from a CourtListener opinion `cluster_id` to a CAP
  `cap_case_id`, from `opinion-clusters-YYYY-MM-DD.csv.bz2`. The CAP case id is parsed from the
  `filepath_json_harvard` column (e.g. `law.free.cap.us.562/1235.5910287.json` → `5910287`).
  A row is recorded for every cluster that has a Harvard/CAP filepath; `cap_case_id` is left
  `NULL` when the parsed CAP case is not present in `cap.cases`.

## How it works

Each file is streamed straight from its bzip2-compressed CSV form into a session-temporary
staging table via Postgres `COPY`, then transformed into the destination table with an
`INSERT … SELECT` (which does the `type` mapping and CAP-id extraction in SQL). Because the
files were produced by Postgres `COPY`, reading them back with `COPY` parses the large, multiline,
quoted text fields losslessly. Each load fully replaces its table inside a transaction, so the
program is safe to re-run.

## Getting the data

Download the bulk files from CourtListener's S3 bucket, for example:

```
https://com-courtlistener-storage.s3-us-west-2.amazonaws.com/bulk-data/citations-2026-03-31.csv.bz2
https://com-courtlistener-storage.s3-us-west-2.amazonaws.com/bulk-data/opinion-clusters-2026-03-31.csv.bz2
```

See the [bulk-data documentation](https://www.courtlistener.com/help/api/bulk-data/) for the
current files. The `opinion-clusters` export is large (~2.5 GB compressed).

## Usage

Apply the migration first (`make db-up`) so the `freelaw` schema exists, then run the importer
with `LAW_DBSTR` set. Either flag may be given on its own; pass both to load both tables.

```
go run ./freelaw-import/ \
    --citations tmp/citations-2026-03-31.csv.bz2 \
    --clusters tmp/opinion-clusters-2026-03-31.csv.bz2 \
    --progress
```

Flags:

- `--citations <path>`: path to the `citations-*.csv.bz2` file.
- `--clusters <path>`: path to the `opinion-clusters-*.csv.bz2` file.
- `--progress`: show a progress bar while reading each file.
