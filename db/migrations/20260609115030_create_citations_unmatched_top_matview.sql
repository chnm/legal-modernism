-- migrate:up
SET ROLE = law_admin;

-- Aggregated, ranked list of MOML citations still to be linked. A citation is
-- "to be linked" when it either has no row in citation_links (no attempt
-- recorded) or its attempt resulted in status 'no_match'. Reporter
-- abbreviations are joined to the whitelist (excluding junk) to get the
-- standard reporter, then grouped by volume, standard reporter, and page so
-- each distinct normalized citation is counted once. Built WITH NO DATA; run
-- REFRESH MATERIALIZED VIEW to populate (see note in migrate:down).
CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.citations_unmatched_top AS
SELECT
  cu.volume,
  w.reporter_standard,
  cu.page,
  count(*) AS n
FROM moml_citations.citations_unlinked cu
LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
JOIN legalhist.whitelist w
  ON w.reporter_found = cu.reporter_abbr
 AND w.junk = false
WHERE cl.citation_id IS NULL      -- no link attempt recorded
   OR cl.status = 'no_match'      -- attempted but not matched
GROUP BY cu.volume, w.reporter_standard, cu.page
HAVING count(*) >= 5
ORDER BY n DESC
WITH NO DATA;

-- Unique index on the grouping key. Each (volume, reporter_standard, page) is
-- unique by construction (it is the GROUP BY key). NULLS NOT DISTINCT treats
-- the NULL volume of single-volume reporters as a single value. This unique
-- index is required for REFRESH MATERIALIZED VIEW CONCURRENTLY.
CREATE UNIQUE INDEX IF NOT EXISTS citations_unmatched_top_uq
  ON moml_citations.citations_unmatched_top (volume, reporter_standard, page) NULLS NOT DISTINCT;

-- Supports ranked reads / pagination (ORDER BY n DESC).
CREATE INDEX IF NOT EXISTS citations_unmatched_top_n_idx
  ON moml_citations.citations_unmatched_top (n DESC);

-- After this migration, populate the view once with a plain (non-concurrent)
-- refresh, since CONCURRENTLY cannot run against a never-populated view:
--   REFRESH MATERIALIZED VIEW moml_citations.citations_unmatched_top;
-- Subsequent refreshes (e.g. after a linking batch) can be non-blocking:
--   REFRESH MATERIALIZED VIEW CONCURRENTLY moml_citations.citations_unmatched_top;

-- migrate:down
DROP MATERIALIZED VIEW IF EXISTS moml_citations.citations_unmatched_top;
