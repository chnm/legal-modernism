-- migrate:up
SET ROLE = law_admin;

CREATE SCHEMA IF NOT EXISTS freelaw;

-- Parallel-citation crosswalk from the CourtListener "citations" bulk export.
-- Rows sharing a cluster_id are parallel citations for the same case.
CREATE TABLE IF NOT EXISTS freelaw.citations (
    id            bigint PRIMARY KEY,
    volume        text NOT NULL,
    reporter      text NOT NULL,
    page          text NOT NULL,
    cite          text GENERATED ALWAYS AS (volume || ' ' || reporter || ' ' || page) STORED,
    type          text NOT NULL,
    cluster_id    bigint NOT NULL,
    date_created  timestamptz,
    date_modified timestamptz,
    CONSTRAINT chk_freelaw_citations_type CHECK (type IN (
        'federal', 'state', 'state_regional', 'specialty', 'scotus_early',
        'lexis', 'west', 'neutral', 'journal'
    ))
);

CREATE INDEX IF NOT EXISTS idx_freelaw_citations_cluster_id
    ON freelaw.citations (cluster_id);
CREATE INDEX IF NOT EXISTS idx_freelaw_citations_cite
    ON freelaw.citations (cite);

-- Crosswalk from a CourtListener opinion cluster to a CAP case. cap_case_id is
-- nullable: a row exists for every cluster with a Harvard filepath, but the id
-- is NULL when the extracted CAP case is not present in cap.cases.
CREATE TABLE IF NOT EXISTS freelaw.clusters_to_cap (
    cluster_id  bigint PRIMARY KEY,
    cap_case_id bigint REFERENCES cap.cases(id)
);

CREATE INDEX IF NOT EXISTS idx_freelaw_clusters_to_cap_cap_case_id
    ON freelaw.clusters_to_cap (cap_case_id) WHERE cap_case_id IS NOT NULL;

-- migrate:down
SET ROLE = law_admin;

DROP TABLE IF EXISTS freelaw.citations;
DROP TABLE IF EXISTS freelaw.clusters_to_cap;
DROP SCHEMA IF EXISTS freelaw;
