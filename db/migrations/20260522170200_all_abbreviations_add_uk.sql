-- migrate:up
SET ROLE = law_admin;
CREATE OR REPLACE VIEW legalhist.all_abbreviations AS
SELECT abbreviation, jurisdiction, jurisdiction LIKE 'uk%' AS uk
FROM (
  SELECT r.reporter_standard AS abbreviation, r.jurisdiction
  FROM legalhist.reporters r
  UNION
  SELECT ra.alt_abbr AS abbreviation, r.jurisdiction
  FROM legalhist.reporters_abbreviations ra
  JOIN legalhist.reporters r ON r.reporter_standard = ra.reporter_standard
  WHERE ra.alt_abbr IS NOT NULL
) u
ORDER BY abbreviation;

-- migrate:down
SET ROLE = law_admin;
CREATE OR REPLACE VIEW legalhist.all_abbreviations AS
SELECT abbreviation FROM (
  SELECT DISTINCT reporter_standard AS abbreviation FROM legalhist.reporters
  UNION
  SELECT DISTINCT alt_abbr AS abbreviation
    FROM legalhist.reporters_abbreviations
    WHERE alt_abbr IS NOT NULL
) u
ORDER BY abbreviation;
