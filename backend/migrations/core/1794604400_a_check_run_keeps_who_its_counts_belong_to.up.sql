SET LOCAL lock_timeout = '2s';
ALTER TABLE assurance_run ADD COLUMN eligible_deal_owners uuid[], ADD COLUMN eligible_signal_owners uuid[];
