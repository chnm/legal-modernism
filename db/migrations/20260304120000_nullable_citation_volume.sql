-- migrate:up

-- Allow volume to be NULL for single-volume reporters (issue #134).
ALTER TABLE moml_citations.citations_unlinked ALTER COLUMN volume DROP NOT NULL;

-- The existing PK includes volume, so NULL volumes are always distinct.
-- Add a partial unique index to prevent duplicate single-vol citations.
CREATE UNIQUE INDEX citations_unlinked_nullvol_uq
    ON moml_citations.citations_unlinked (moml_treatise, moml_page, reporter_abbr, page)
    WHERE volume IS NULL;

-- Convert existing volume=0 rows for single-volume reporters to NULL.
UPDATE moml_citations.citations_unlinked
SET volume = NULL
WHERE volume = 0
  AND reporter_abbr IN (SELECT alt_abbr FROM legalhist.reporters_single_volume_abbr);

-- migrate:down

-- Restore volume=0 for any NULL volumes.
UPDATE moml_citations.citations_unlinked
SET volume = 0
WHERE volume IS NULL;

DROP INDEX IF EXISTS moml_citations.citations_unlinked_nullvol_uq;

ALTER TABLE moml_citations.citations_unlinked ALTER COLUMN volume SET NOT NULL;
