-- migrate:up
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

-- migrate:down
DROP VIEW IF EXISTS legalhist.all_abbreviations;
