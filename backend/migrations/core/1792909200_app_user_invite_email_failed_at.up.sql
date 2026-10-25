-- 1792909200: app_user_invite_email_failed_at.
--
-- When the relay last refused the mail that carries a member's invitation link.
-- NULL means no send has failed, which is also the answer for a member whose
-- invitation went out. The roster reads it only while the seat is `invited`.
ALTER TABLE app_user ADD COLUMN invite_email_failed_at timestamptz;
