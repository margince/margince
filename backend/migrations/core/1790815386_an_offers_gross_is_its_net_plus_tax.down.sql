-- Let the three money columns disagree again.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer DROP CONSTRAINT offer_gross_is_net_plus_tax;
