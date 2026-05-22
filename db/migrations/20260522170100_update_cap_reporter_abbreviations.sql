-- migrate:up
SET ROLE = law_admin;
DROP MATERIALIZED VIEW IF EXISTS cap.reporter_abbreviations;

CREATE MATERIALIZED VIEW cap.reporter_abbreviations AS
WITH extracted AS (
  SELECT COALESCE(
    substring(c.cite from '^[0-9]+\s+(.+?)\s+[0-9]+[A-Za-z-]*$'),
    substring(c.cite from '^(.+?)\s+[0-9]+[A-Za-z-]*$'),
    substring(c.cite from '^[0-9]{4}-(.+?)-[0-9]+$')
  ) AS abbreviation
  FROM cap.citations c
  JOIN cap.cases cs ON cs.id = c."case"
  WHERE c.cite NOT LIKE '%;%'
    AND cs.decision_year <= 1925
)
SELECT abbreviation, COUNT(*)::bigint AS n
FROM extracted
WHERE abbreviation ~ '^[A-Za-z]'
GROUP BY abbreviation
HAVING COUNT(*) > 1
ORDER BY abbreviation;

CREATE UNIQUE INDEX reporter_abbreviations_abbreviation_idx
  ON cap.reporter_abbreviations (abbreviation);

-- migrate:down
SET ROLE = law_admin;
DROP MATERIALIZED VIEW IF EXISTS cap.reporter_abbreviations;

CREATE MATERIALIZED VIEW cap.reporter_abbreviations AS
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

CREATE UNIQUE INDEX reporter_abbreviations_abbreviation_idx
  ON cap.reporter_abbreviations (abbreviation);
