-- migrate:up
CREATE TABLE IF NOT EXISTS legalhist.reporters (
    reporter_standard text PRIMARY KEY,
    reporter_title    text,
    level             text,
    jurisdiction      text,
    year_start        integer,
    year_end          integer,
    single_vol        boolean,
    type              text,
    reporter_cap      text
);

CREATE INDEX IF NOT EXISTS idx_reporters_jurisdiction
    ON legalhist.reporters USING btree (jurisdiction text_pattern_ops);

DROP VIEW IF EXISTS legalhist.top_reporters_not_whitelisted;

ALTER TABLE legalhist.reporters_citation_to_cap
    DROP COLUMN IF EXISTS reporter_cap,
    DROP COLUMN IF EXISTS statute,
    DROP COLUMN IF EXISTS uk,
    DROP COLUMN IF EXISTS cap_different;

ALTER TABLE legalhist.reporters_citation_to_cap
    RENAME TO whitelist;

-- migrate:down
ALTER TABLE legalhist.whitelist
    RENAME TO reporters_citation_to_cap;

ALTER TABLE legalhist.reporters_citation_to_cap
    ADD COLUMN IF NOT EXISTS reporter_cap text,
    ADD COLUMN IF NOT EXISTS statute boolean,
    ADD COLUMN IF NOT EXISTS uk boolean,
    ADD COLUMN IF NOT EXISTS cap_different boolean;

DROP INDEX IF EXISTS legalhist.idx_reporters_jurisdiction;

DROP TABLE IF EXISTS legalhist.reporters;
