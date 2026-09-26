-- Create the key CAP metadata tables
BEGIN;
CREATE TABLE IF NOT EXISTS cap.cases (
  id bigint PRIMARY KEY,
  name_abbreviation text NOT NULL,
  name text NOT NULL,
  decision_year int,
  decision_date date,
  first_page int,
  last_page int,
  volume text NOT NULL,
  reporter bigint NOT NULL,
  court bigint NOT NULL,
  jurisdiction bigint NOT NULL,
  url text NOT NULL,
  frontend_url text NOT NULL,
  frontend_pdf_url text NOT NULL,
  decision_date_raw text NOT NULL,
  docket_number text,
  first_page_raw text NOT NULL,
  last_page_raw text NOT NULL,
  analysis jsonb NOT NULL,
  provenance jsonb NOT NULL,
  judges jsonb NOT NULL,
  parties jsonb NOT NULL,
  attorneys jsonb NOT NULL,
  last_updated timestamp with time zone NOT NULL,
  imported timestamp with time zone NOT NULL
);
CREATE TABLE IF NOT EXISTS cap.citations (
  cite text NOT NULL,
  type text NOT NULL,
  "case" bigint NOT NULL
);
CREATE TABLE IF NOT EXISTS cap.courts (
  id bigint PRIMARY KEY,
  name text NOT NULL,
  name_abbreviation text NOT NULL,
  slug text NOT NULL,
  url text NOT NULL
);
CREATE TABLE IF NOT EXISTS cap.jurisdictions (
  id bigint PRIMARY KEY,
  name_long text NOT NULL,
  name text NOT NULL,
  slug text NOT NULL,
  whitelisted bool NOT NULL,
  url text NOT NULL
);
-- The table as the import created it. It has since gained an id (bigint,
-- identity, primary key) and an index on "case", added by
-- db/migrations/20260926130000_cap-opinion-ids.sql (issue #74), which
-- backfills the id instead of rewriting the 38 GB table; the migration, not
-- this script, is where the live table's shape is recorded.
CREATE TABLE IF NOT EXISTS cap.opinions (
  "case" bigint NOT NULL,
  "type" text NOT NULL,
  author jsonb NOT NULL,
  "text" text NOT NULL
);

ALTER TABLE cap.cases
  ADD CONSTRAINT cap_cases_reporter_fk FOREIGN KEY (reporter) REFERENCES cap.reporters (id);
ALTER TABLE cap.cases
  ADD CONSTRAINT cap_cases_court_fk FOREIGN KEY (court) REFERENCES cap.courts (id);
ALTER TABLE cap.cases
  ADD CONSTRAINT cap_cases_jurisdiction_fk FOREIGN KEY (jurisdiction) REFERENCES cap.jurisdictions (id);
ALTER TABLE cap.cases
  ADD CONSTRAINT cap_cases_volume_fk FOREIGN KEY (volume) REFERENCES cap.volumes (barcode);
ALTER TABLE cap.citations
  ADD CONSTRAINT cap_citations_case_fk FOREIGN KEY ("case") REFERENCES cap.cases (id);
ALTER TABLE cap.opinions
  ADD CONSTRAINT cap_opinions_case_fk FOREIGN KEY ("case") REFERENCES cap.cases (id);
CREATE INDEX cap_case_year_idx ON cap.cases (decision_year);
CREATE INDEX cap_case_first_page_idx ON cap.cases (first_page);
CREATE INDEX cap_case_last_page_idx ON cap.cases (last_page);
CREATE INDEX cap_case_volume_idx ON cap.cases (volume);
CREATE INDEX cap_case_reporter_idx ON cap.cases (reporter);
CREATE INDEX cap_case_jurisdiction_idx ON cap.cases (jurisdiction);
CREATE INDEX cap_citations_cite_idx ON cap.citations (cite);
CREATE INDEX cap_citations_case_idx ON cap.citations ("case");
COMMIT;

