-- migrate:up
SET ROLE = law_admin;

ALTER TABLE legalhist.reporters_diffvols
    ALTER COLUMN reporter_standard SET NOT NULL;

ALTER TABLE legalhist.reporters_diffvols
    DROP CONSTRAINT IF EXISTS fk_reporters_diffvols_reporter_standard;

ALTER TABLE legalhist.reporters_diffvols
    ADD CONSTRAINT fk_reporters_diffvols_reporter_standard
    FOREIGN KEY (reporter_standard) REFERENCES legalhist.reporters(reporter_standard);

-- migrate:down
ALTER TABLE legalhist.reporters_diffvols
    DROP CONSTRAINT IF EXISTS fk_reporters_diffvols_reporter_standard;

ALTER TABLE legalhist.reporters_diffvols
    ALTER COLUMN reporter_standard DROP NOT NULL;
