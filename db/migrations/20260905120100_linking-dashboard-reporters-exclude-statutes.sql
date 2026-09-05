-- migrate:up
SET ROLE = law_admin;

-- moml_citations.linking_dashboard_reporters counts, per whitelisted reporter,
-- the citations that linked, ended in no_match, or were never attempted. It
-- excludes junk by joining the whitelist with junk = false, but the statute
-- reporters seeded by 20260905120000_seed-statute-reporters.sql are non-junk
-- rows whose citations are all skipped_statute, a status none of the three
-- counts recognizes. Without this change each statute family would appear on
-- the dashboard as a reporter with zero of everything. Statutes are excluded
-- the way junk is, since neither is a potential case citation (issue #246).
--
-- A materialized view cannot be altered in place, so it is dropped and
-- recreated with the same columns chambers reads (reporter_standard, linked,
-- no_match, unprocessed, uk) and the same unique index. It is empty afterwards
-- until `make db-maintenance` refreshes it, which the post-linker maintenance
-- does for every materialized view without listing them.

DROP MATERIALIZED VIEW IF EXISTS moml_citations.linking_dashboard_reporters;

CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.linking_dashboard_reporters AS
SELECT
  wl.reporter_standard,
  count(*) FILTER (WHERE cl.status LIKE 'linked%') AS linked,
  count(*) FILTER (WHERE cl.status = 'no_match')   AS no_match,
  count(*) FILTER (WHERE cl.status IS NULL)         AS unprocessed,
  COALESCE(bool_or(r.jurisdiction LIKE 'uk:%'), false) AS uk
FROM moml_citations.citations_unlinked cu
JOIN legalhist.whitelist wl ON cu.reporter_abbr = wl.reporter_found
JOIN legalhist.reporters r ON r.reporter_standard = wl.reporter_standard
LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
WHERE wl.reporter_standard IS NOT NULL
  AND wl.junk = false
  AND COALESCE(r.type, '') <> 'statute'
GROUP BY wl.reporter_standard
WITH NO DATA;

CREATE UNIQUE INDEX IF NOT EXISTS linking_dashboard_reporters_uq
  ON moml_citations.linking_dashboard_reporters (reporter_standard);

-- migrate:down
SET ROLE = law_admin;

-- Restore the definition from 20260609133000_create_linking_dashboard_matviews.sql.
DROP MATERIALIZED VIEW IF EXISTS moml_citations.linking_dashboard_reporters;

CREATE MATERIALIZED VIEW IF NOT EXISTS moml_citations.linking_dashboard_reporters AS
SELECT
  wl.reporter_standard,
  count(*) FILTER (WHERE cl.status LIKE 'linked%') AS linked,
  count(*) FILTER (WHERE cl.status = 'no_match')   AS no_match,
  count(*) FILTER (WHERE cl.status IS NULL)         AS unprocessed,
  COALESCE(bool_or(r.jurisdiction LIKE 'uk:%'), false) AS uk
FROM moml_citations.citations_unlinked cu
JOIN legalhist.whitelist wl ON cu.reporter_abbr = wl.reporter_found
JOIN legalhist.reporters r ON r.reporter_standard = wl.reporter_standard
LEFT JOIN moml_citations.citation_links cl ON cl.citation_id = cu.id
WHERE wl.reporter_standard IS NOT NULL
  AND wl.junk = false
GROUP BY wl.reporter_standard
WITH NO DATA;

CREATE UNIQUE INDEX IF NOT EXISTS linking_dashboard_reporters_uq
  ON moml_citations.linking_dashboard_reporters (reporter_standard);
