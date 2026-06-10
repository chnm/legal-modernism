-- migrate:up
SET ROLE = law_admin;

ALTER TABLE legalhist.whitelist
    ADD CONSTRAINT chk_whitelist_junk_no_standard
    CHECK (NOT (junk AND reporter_standard IS NOT NULL));

ALTER TABLE legalhist.whitelist
    ADD CONSTRAINT chk_whitelist_nonjunk_has_standard
    CHECK (junk OR reporter_standard IS NOT NULL);

-- migrate:down
ALTER TABLE legalhist.whitelist
    DROP CONSTRAINT IF EXISTS chk_whitelist_nonjunk_has_standard;

ALTER TABLE legalhist.whitelist
    DROP CONSTRAINT IF EXISTS chk_whitelist_junk_no_standard;
