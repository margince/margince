-- The validation of the two update guards the previous file added. A file of
-- its own because VALIDATE takes a lighter lock than the ADD, and only a
-- separate transaction lets writers through while it scans.
--
-- Neither can fail. Each constraint replaces one that held the same rows
-- against the same catalog, and only its update action changed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal VALIDATE CONSTRAINT deal_acquisition_source_fkey;
ALTER TABLE lead VALIDATE CONSTRAINT lead_origin_fkey;
