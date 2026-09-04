-- Narrowing the CHECK back cannot succeed while any row carries the four Deal Room lifecycle verbs this migration added,
-- and this migration REFUSES rather than trying to make room.
--
-- It used to open with `DELETE FROM audit_log WHERE action ...`, under a comment
-- saying the rows "go first". They do not and never did: audit_log carries
-- trg_audit_no_mutate, a BEFORE DELETE OR UPDATE trigger that raises
-- `audit_log is append-only` on the first matching row. So the statement was
-- either a no-op (no row carries the verb) or an abort — it never once removed
-- an audit row, and three migrations described it as if it had.
-- margince/margince#496.
--
-- Refusing says the same thing the trigger would, earlier and in the operator's
-- terms: the ledger is append-only, so a vocabulary that has been WRITTEN cannot
-- be narrowed, and the way back is to leave the verb in the CHECK. A trigger
-- error names a row id; this names the decision.
--
-- No row_security change and no workspace bind, because audit_log carries
-- neither: the committed schema records it `rls=false force=false` and no
-- migration enables it. An earlier reading of this gap assumed FORCE RLS made
-- the probe blind; it does not exist to be blind.

DO $$
DECLARE
    written bigint;
BEGIN
    SELECT count(*) INTO written FROM audit_log WHERE action IN ('publish', 'pause', 'resume', 'close');
    IF written > 0 THEN
        RAISE EXCEPTION
            'audit_log holds % row(s) written under a verb this migration would remove from audit_log_action_check, and audit_log is append-only', written
            USING ERRCODE = 'check_violation',
                  HINT = 'The rows cannot be deleted and the narrowed CHECK cannot validate around them. Leave the verb in the vocabulary, or reverse to a point before it was ever written.';
    END IF;
END;
$$;
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_check_v1
    CHECK (action IN ('create', 'update', 'archive', 'merge', 'promote', 'restore',
                      'export', 'erase', 'assign', 'advance_stage', 'advance_phase',
                      'approve', 'reject', 'consent_grant', 'consent_withdraw',
                      'activity_relink', 'record_share', 'record_unshare', 'resolve',
                      'demote', 'import', 'import_undo', 'disqualify', 'anonymize',
                      'send_email', 'reset_data', 'password_link_issued', 'connect',
                      'disconnect', 'schedule', 'reschedule', 'cancel', 'release',
                      'hold', 'expire', 'restrict', 'pin', 'accrue', 'pay')) NOT VALID;

ALTER TABLE audit_log VALIDATE CONSTRAINT audit_log_action_check_v1;

ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_check;

ALTER TABLE audit_log RENAME CONSTRAINT audit_log_action_check_v1 TO audit_log_action_check;
