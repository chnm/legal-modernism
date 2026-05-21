-- migrate:up
SET ROLE = law_admin;
CREATE MATERIALIZED VIEW IF NOT EXISTS cap.reporter_abbreviations AS
WITH extracted AS (
  SELECT COALESCE(
    substring(cite from '^[0-9]+\s+(.+?)\s+[0-9]+[A-Za-z-]*$'),
    substring(cite from '^(.+?)\s+[0-9]+[A-Za-z-]*$'),
    substring(cite from '^[0-9]{4}-(.+?)-[0-9]+$')
  ) AS abbreviation
  FROM cap.citations
  WHERE type != 'parallel'
)
SELECT abbreviation, COUNT(*)::bigint AS n
FROM extracted
WHERE abbreviation ~ '^[A-Za-z]'
GROUP BY abbreviation
HAVING COUNT(*) > 1
ORDER BY abbreviation;

CREATE UNIQUE INDEX IF NOT EXISTS reporter_abbreviations_abbreviation_idx
  ON cap.reporter_abbreviations (abbreviation);

-- migrate:down
DROP MATERIALIZED VIEW IF EXISTS cap.reporter_abbreviations;
