-- A unique index over the raw value sees two spellings of one address as two
-- rows. Each check makes the stored form the lowercased, trimmed one. It is
-- added unvalidated, so the table is not scanned under the lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user ADD CONSTRAINT app_user_email_is_canonical
    CHECK (email = lower(btrim(email))) NOT VALID;
ALTER TABLE capture_pending_counterparty ADD CONSTRAINT capture_pending_counterparty_email_is_canonical
    CHECK (email = lower(btrim(email))) NOT VALID;
ALTER TABLE capture_sender_override ADD CONSTRAINT capture_sender_override_address_is_canonical
    CHECK (address = lower(btrim(address))) NOT VALID;
ALTER TABLE communication_suppression ADD CONSTRAINT communication_suppression_address_is_canonical
    CHECK (address = lower(btrim(address))) NOT VALID;

-- Every writer folds the value today. A row an earlier release stored as typed
-- is folded here, unless another row already holds the folded form.
UPDATE app_user u SET email = lower(btrim(u.email))
 WHERE u.email <> lower(btrim(u.email))
   AND NOT EXISTS (SELECT 1 FROM app_user twin
                    WHERE twin.id <> u.id AND lower(twin.email) = lower(btrim(u.email)));
UPDATE capture_pending_counterparty p SET email = lower(btrim(p.email))
 WHERE p.email <> lower(btrim(p.email))
   AND NOT EXISTS (SELECT 1 FROM capture_pending_counterparty twin
                    WHERE twin.id <> p.id AND twin.email = lower(btrim(p.email)));
UPDATE capture_sender_override o SET address = lower(btrim(o.address))
 WHERE o.address <> lower(btrim(o.address))
   AND NOT EXISTS (SELECT 1 FROM capture_sender_override twin
                    WHERE twin.user_id = o.user_id AND twin.address = lower(btrim(o.address)));
UPDATE communication_suppression s SET address = lower(btrim(s.address))
 WHERE s.address <> lower(btrim(s.address))
   AND NOT EXISTS (SELECT 1 FROM communication_suppression twin
                    WHERE twin.id <> s.id AND lower(twin.address) = lower(btrim(s.address)));
