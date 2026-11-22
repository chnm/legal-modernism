-- migrate:up
SET ROLE = law_admin;

-- Volumes taken from HathiTrust and kept in the moml tables beside Gale's
-- (issue #402). The first are the HathiTrust printings of Stephen on Pleading
-- (work 1107), which scripts/hathitrust-import loads. Such a volume is an
-- ordinary row of moml.volumes with its own edition; this table says where it
-- came from and which MOML volume, if any, is the same book, so a reader that
-- must count an edition once can leave one of the pair out.

CREATE TABLE IF NOT EXISTS moml.hathitrust_volumes (
    psmid        text PRIMARY KEY REFERENCES moml.volumes (psmid),
    htid         text NOT NULL UNIQUE,
    ht_record    text NOT NULL,
    oclc         text,
    rights       text NOT NULL,
    digitized_by text,
    moml_psmid   text REFERENCES moml.volumes (psmid),
    relation     text CHECK (relation IN ('same_printing', 'same_edition')),
    note         text,
    imported_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (psmid <> moml_psmid),
    CHECK ((moml_psmid IS NULL) = (relation IS NULL))
);

CREATE INDEX IF NOT EXISTS hathitrust_volumes_moml_psmid_idx
    ON moml.hathitrust_volumes (moml_psmid);

COMMENT ON TABLE moml.hathitrust_volumes IS
  'Provenance of the volumes in moml.volumes that were taken from HathiTrust rather than Gale (issue #402), and the MOML volume each one duplicates, if any';
COMMENT ON COLUMN moml.hathitrust_volumes.htid IS
  'HathiTrust item id; the item is at https://hdl.handle.net/2027/<htid>, and page sequence n is the volume''s pageid n * 10';
COMMENT ON COLUMN moml.hathitrust_volumes.ht_record IS
  'HathiTrust catalog record number, https://catalog.hathitrust.org/Record/<ht_record>';
COMMENT ON COLUMN moml.hathitrust_volumes.rights IS
  'HathiTrust rights code when the volume was imported (pd: public domain)';
COMMENT ON COLUMN moml.hathitrust_volumes.digitized_by IS
  'Who scanned the item (google, ia, cornell-ms), which governs how the scans may be reused';
COMMENT ON COLUMN moml.hathitrust_volumes.moml_psmid IS
  'The Gale volume of the same book, NULL when MOML has none';
COMMENT ON COLUMN moml.hathitrust_volumes.relation IS
  'same_printing: the same edition, imprint and year as moml_psmid; same_edition: the same edition in another printing';

GRANT SELECT ON moml.hathitrust_volumes TO law_service;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON moml.hathitrust_volumes TO law_dev;

-- migrate:down
SET ROLE = law_admin;

DROP TABLE IF EXISTS moml.hathitrust_volumes;
