-- migrate:up

CREATE SCHEMA IF NOT EXISTS to_delete;

ALTER TABLE IF EXISTS legalhist.reporters_diffvols
    DROP CONSTRAINT IF EXISTS reporters_diffvols_reporter_title_fkey;

ALTER TABLE IF EXISTS legalhist.reporters_nominate SET SCHEMA to_delete;

-- migrate:down

ALTER TABLE IF EXISTS to_delete.reporters_nominate SET SCHEMA legalhist;

ALTER TABLE IF EXISTS legalhist.reporters_diffvols
    ADD CONSTRAINT reporters_diffvols_reporter_title_fkey
    FOREIGN KEY (reporter_title)
    REFERENCES legalhist.reporters_nominate (reporter_title)
    ON UPDATE CASCADE;
