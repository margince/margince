-- Back to the vocabulary without the legal-hold verbs. A row already carrying
-- one would fail the narrower constraint, so this is safe only on a database
-- that never placed a hold — which is what a down migration of a just-applied
-- version is for.
--
-- The list is the head's, verb for verb. Rebuilt from an older migration it
-- would silently drop whatever was added since — `delete` was exactly that,
-- and the audit-coherence gate is what caught it.
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
                      'pause', 'resume', 'close', 'invite', 'revoke', 'delete'));
