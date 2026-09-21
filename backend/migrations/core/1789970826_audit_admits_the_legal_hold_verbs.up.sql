-- Placing a litigation hold, and lifting it.
--
-- Neither reuses an existing verb. `hold` is the Deal Room's, and `release` is
-- what the retention override does when it releases a record from the
-- statutory floor — which ERASES it. A lift returns a record to the ordinary
-- ladder and destroys nothing, so sharing the word would make the two
-- indistinguishable in the one place an auditor looks to tell them apart.
--
-- `update` is wrong for the reason the Deal Room's verbs were: the question
-- months later is not "when did this row change" but "who decided this record
-- had to be preserved, and on what stated grounds" — and the grounds live in
-- this row's payload, no column carrying them.
--
-- Dropped and re-added plainly rather than the NOT VALID two-step the older
-- verb migrations use: the runner wraps a file in ONE transaction, so the
-- ACCESS EXCLUSIVE the ALTER takes is held across the VALIDATE anyway and the
-- split buys nothing. Held by migrations/notvalidsplit_test.go.
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_check;

ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_check
    CHECK (action IN ('create', 'update', 'archive', 'merge', 'promote', 'restore',
                      'export', 'erase', 'assign', 'advance_stage', 'advance_phase',
                      'approve', 'reject', 'consent_grant', 'consent_withdraw',
                      'activity_relink', 'record_share', 'record_unshare', 'resolve',
                      'demote', 'import', 'import_undo', 'disqualify', 'anonymize',
                      'send_email', 'reset_data', 'password_link_issued', 'connect',
                      'disconnect', 'schedule', 'reschedule', 'cancel', 'release',
                      'hold', 'expire', 'restrict', 'pin', 'accrue', 'pay', 'publish',
                      'pause', 'resume', 'close', 'invite', 'revoke', 'delete',
                      'place_legal_hold', 'lift_legal_hold'));
