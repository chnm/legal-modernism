-- migrate:up
ALTER TABLE legalhist.reporters_alt_diffvols_reporters RENAME TO reporters_nominate;

-- migrate:down
ALTER TABLE legalhist.reporters_nominate RENAME TO reporters_alt_diffvols_reporters;

