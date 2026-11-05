-- pgmigrate:no-transaction
-- Restores the address-keyed index before dropping the subject-keyed one, so
-- the arm always has one of them. The first drop clears an invalid index a
-- failed build left behind, so the file is safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_answer_mail;
CREATE INDEX CONCURRENTLY idx_activity_answer_mail
    ON activity (counterparty_email, kind, occurred_at)
    WHERE direction = 'outbound'
      AND counterparty_outbound_attested
      AND restricted_at IS NULL
      AND audience = 'workspace'
      AND archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_answer_mail_subject;
