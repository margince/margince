-- Put both defaults back as the catalog held them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE federated_identity ALTER COLUMN id SET DEFAULT gen_random_uuid();
