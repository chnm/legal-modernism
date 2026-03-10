-- migrate:up
-- Rename the table
ALTER TABLE legalhist.reporters_alt_diffvols_volumes
RENAME TO reporters_diffvols;

-- Fix trailing non-breaking space in Harper's South Carolina Law Reports
-- Replace FK with ON UPDATE CASCADE, update parent (child follows automatically), then restore plain FK
ALTER TABLE legalhist.reporters_diffvols
DROP CONSTRAINT IF EXISTS reporters_alt_diffvols_volumes_reporter_title_fkey;

ALTER TABLE legalhist.reporters_diffvols
ADD CONSTRAINT reporters_diffvols_reporter_title_fkey FOREIGN KEY (reporter_title) REFERENCES legalhist.reporters_nominate (reporter_title) ON UPDATE CASCADE;

-- Drop FK on to_delete table that also references reporter_title
ALTER TABLE to_delete.reporters_alt_diffvols_abbreviations
DROP CONSTRAINT IF EXISTS reporters_alt_diffvols_abbreviations_reporter_title_fkey;

UPDATE legalhist.reporters_nominate
SET
	reporter_title = 'Harper''s South Carolina Law Reports'
WHERE
	reporter_title = E'Harper''s South Carolina Law Reports\u00a0';


-- Add reporter_standard to reporters_nominate (one value per reporter)
ALTER TABLE legalhist.reporters_nominate
ADD COLUMN IF NOT EXISTS reporter_standard TEXT;

-- Populate reporter_standard
UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Allen'
WHERE
	reporter_title = 'Allen''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Appl.'
WHERE
	reporter_title = 'Appleton''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Bail. Eq.'
WHERE
	reporter_title = 'Bailey''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Bail.'
WHERE
	reporter_title = 'Bailey''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Baxter'
WHERE
	reporter_title = 'Baxter''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Bay'
WHERE
	reporter_title = 'Bay''s South Carolina Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Beas.'
WHERE
	reporter_title = 'Beasley''s Chencery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Black'
WHERE
	reporter_title = 'Black''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Boyce'
WHERE
	reporter_title = 'Boyce''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Brev.'
WHERE
	reporter_title = 'Brevard''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Buch.'
WHERE
	reporter_title = 'Buchanan''s Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Busb. Eq.'
WHERE
	reporter_title = 'Busbee''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Busb.'
WHERE
	reporter_title = 'Busbee''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Green Eq.'
WHERE
	reporter_title = 'C. E. Green''s Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Cam. & Nor.'
WHERE
	reporter_title = 'Cameron & Norwood''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Car. L. Rep.'
WHERE
	reporter_title = 'Carolina Law Repository';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Cates'
WHERE
	reporter_title = 'Cates'' Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Chev. Eq.'
WHERE
	reporter_title = 'Cheves'' Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Chev.'
WHERE
	reporter_title = 'Cheves'' South Carolina Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Comst.'
WHERE
	reporter_title = 'Comstock''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Mill'
WHERE
	reporter_title = 'Constitutional Reports (Mill)';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Coxe'
WHERE
	reporter_title = 'Coxe''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Cranch'
WHERE
	reporter_title = 'Cranch''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Cush.'
WHERE
	reporter_title = 'Cushing''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Des. Eq.'
WHERE
	reporter_title = 'Desaussure''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Dev. & Bat. Eq.'
WHERE
	reporter_title = 'Devereux & Battle''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Dev. & Bat. Eq.'
WHERE
	reporter_title = 'Devereux & Battle''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Dev. Eq.'
WHERE
	reporter_title = 'Devereux''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Dev.'
WHERE
	reporter_title = 'Devereux''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Dick. Eq.'
WHERE
	reporter_title = 'Dickinson''s Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Dud. Eq.'
WHERE
	reporter_title = 'Dudley''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Dud.'
WHERE
	reporter_title = 'Dudley''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Dutch.'
WHERE
	reporter_title = 'Dutcher''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Fairf.'
WHERE
	reporter_title = 'Fairfield''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Gray'
WHERE
	reporter_title = 'Gray''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Green Eq.'
WHERE
	reporter_title = 'Green''s Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Green'
WHERE
	reporter_title = 'Greene''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Greenl.'
WHERE
	reporter_title = 'Greenleaf''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Hagan'
WHERE
	reporter_title = 'Hagans'' Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Halst. Eq.'
WHERE
	reporter_title = 'Halsted''s Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Halst.'
WHERE
	reporter_title = 'Halsted''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Harp.'
WHERE
	reporter_title = 'Harper''s South Carolina Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Harrison'
WHERE
	reporter_title = 'Harrison''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Hawks'
WHERE
	reporter_title = 'Hawks'' Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Hayw.'
WHERE
	reporter_title = 'Haywood''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Hill Eq.'
WHERE
	reporter_title = 'Hill''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Hill'
