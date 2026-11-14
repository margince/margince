SET LOCAL lock_timeout = '2s';
ALTER TABLE assurance_run DROP COLUMN eligible_deal_owners, DROP COLUMN eligible_signal_owners;
