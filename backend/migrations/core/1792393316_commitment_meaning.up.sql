SET LOCAL lock_timeout = '5s';
ALTER TABLE activity_commitment_admission ADD COLUMN meaning jsonb;
