-- Puts the check back as this migration found it: present and unvalidated.
-- Postgres cannot un-validate, so it is dropped and added NOT VALID, no scan.
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
                      'place_legal_hold', 'lift_legal_hold',
                      'request_legal_review', 'withdraw_legal_review',
                      'mfa_removed')) NOT VALID;
