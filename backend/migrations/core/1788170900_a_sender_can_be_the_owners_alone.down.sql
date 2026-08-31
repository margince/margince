SET LOCAL lock_timeout = '3s';

-- Rows already carrying a kind this constraint refuses would block the ADD, and
-- a down migration that cannot run is not a rollback. Each falls back to the
-- old kind that preserves its EFFECT, which is not the same as preserving its
-- meaning — the rollback is lossy and these are the least-lossy targets.
--
-- `advisor` becomes `role_mailbox`, not `person`. Both are status `real`, but
-- the sink reads `real AND kind <> 'person'` as "decided, mint nothing"; the
-- rewrite to `person` would flip every advisor row to "known counterparty", so
-- the next message from a founder's lawyer would mint and publish the contact
-- this kind exists to withhold. What is lost is the reason: the row will say
-- there was no human to name, when in fact there was one who was the owner's.
--
-- `personal` becomes `spam`, which keeps it withheld and is the honest floor
-- available: every noise kind carries some claim about the sender that a family
-- member does not deserve, and there is no neutral one to reduce to.
UPDATE capture_pending_counterparty
   SET kind = 'role_mailbox' WHERE kind = 'advisor';
UPDATE capture_pending_counterparty
   SET kind = 'spam' WHERE kind = 'personal';

ALTER TABLE capture_pending_counterparty
    DROP CONSTRAINT IF EXISTS capture_pending_counterparty_kind_check;

ALTER TABLE capture_pending_counterparty
    ADD CONSTRAINT capture_pending_counterparty_kind_check
    CHECK (kind IS NULL OR kind IN (
        'person', 'role_mailbox', 'organization_sender',
        'newsletter', 'transactional', 'spam'));
