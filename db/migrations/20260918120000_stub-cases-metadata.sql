-- migrate:up
SET ROLE = law_admin;

-- Refine the structure of legalhist.stub_cases (issue #320).
--
-- A stub is a record of a case that likely exists but that no source we hold
-- can supply (issue #248). The table was created carrying four statistics
-- about how the corpus cites each stub -- n_citations, n_treatises,
-- first_cited_year, last_cited_year -- and an updated_at that only recorded
-- when those counts last changed. None of that is a fact about the case. It
-- is derived from citation_links, it can be recomputed there at any time, and
-- keeping it here made the registry look like a statistics table. This drops
-- the statistics and their index, so a stub row records identity only: the
-- cite string it is keyed by, the reporter, year, volume and page that string
-- was built from, and when the row was first minted.
--
-- What a case record needs next is the metadata the cite string does not
-- carry: the parties, the year it was decided, the jurisdiction. Those are
-- added as a separate table, legalhist.stub_case_metadata, keyed by the same
-- cite string, rather than as columns on stub_cases, because the two have
-- different lifecycles. stub_cases is derived: db/stub_cases.sql (make
-- db-stubs) rebuilds it from the linker's misses and prunes any row that
-- falls below the threshold or whose reporter gains a real source, and the
-- seed migrations that remap a reporter's spellings have deleted its stubs
-- outright. Metadata is curated -- filled in by hand or by a later pass, and
-- expensive to produce -- and must survive all of that. The foreign key from
-- stub_case_metadata to stub_cases is ON DELETE NO ACTION, so the database
-- refuses to delete, truncate or drop a stub that has metadata: the refresh
-- skips annotated stubs when it prunes, and a migration that removes or
-- renames stubs has to move their metadata first (an UPDATE of the cite
-- cascades) or delete it deliberately. An annotated stub is therefore
-- permanent, even below the threshold: the threshold was only ever a proxy
-- for "this string is a case", and metadata settles that directly. The
-- linker never consults the registry for a reporter that has a source, so a
-- kept stub in such a reporter is inert.
--
-- stub_cases.year is not year_decided. year is the citation year that forms
-- part of the key for a reporter cited by year, "[1905] 2 K.B. 1" (issues
-- #312 and #314), and is NULL for every other stub. year_decided is the
-- decision date of the case, whatever the reporter.
--
-- The three metadata columns are NULL to begin with; nothing populates them
-- yet. jurisdiction follows the scheme legalhist.reporters.jurisdiction uses,
-- 'us:ny', 'uk:kb'. A row must carry at least one value, since an empty row
-- would say nothing and would only block a prune.

DROP INDEX IF EXISTS legalhist.idx_stub_cases_n_citations;

ALTER TABLE legalhist.stub_cases
  DROP COLUMN IF EXISTS n_citations,
  DROP COLUMN IF EXISTS n_treatises,
  DROP COLUMN IF EXISTS first_cited_year,
  DROP COLUMN IF EXISTS last_cited_year,
  DROP COLUMN IF EXISTS updated_at;

CREATE TABLE IF NOT EXISTS legalhist.stub_case_metadata (
    cite         text PRIMARY KEY
                 REFERENCES legalhist.stub_cases(cite) ON UPDATE CASCADE,
    party_names  text,
    year_decided integer CHECK (year_decided BETWEEN 1000 AND 2100),
    jurisdiction text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT stub_case_metadata_some_value_check
      CHECK (party_names IS NOT NULL OR year_decided IS NOT NULL OR jurisdiction IS NOT NULL)
);

-- migrate:down
SET ROLE = law_admin;

DROP TABLE IF EXISTS legalhist.stub_case_metadata;

-- Restore the statistics columns with their original definitions. Existing
-- rows have no counts, so n_citations and n_treatises are added with a
-- placeholder default of 1 to satisfy NOT NULL and the check, and the default
-- is then dropped so the column matches its original definition. The pre-#320
-- db/stub_cases.sql rewrites every row's counts on its next run.
ALTER TABLE legalhist.stub_cases
  ADD COLUMN IF NOT EXISTS n_citations integer NOT NULL DEFAULT 1
      CONSTRAINT stub_cases_n_citations_check CHECK (n_citations > 0),
  ADD COLUMN IF NOT EXISTS n_treatises integer NOT NULL DEFAULT 1
      CONSTRAINT stub_cases_n_treatises_check CHECK (n_treatises > 0),
  ADD COLUMN IF NOT EXISTS first_cited_year integer,
  ADD COLUMN IF NOT EXISTS last_cited_year integer,
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE legalhist.stub_cases
  ALTER COLUMN n_citations DROP DEFAULT,
  ALTER COLUMN n_treatises DROP DEFAULT;

CREATE INDEX IF NOT EXISTS idx_stub_cases_n_citations
    ON legalhist.stub_cases (n_citations DESC);
