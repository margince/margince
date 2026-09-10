-- 1789018391: mfa_totp_enrolment_and_recovery_codes (down).
--
-- Bounded, because DROP TABLE takes ACCESS EXCLUSIVE and dropping a table whose
-- foreign key references app_user briefly locks app_user too — a table this
-- migration did not create and one the send path writes on constantly. An open
-- transaction holding a conflicting lock would otherwise stall those writes for
-- as long as this is willing to queue. Failing the rollback is the better
-- outcome: visible, and retryable a moment later.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS mfa_recovery_code;
DROP TABLE IF EXISTS user_mfa;
