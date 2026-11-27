-- 1795643005: a_raw_original_names_the_identities_it_belongs_to.
SET LOCAL lock_timeout = '3s';

-- A raw original says whose message it is. Erasure and access requests used to find
-- a chat original by reading one network's payload, so a network the selector did
-- not know kept its originals after an erasure. A network now states the identities
-- of the message when it stores the original, as `provider:account`. Rows stored
-- before this column keep the payload match that already finds them.
ALTER TABLE raw_capture ADD COLUMN IF NOT EXISTS channel_subjects text[];
