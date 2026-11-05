-- The validation of the key the lead company migration added. A file of its
-- own because VALIDATE takes a lighter lock than the ADD, and only a separate
-- transaction lets writers through while it scans. It cannot fail: every row
-- written before the column has a NULL key, which the foreign key admits.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead VALIDATE CONSTRAINT lead_company_id_fkey;
