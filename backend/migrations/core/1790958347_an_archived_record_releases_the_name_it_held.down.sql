-- Back to holding the name after the archive.
--
-- THIS CAN FAIL, and failing is the honest outcome: once a name has been reused, the
-- table holds two rows the unconditional shape maps to one. Which of them keeps the
-- name is a question about the records, so there is no repair to automate here. The
-- migration runs in one transaction, so a failure leaves the schema as it was.
SET LOCAL lock_timeout = '3s';

DROP INDEX pipeline_name_unique;
ALTER TABLE pipeline ADD CONSTRAINT pipeline_name_unique UNIQUE (name);

DROP INDEX team_name_unique;
ALTER TABLE team ADD CONSTRAINT team_name_unique UNIQUE (name);

DROP INDEX offer_template_name_unique;
ALTER TABLE offer_template ADD CONSTRAINT offer_template_name_unique UNIQUE (name);
