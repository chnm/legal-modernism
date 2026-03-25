-- migrate:up
ALTER TABLE legalhist.whitelist
    ADD CONSTRAINT fk_whitelist_reporter_standard
    FOREIGN KEY (reporter_standard) REFERENCES legalhist.reporters(reporter_standard);

-- migrate:down
ALTER TABLE legalhist.whitelist
    DROP CONSTRAINT IF EXISTS fk_whitelist_reporter_standard;
