-- Validate the two basis checks the migration before this one added NOT VALID.
-- Its own file, so the scan runs under the lighter lock and not under the
-- ACCESS EXCLUSIVE that the ALTER held to its commit. The rows were moved onto
-- the new list there, so both scans pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_basis VALIDATE CONSTRAINT communication_basis_kind;
ALTER TABLE communication_decision VALIDATE CONSTRAINT communication_decision_basis;
