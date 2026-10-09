-- certified_staff carried no CHECK, and the handler cast a JSON float straight
-- to smallint: 1e30 wrapped to -1, and a negative seat count was stored as
-- sent.
--
-- Its sibling retention_rate already carries its domain
-- (partner_retention_rate_check, 0..100), as do partner_fit_score and
-- relationship_health. This column was the gap.
--
-- Existing rows are repaired first. A wrapped value is not data anybody
-- entered, and a constraint that cannot validate is one that only binds new
-- rows.
SET LOCAL lock_timeout = '3s';

UPDATE partner SET certified_staff = 0 WHERE certified_staff < 0;

ALTER TABLE partner
	ADD CONSTRAINT partner_certified_staff_check
	CHECK (certified_staff >= 0) NOT VALID;
