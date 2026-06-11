-- migrate:up
SET ROLE = law_admin;

-- A precomputed cite -> cap_case_id crosswalk covering every parallel citation
-- form of each decision FreeLaw (CourtListener) knows about. The cite-linker
-- loads this into memory and consults it as a fallback after the exact
-- cap.citations lookup misses: if any parallel form of a decision is in our CAP
-- data, we can reach the CAP case from every form (e.g. a treatise citing a
-- reporter form CAP doesn't index directly). Keyed on the same
-- "{volume} {reporter} {page}" string the linker builds (freelaw.citations.cite,
-- a generated column), so no new matching machinery is needed.
--
-- Two routes resolve a cluster to a CAP case:
--   Route A: freelaw.clusters_to_cap.cap_case_id (the direct ~99.8% path).
--   Route B: for clusters Route A cannot resolve (NULL cap_case_id, or no
--            clusters_to_cap row at all), recover a cap_case_id via a sibling
--            parallel cite of the same cluster that exists in cap.citations.
--
-- A cite is kept only if it resolves UNAMBIGUOUSLY to a single cap_case_id.
-- ~6% of linkable cites map to 2+ CAP cases — dominated by memorandum/table
-- pages (e.g. "108 S. Ct. 185" lists 6 cert denials; "234 A.D. 748" lists 13
-- memorandum decisions) where the cite cannot identify one case. Linking those
-- to an arbitrary case would inject systematic errors, so they are dropped.
--
-- lexis/west/journal citation types are excluded: these are database-ID cites
-- ("5 WL 55", "5 LEXIS 55", journal cites) that the linker never constructs from
-- a reporter name, so they would only bloat the in-memory map.
--
-- This is a SOURCE crosswalk: it depends on the freelaw and cap data, not on
-- linker output, so it is NOT refreshed by the cite-linker on each run. Build it
-- WITH NO DATA and populate it with `make db-refresh` (which auto-discovers every
-- materialized view) or:
--   REFRESH MATERIALIZED VIEW freelaw.cite_to_cap;
-- The build runs two large joins and takes several minutes.
CREATE MATERIALIZED VIEW IF NOT EXISTS freelaw.cite_to_cap AS
WITH relevant AS (
  SELECT cite, cluster_id
  FROM freelaw.citations
  WHERE type NOT IN ('lexis', 'west', 'journal')
),
cluster_cap AS (
  -- Route A: direct cluster -> CAP mapping.
  SELECT cluster_id, cap_case_id
  FROM freelaw.clusters_to_cap
  WHERE cap_case_id IS NOT NULL
  UNION
  -- Route B: clusters Route A cannot resolve, recovered via a sibling parallel
  -- cite present in cap.citations.
  SELECT r.cluster_id, capc."case" AS cap_case_id
  FROM relevant r
  JOIN cap.citations capc ON capc.cite = r.cite
  WHERE NOT EXISTS (
    SELECT 1 FROM freelaw.clusters_to_cap a
    WHERE a.cluster_id = r.cluster_id AND a.cap_case_id IS NOT NULL
  )
),
cite_caps AS (
  -- Map every parallel cite form to the cap_case_id(s) of its cluster.
  SELECT r.cite, cc.cap_case_id
  FROM relevant r
  JOIN cluster_cap cc ON cc.cluster_id = r.cluster_id
)
SELECT cite, min(cap_case_id) AS cap_case_id
FROM cite_caps
GROUP BY cite
HAVING count(DISTINCT cap_case_id) = 1
WITH NO DATA;

-- Unique index on the lookup key (cite is the GROUP BY key, unique by
-- construction). Required for the in-memory load to be a clean cite -> id map and
-- keeps REFRESH MATERIALIZED VIEW CONCURRENTLY available.
CREATE UNIQUE INDEX IF NOT EXISTS freelaw_cite_to_cap_uq
  ON freelaw.cite_to_cap (cite);

-- migrate:down
SET ROLE = law_admin;

DROP MATERIALIZED VIEW IF EXISTS freelaw.cite_to_cap;
