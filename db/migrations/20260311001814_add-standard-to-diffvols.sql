-- migrate:up
ALTER TABLE legalhist.reporters_diffvols
ADD COLUMN IF NOT EXISTS reporter_standard TEXT;

UPDATE legalhist.reporters_diffvols dv
SET reporter_standard = rn.reporter_standard
FROM legalhist.reporters_nominate rn
WHERE dv.reporter_title = rn.reporter_title;

-- migrate:down
ALTER TABLE legalhist.reporters_diffvols
DROP COLUMN IF EXISTS reporter_standard;
