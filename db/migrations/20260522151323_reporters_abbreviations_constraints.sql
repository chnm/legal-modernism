-- migrate:up
SET ROLE = law_admin;

ALTER TABLE legalhist.reporters_abbreviations
    ALTER COLUMN alt_abbr SET NOT NULL;

ALTER TABLE legalhist.reporters_abbreviations
    DROP CONSTRAINT IF EXISTS reporters_abbreviations_distinct_check;

ALTER TABLE legalhist.reporters_abbreviations
    ADD CONSTRAINT reporters_abbreviations_distinct_check
    CHECK (reporter_standard <> alt_abbr);

-- migrate:down
ALTER TABLE legalhist.reporters_abbreviations
    DROP CONSTRAINT IF EXISTS reporters_abbreviations_distinct_check;

ALTER TABLE legalhist.reporters_abbreviations
    ALTER COLUMN alt_abbr DROP NOT NULL;
