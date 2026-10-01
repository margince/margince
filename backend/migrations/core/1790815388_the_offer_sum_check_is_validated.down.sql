-- Back to a recorded but unscanned constraint. The repaired rows stay repaired:
-- they were self-contradictory, and no reading of them is worth restoring.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer DROP CONSTRAINT offer_gross_is_net_plus_tax;
ALTER TABLE offer
    ADD CONSTRAINT offer_gross_is_net_plus_tax
    CHECK (gross_minor = net_minor + tax_minor) NOT VALID;
