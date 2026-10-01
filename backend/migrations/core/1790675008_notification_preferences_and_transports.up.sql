-- One person's chosen transport per notification class. A missing row is
-- "never chosen" and follows the compiled default, which is a different
-- fact from choosing 'in_app' — the app_user delivery columns' rule.
SET LOCAL lock_timeout = '3s';

CREATE TABLE notification_preference (
  user_id    uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  class      text NOT NULL,
  delivery   text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, class),
  CONSTRAINT notification_preference_delivery_check
    CHECK (delivery IN ('off', 'in_app', 'email', 'digest'))
);

-- The morning goes out once: one claim row per (recipient, local day),
-- taken by a conditional insert exactly one worker tick wins.
CREATE TABLE notification_digest_run (
  user_id           uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  digest_date       date NOT NULL,
  mail_attempted_at timestamptz NOT NULL DEFAULT now(),
  mail_error        text,
  PRIMARY KEY (user_id, digest_date),
  CONSTRAINT notification_digest_error_bounded CHECK (length(mail_error) <= 500)
);

-- The immediate-email attempt stamp: claimed once, never released; a
-- failure records its cause beside the claim rather than retrying.
--
-- NOT VALID, and the VALIDATE is a migration of its own rather than the next
-- line here. A plain ADD CONSTRAINT scans every existing row while holding
-- ACCESS EXCLUSIVE, and `lock_timeout` does not help: it bounds the wait to
-- ACQUIRE a lock, never how long one is held. On a mature notice table — a row
-- per thing the product has ever told anybody — that scan is the outage.
--
-- Splitting it INSIDE one file would not have helped either, which is the part
-- worth knowing before copying the shape from elsewhere in this directory: the
-- runner wraps each migration's whole script in one transaction, so a VALIDATE
-- on the next line still runs under the ACCESS EXCLUSIVE this statement took
-- and holds until commit. Only a separate migration gets its own transaction,
-- and with it the SHARE UPDATE EXCLUSIVE that readers and writers pass.
ALTER TABLE notice
  ADD COLUMN email_attempted_at timestamptz,
  ADD COLUMN email_error text,
  ADD CONSTRAINT notice_email_error_after_attempt
    CHECK (email_error IS NULL OR email_attempted_at IS NOT NULL) NOT VALID;
