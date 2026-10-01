-- Two uuid primary keys defaulted to gen_random_uuid(). uuidv7() is the house
-- rule and ids.NewV7 states the reason: a v7 carries its millisecond in the
-- leading bits, so inserts land at the index's right edge instead of at a random
-- point in it, and a v4 key pays the whole index as its working set.
--
-- APPEND-MOSTLY, not strictly ordered. ids.NewV7 keeps a monotonic counter
-- within a millisecond; the database function does not, so two defaults in the
-- same millisecond may sort either way. That buys the locality this is for and
-- nothing stronger — nothing here reads an id as a sequence.
--
-- The default is what fires here — neither table's insert names `id` — so the
-- rows already written keep their v4 keys and every new one is a v7. Mixed is
-- fine: the ordering claim is about where the NEXT insert lands, not about the
-- index being wholly ordered.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE federated_identity ALTER COLUMN id SET DEFAULT uuidv7();
