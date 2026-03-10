-- migrate:up
CREATE SCHEMA IF NOT EXISTS english_reports;

CREATE TABLE IF NOT EXISTS
    english_reports.cases (
        "id" TEXT,
        "er_name" TEXT,
        "er_year" INT NOT NULL,
        "er_date" date NOT NULL,
        "er_cite" TEXT NOT NULL,
        "er_cite_disambiguated" TEXT NOT NULL,
        "er_parallel_cite" TEXT,
        "murrell_uid" TEXT,
        "murrell_year" INT,
        "murrell_title" TEXT,
        "er_filename" TEXT NOT NULL,
        "er_url" TEXT,
        "court" TEXT,
        "word_count" INT,
        PRIMARY KEY ("id")
    );

CREATE INDEX "er_cases_er_year_idx" ON "english_reports"."cases" ("er_year");

CREATE INDEX "er_cases_er_cite_idx" ON "english_reports"."cases" ("er_cite");

CREATE INDEX "er_cases_er_parallel_cite_idx" ON "english_reports"."cases" ("er_parallel_cite");

-- migrate:down
DROP TABLE IF EXISTS english_reports.cases;

DROP SCHEMA IF EXISTS english_reports;
