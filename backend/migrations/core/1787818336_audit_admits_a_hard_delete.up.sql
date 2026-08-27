-- Destroying a document, its passages, its vectors and its stored file.
--
-- No existing verb says it. `archive` is the closest and is wrong in the way
-- that matters: an archived row is still there, and the whole point of this act
-- is that the bytes are gone. `erase` is the other candidate and is wrong for a
-- different reason — it is this product's Art. 17 verb, and an auditor
-- filtering the trail for subject erasures would find corpus housekeeping mixed
-- in with them. A verb whose meaning is "somebody removed a file they had
-- uploaded" is not the verb for "a data subject exercised a right".
--
-- ADD ... NOT VALID then VALIDATE, then swap: the runner wraps each migration
-- file in one transaction, so a failure at any statement rolls the whole file
-- back and audit_log can never be left with no action constraint. The two-step
-- still shortens the ACCESS EXCLUSIVE hold, since NOT VALID takes that lock
-- without scanning and VALIDATE drops to SHARE UPDATE EXCLUSIVE for the pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_check_v4
    CHECK (action IN ('create', 'update', 'archive', 'merge', 'promote', 'restore',
                      'export', 'erase', 'assign', 'advance_stage', 'advance_phase',
                      'approve', 'reject', 'consent_grant', 'consent_withdraw',
                      'activity_relink', 'record_share', 'record_unshare', 'resolve',
                      'demote', 'import', 'import_undo', 'disqualify', 'anonymize',
                      'send_email', 'reset_data', 'password_link_issued', 'connect',
                      'disconnect', 'schedule', 'reschedule', 'cancel', 'release',
                      'hold', 'expire', 'restrict', 'pin', 'accrue', 'pay', 'publish',
                      'pause', 'resume', 'close', 'invite', 'revoke', 'delete')) NOT VALID;

ALTER TABLE audit_log VALIDATE CONSTRAINT audit_log_action_check_v4;

ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_check;

ALTER TABLE audit_log RENAME CONSTRAINT audit_log_action_check_v4 TO audit_log_action_check;
