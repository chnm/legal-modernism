-- migrate:up
-- PostgreSQL does not allow nullable columns in a primary key, so we must
-- drop the PK before making volume nullable, then replace it with a unique
-- index that uses COALESCE to handle NULLs.
ALTER TABLE moml_citations.citations_unlinked
DROP CONSTRAINT IF EXISTS moml_citations_pkey;

ALTER TABLE moml_citations.citations_unlinked
ADD CONSTRAINT IF NOT EXISTS citations_unlinked_pkey PRIMARY KEY (id);

ALTER TABLE moml_citations.citations_unlinked
ALTER COLUMN volume
DROP NOT NULL;

-- Replace the former PK with a unique index. COALESCE maps NULL to -1
-- (an impossible volume) so the index treats all NULLs as equal for dedup.
CREATE UNIQUE INDEX IF NOT EXISTS citations_unlinked_uq ON moml_citations.citations_unlinked (moml_treatise, moml_page, COALESCE(volume, -1), reporter_abbr, page);

-- Convert existing volume=0 rows for single-volume reporters to NULL.
UPDATE moml_citations.citations_unlinked
SET
  volume = NULL
WHERE
  volume = 0
  AND reporter_abbr IN (
    SELECT
      alt_abbr
    FROM
      legalhist.reporters_single_volume_abbr
  );

-- migrate:down
-- Restore volume=0 for any NULL volumes.
UPDATE moml_citations.citations_unlinked
SET
  volume = 0
WHERE
  volume IS NULL;

DROP INDEX IF EXISTS moml_citations.citations_unlinked_uq;

ALTER TABLE moml_citations.citations_unlinked
ALTER COLUMN volume
SET NOT NULL;

ALTER TABLE moml_citations.citations_unlinked
DROP CONSTRAINT IF EXISTS citations_unlinked_pkey;

ALTER TABLE moml_citations.citations_unlinked
ADD CONSTRAINT IF NOT EXISTS moml_citations_pkey PRIMARY KEY (moml_treatise, moml_page, volume, reporter_abbr, page);
