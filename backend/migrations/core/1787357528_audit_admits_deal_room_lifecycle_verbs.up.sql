-- The Deal Room lifecycle writes four verbs the audit CHECK does not yet admit,
-- and an audit row that fails its CHECK takes the whole mutation down with it:
-- publishing a room would roll back rather than record.
--
-- None of the four reuses an existing verb, because none means the same thing.
-- `publish` puts editorial text in front of a party outside the company —
-- nothing else in this vocabulary crosses that boundary. `close` freezes a
-- room's content while buyer access deliberately continues, which `archive`
-- does not describe. `pause`/`resume` suspend reads without touching a single
-- credential; `hold`/`release` are retention verbs and `expire` is scheduling,
-- so borrowing any of them would make an access change read as a data-lifecycle
-- one in every audit query that groups by action.
--
-- ADD ... NOT VALID then VALIDATE, rather than one validated ADD.
--
-- Stated honestly, because the usual reason does NOT apply here and the
-- correction is worth more than the tidy version: the runner wraps each
-- migration file in a single transaction (dbmigrate.go's inTx), so ADD
-- CONSTRAINT's ACCESS EXCLUSIVE is held until the file commits — THROUGH the
-- VALIDATE. Postgres does not downgrade a lock it already holds, so the split
-- shortens nothing here.
--
-- (This paragraph used to claim it did, and the claim was copied into a later
-- migration before a review caught it. A wrong sentence about locking is worse
-- than none: it is the kind a reader trusts because it sounds careful.)
--
-- The split is kept anyway, for a reason that survives: the two statements say
-- what they each do, and a table where the scan is the expensive part wants the
-- VALIDATE in a migration of its OWN — a second file, which commits separately
-- and does get the downgrade. Writing it as a pair here is the shape that
-- change starts from.
--
-- The transaction is also what makes this safe to interrupt: a failure at any
-- statement rolls the whole file back, so audit_log can never be left with no
-- action constraint at all.
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_check_v2
    CHECK (action IN ('create', 'update', 'archive', 'merge', 'promote', 'restore',
                      'export', 'erase', 'assign', 'advance_stage', 'advance_phase',
                      'approve', 'reject', 'consent_grant', 'consent_withdraw',
                      'activity_relink', 'record_share', 'record_unshare', 'resolve',
                      'demote', 'import', 'import_undo', 'disqualify', 'anonymize',
                      'send_email', 'reset_data', 'password_link_issued', 'connect',
                      'disconnect', 'schedule', 'reschedule', 'cancel', 'release',
                      'hold', 'expire', 'restrict', 'pin', 'accrue', 'pay',
                      'publish', 'pause', 'resume', 'close')) NOT VALID;

ALTER TABLE audit_log VALIDATE CONSTRAINT audit_log_action_check_v2;

ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_check;

ALTER TABLE audit_log RENAME CONSTRAINT audit_log_action_check_v2 TO audit_log_action_check;
