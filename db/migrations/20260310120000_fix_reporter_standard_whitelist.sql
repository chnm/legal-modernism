-- migrate:up
-- Fix incorrect reporter_standard values for the English Reports 
-- See issue #111
SET ROLE = law_admin;

BEGIN;

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'E.R.'
WHERE
	reporter_standard = 'Eng. Rep.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Colles'
WHERE
	reporter_standard = 'Coll';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Bro PC'
WHERE
	reporter_standard = 'Bro. P. C.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Bligh PC'
WHERE
	reporter_standard = 'Bligh';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Bligh NS PC'
WHERE
	reporter_standard = 'Bligh NS';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Chan Cas'
WHERE
	reporter_standard = 'Ch.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Chan Cas'
WHERE
	reporter_standard = 'Ch. Ca.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Bro CC'
WHERE
	reporter_standard = 'Bro. C.C.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Bro CC'
WHERE
	reporter_standard = 'Bro. Ch.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Bro PC'
WHERE
	reporter_standard = 'Bro. P.C.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Bro PC'
WHERE
	reporter_standard = 'Bro. Par.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Mer'
WHERE
	reporter_standard = 'Meriv.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Jac & W'
WHERE
	reporter_standard = 'J. & W.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'East'
WHERE
	reporter_standard = 'East P. C.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'East'
WHERE
	reporter_standard = 'East, P. C.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'West',
	uk = TRUE
WHERE
	reporter_standard = 'West. Rep.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Cox',
	reporter_cap = NULL
WHERE
	reporter_standard = 'Cox.';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Chan Cas'
WHERE
	reporter_standard = 'Choyce Cases';

UPDATE legalhist.reporters_citation_to_cap
SET
	reporter_standard = 'Barn C'
WHERE
	reporter_standard = 'Bar N';

ROLLBACK;

-- migrate:down
