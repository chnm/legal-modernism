-- migrate:up
SET ROLE = law_admin;

-- Stub cases: records for the cases that treatises cite often but that no
-- source we hold can supply (issue #248, implementing the plan in #198).
--
-- 197 whitelisted reporters have no target dataset at all: none of their
-- citations links, and every one of their no_match rows is us_reporter_absent
-- or uk_reporter_absent, so no probed spelling of the reporter appears in CAP,
-- the FreeLaw crosswalk, the code reporter, or the English Reports. Together
-- they hold 5.05M citations (measured 2026-09-06), dominated by the post-1865
-- English Law Reports era -- L.T., Ch. D., L.J. Ch., L.J. K.B., Q.B.D.,
-- App. Cas. -- and the American Bankruptcy Reports. A cite string that recurs
-- across the corpus is evidence of a case even when nothing else is known
-- about it: "14 L.R.A.C. 337", cited 1,001 times by 524 treatises, is Derry v.
-- Peek. This table gives such a string a record that the linker can point at.
--
-- A row is identified by its cite string, "{volume} {reporter_standard}
-- {page}", the form citation_links.cite_cleaned takes, so the two join
-- directly. The identity is approximate in the way #248 describes: within one
-- of these reporters a first-page cite and a pin cite to an interior page are
-- indistinguishable, so "34 L.T. 100" and "34 L.T. 102" may be one case. On
-- the covered reporters, where the answer is known, 80% of distinct cite
-- strings cited ten or more times are first pages (US 90%, UK 55%), and no
-- clustering rule on page distance separates the rest: two heavily-cited
-- adjacent pages of one volume are different cases 49% of the time. So this
-- keys on the string, records the counts, and leaves clustering and case-name
-- extraction (#99) for later passes. Two further limits are known and left in:
-- a reporter cited by year rather than by volume (K.B. after 1901, and the
-- I.R. spellings whitelisted under L.R.Ir.) yields one string for every case
-- at that volume and page across the years, and a truncated whitelist spelling
-- ("Am. B." for "Am. B. R.") lets a misread trailing letter become a small
-- page number. Both are whitelist questions, tracked on #248.
--
-- The table is populated by db/stub_cases.sql (make db-stubs), not by the
-- linker and not as a materialized view. A plain table keeps a stable row per
-- case that later passes can annotate -- a case name from the surrounding
-- text, a link to a real record if a dataset is imported -- and the refresh
-- upserts the counts rather than replacing the rows. Rows are pruned when
-- they fall below the threshold or their reporter gains a real source.
CREATE TABLE IF NOT EXISTS legalhist.stub_cases (
    cite              text PRIMARY KEY,
    reporter_standard text NOT NULL REFERENCES legalhist.reporters(reporter_standard),
    -- For a single-volume reporter the refresh writes the volume-less
    -- detected form as volume 1, the same equivalence the linker's
    -- volumeForms applies, so one case gets one row.
    volume            integer NOT NULL CHECK (volume > 0),
    page              integer NOT NULL CHECK (page > 0),
    -- Citations to this string across the treatise corpus (raw: a page that
    -- cites the case twice counts twice) and the distinct treatises among
    -- them. n_citations is what the threshold applies to; n_treatises is kept
    -- so a stricter definition can be evaluated without a rebuild.
    n_citations       integer NOT NULL CHECK (n_citations > 0),
    n_treatises       integer NOT NULL CHECK (n_treatises > 0),
    -- Publication years of the earliest and latest treatises that cite it.
    -- The earliest bounds the decision date from above, which is the most
    -- useful clue for finding the case.
    first_cited_year  integer,
    last_cited_year   integer,
    created_at        timestamptz NOT NULL DEFAULT now(),
    -- When a refresh last changed the counts, not when it last ran.
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT stub_cases_reporter_volume_page_uq UNIQUE (reporter_standard, volume, page)
);

CREATE INDEX IF NOT EXISTS idx_stub_cases_n_citations
    ON legalhist.stub_cases (n_citations DESC);

-- citation_links gains the stub as a fourth link target, beside cap_case_id,
-- code_reporter_id and er_case_id. The column carries the stub's key, which is
-- the cite string itself, so a numeric id would add nothing.
--
-- Deliberately no foreign key. The registry is derived from citation_links: a
-- refresh counts the linker's misses together with the existing stub links,
-- and drops the rows that no longer qualify, which a foreign key from this
-- table would forbid for as long as stale links point at them. The linker
-- re-establishes consistency on its next full rebuild, exactly as it does
-- after a whitelist change; until then a linked_stub row whose stub is gone
-- is a stale link, not a broken one.
ALTER TABLE moml_citations.citation_links
  ADD COLUMN IF NOT EXISTS stub_cite text;

CREATE INDEX IF NOT EXISTS idx_citation_links_stub
    ON moml_citations.citation_links (stub_cite) WHERE stub_cite IS NOT NULL;

-- A stub link records the success tier stub_direct, like the other targets.
-- The linker consults the registry last, and only for a citation whose
-- failure tier would have been reporter_absent, so a stub can never outrank a
-- real case and a stale registry cannot capture a reporter that has since
-- gained a source.
ALTER TABLE moml_citations.citation_links
  DROP CONSTRAINT IF EXISTS chk_citation_links_match_tier;
ALTER TABLE moml_citations.citation_links
  ADD CONSTRAINT chk_citation_links_match_tier CHECK (
    match_tier IS NULL OR match_tier IN (
      -- no_match, US route (CAP -> FreeLaw -> alternate spellings -> code reporter)
      'us_reporter_absent',       -- no probed reporter spelling appears in any US source
      'us_diffvols_missing',      -- renumbers in CAP, but no reporters_diffvols row for this volume
      'us_volume_absent',         -- reporter present, this volume never appears
      'us_volume_missing',        -- #261: reporter present, but the citation carries no volume
      'us_page_absent',           -- reporter and volume present, page is not a first-page cite
      'us_page_ambiguous',        -- #242: page falls inside two or more case spans
      'us_page_gap',              -- #242: page falls inside no case span in a covered volume
      -- no_match, UK route (English Reports)
      'uk_reporter_absent',
      'uk_volume_absent',
      'uk_volume_missing',        -- #261
      'uk_page_absent',
      'uk_page_ambiguous',        -- #256
      'uk_page_gap',              -- #243
      -- linked_*: which probe produced the link
      'cap_direct',               -- cap.citations, under the normalized cite
      'cap_freelaw',              -- freelaw.cite_to_cap, under the normalized cite
      'cap_alt_spelling',         -- cap.citations, under a reporters_abbreviations alternate
      'cap_freelaw_alt_spelling', -- freelaw.cite_to_cap, under an alternate
      'cap_page_interior',        -- #242: unique containment in a case's page span
      'code_direct',              -- legalhist.code_reporter, under the cleaned cite
      'er_direct',
      'er_page_interior',         -- #243
      'stub_direct'               -- #248: legalhist.stub_cases, under the cleaned cite
    )
  );

-- migrate:down
SET ROLE = law_admin;

-- Fold every stub link back into the no_match it came from. The linker only
-- links to a stub when the failure tier would have been reporter_absent on the
-- citation's route, and the route is the reporter's jurisdiction, so the
-- original row can be reconstructed exactly.
UPDATE moml_citations.citation_links cl
   SET status = 'no_match',
       match_tier = CASE WHEN r.jurisdiction LIKE 'uk%'
                         THEN 'uk_reporter_absent'
                         ELSE 'us_reporter_absent' END,
       stub_cite = NULL,
       cite_linked = NULL
  FROM moml_citations.citations_unlinked cu
  JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr
  LEFT JOIN legalhist.reporters r ON r.reporter_standard = wl.reporter_standard
 WHERE cu.id = cl.citation_id
   AND cl.status = 'linked_stub';

ALTER TABLE moml_citations.citation_links
  DROP CONSTRAINT IF EXISTS chk_citation_links_match_tier;
ALTER TABLE moml_citations.citation_links
  ADD CONSTRAINT chk_citation_links_match_tier CHECK (
    match_tier IS NULL OR match_tier IN (
      'us_reporter_absent',
      'us_diffvols_missing',
      'us_volume_absent',
      'us_volume_missing',
      'us_page_absent',
      'us_page_ambiguous',
      'us_page_gap',
      'uk_reporter_absent',
      'uk_volume_absent',
      'uk_volume_missing',
      'uk_page_absent',
      'uk_page_ambiguous',
      'uk_page_gap',
      'cap_direct',
      'cap_freelaw',
      'cap_alt_spelling',
      'cap_freelaw_alt_spelling',
      'cap_page_interior',
      'code_direct',
      'er_direct',
      'er_page_interior'
    )
  );

DROP INDEX IF EXISTS moml_citations.idx_citation_links_stub;
ALTER TABLE moml_citations.citation_links
  DROP COLUMN IF EXISTS stub_cite;

DROP TABLE IF EXISTS legalhist.stub_cases;
