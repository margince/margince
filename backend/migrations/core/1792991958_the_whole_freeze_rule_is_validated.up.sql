-- The validation of the freeze rule the previous file tightened, and of the
-- foreign key its down file restores. A file of its own because VALIDATE takes
-- a lighter lock than the ADD, and only a separate transaction lets writers
-- through while it scans.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal VALIDATE CONSTRAINT deal_closed_fx;