WHERE
	reporter_title = 'Hill''s South Carolina Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'How.'
WHERE
	reporter_title = 'Howard''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Ired. Eq.'
WHERE
	reporter_title = 'Iredell''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Ired.'
WHERE
	reporter_title = 'Iredell''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Jones Eq.'
WHERE
	reporter_title = 'Jones'' Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Jones'
WHERE
	reporter_title = 'Jones'' Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Kelly'
WHERE
	reporter_title = 'Kelly''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Kern.'
WHERE
	reporter_title = 'Kernan''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Lea'
WHERE
	reporter_title = 'Lea''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Martin'
WHERE
	reporter_title = 'Martin''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'McCart.'
WHERE
	reporter_title = 'McCarter''s Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'McCord Eq.'
WHERE
	reporter_title = 'McCord''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'McCord'
WHERE
	reporter_title = 'McCord''s South Carolina Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'McMul. Eq.'
WHERE
	reporter_title = 'McMullan''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'McMul.'
WHERE
	reporter_title = 'McMullan''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Met.'
WHERE
	reporter_title = 'Metcalf''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Mur.'
WHERE
	reporter_title = 'Murphey''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'McCord'
WHERE
	reporter_title = 'Nott & McCord''s South Carolina Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Otto'
WHERE
	reporter_title = 'Otto''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Penne.'
WHERE
	reporter_title = 'Pennewill''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Pennington'
WHERE
	reporter_title = 'Pennington''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Pet.'
WHERE
	reporter_title = 'Peters'' Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Phil.'
WHERE
	reporter_title = 'Phillips'' Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Phil.'
WHERE
	reporter_title = 'Phillips'' Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Pick.'
WHERE
	reporter_title = 'Pickering''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Pickle'
WHERE
	reporter_title = 'Pickle''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Rice'
WHERE
	reporter_title = 'Rice''s South Carolina Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Rich. Eq.'
WHERE
	reporter_title = 'Richardson''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Rich.'
WHERE
	reporter_title = 'Richardson''s Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Ril. Eq.'
WHERE
	reporter_title = 'Riley''s Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Ril.'
WHERE
	reporter_title = 'Riley''s South Carolina Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Robb.'
WHERE
	reporter_title = 'Robbins'' Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Saxton'
WHERE
	reporter_title = 'Saxton''s Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Seld.'
WHERE
	reporter_title = 'Selden''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Shep.'
WHERE
	reporter_title = 'Shepley''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'South.'
WHERE
	reporter_title = 'Southard''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Speers Eq.'
WHERE
	reporter_title = 'Speers Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Speers'
WHERE
	reporter_title = 'Speers'' South Carolina Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Spencer'
WHERE
	reporter_title = 'Spencer''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Stew.'
WHERE
	reporter_title = 'Stewart''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Stock.'
WHERE
	reporter_title = 'Stockton''s Chancery Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Storey'
WHERE
	reporter_title = 'Storey''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Strob. Eq.'
WHERE
	reporter_title = 'Strobhart''s Equity Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Strob.'
WHERE
	reporter_title = 'Strobhart''s South Carolina Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Tay.'
WHERE
	reporter_title = 'Taylor''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Taylor'
WHERE
	reporter_title = 'Term Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Terry'
WHERE
	reporter_title = 'Terry''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Tread'
WHERE
	reporter_title = 'Treadway''s South Carolina Law Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Tyng'
WHERE
	reporter_title = 'Tyng''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Vroom'
WHERE
	reporter_title = 'Vroom G.D.W.''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Vroom (o.s.)'
WHERE
	reporter_title = 'Vroom''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'W.W. Harr.'
WHERE
	reporter_title = 'W. W. Harrington''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Wall.'
WHERE
	reporter_title = 'Wallace''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Wheat.'
WHERE
	reporter_title = 'Wheaton''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Win.'
WHERE
	reporter_title = 'Winston''s Reports';

UPDATE legalhist.reporters_nominate
SET
	reporter_standard = 'Zab.'
WHERE
	reporter_title = 'Zabriskie''s Reports';

-- migrate:down
ALTER TABLE legalhist.reporters_nominate
DROP COLUMN IF EXISTS reporter_standard;

-- Restore the old FK constraint name
ALTER TABLE legalhist.reporters_diffvols
DROP CONSTRAINT IF EXISTS reporters_diffvols_reporter_title_fkey;

ALTER TABLE legalhist.reporters_diffvols
ADD CONSTRAINT reporters_alt_diffvols_volumes_reporter_title_fkey FOREIGN KEY (reporter_title) REFERENCES legalhist.reporters_nominate (reporter_title) ON UPDATE CASCADE;

ALTER TABLE legalhist.reporters_diffvols
RENAME TO reporters_alt_diffvols_volumes;

-- Not restoring FK on to_delete.reporters_alt_diffvols_abbreviations
-- because the Harper's title fix is not reverted, so rows would violate the constraint.
