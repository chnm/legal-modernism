-- migrate:up
SET ROLE = law_admin;

CREATE OR REPLACE VIEW legalhist.abbrs_legalhist_not_in_cap AS
SELECT abbreviation
FROM legalhist.all_abbreviations
WHERE NOT uk
EXCEPT
SELECT abbreviation FROM cap.reporter_abbreviations
ORDER BY abbreviation;

CREATE OR REPLACE VIEW legalhist.abbrs_cap_not_in_legalhist AS
SELECT abbreviation, n
FROM cap.reporter_abbreviations
WHERE abbreviation NOT IN (
  SELECT abbreviation FROM legalhist.all_abbreviations WHERE NOT uk
)
ORDER BY abbreviation;

-- migrate:down
SET ROLE = law_admin;
DROP VIEW IF EXISTS legalhist.abbrs_cap_not_in_legalhist;
DROP VIEW IF EXISTS legalhist.abbrs_legalhist_not_in_cap;
