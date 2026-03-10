-- migrate:up

CREATE SCHEMA IF NOT EXISTS to_delete;

ALTER TABLE IF EXISTS legalhist.reporters_alt_samevols_abbreviations SET SCHEMA to_delete;
ALTER TABLE IF EXISTS legalhist.reporters_alt_diffvols_abbreviations SET SCHEMA to_delete;
ALTER VIEW IF EXISTS legalhist.reporters_single_volume_abbr SET SCHEMA to_delete;

-- migrate:down

ALTER VIEW IF EXISTS to_delete.reporters_single_volume_abbr SET SCHEMA legalhist;
ALTER TABLE IF EXISTS to_delete.reporters_alt_diffvols_abbreviations SET SCHEMA legalhist;
ALTER TABLE IF EXISTS to_delete.reporters_alt_samevols_abbreviations SET SCHEMA legalhist;

DROP SCHEMA IF EXISTS to_delete;

