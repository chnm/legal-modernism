-- migrate:up
CREATE TABLE IF NOT EXISTS
    legalhist.code_reporter (
        "id" BIGINT GENERATED ALWAYS AS IDENTITY,
        "name" TEXT NOT NULL,
        "name_abbreviation" TEXT NOT NULL,
        "decision_year" INT NOT NULL,
        "first_page" INT NOT NULL,
        "last_page" INT NOT NULL,
        "volume_number" INT NOT NULL,
        "official_citation" TEXT NOT NULL,
        "parallel_citation" TEXT,
        "reporter" TEXT,
        "court_name" TEXT,
        "court_cap_id" BIGINT,
        "jurisdiction" TEXT,
        "jurisdiction_slug" TEXT,
        "author" TEXT,
        "text" TEXT,
        PRIMARY KEY ("id")
    );

ALTER TABLE ONLY legalhist.code_reporter
ADD CONSTRAINT code_reporter_court_cap_id_fk FOREIGN KEY (court_cap_id) REFERENCES cap.courts (id);

CREATE INDEX "code_reporter_decision_year_idx" ON "legalhist"."code_reporter" ("decision_year");

CREATE INDEX "code_reporter_court_cap_id_idx" ON "legalhist"."code_reporter" ("court_cap_id");

CREATE INDEX "code_reporter_official_citation_idx" ON "legalhist"."code_reporter" ("official_citation");

CREATE INDEX "code_reporter_volume_number_idx" ON "legalhist"."code_reporter" ("volume_number");

-- migrate:down
DROP TABLE IF EXISTS legalhist.code_reporter;
