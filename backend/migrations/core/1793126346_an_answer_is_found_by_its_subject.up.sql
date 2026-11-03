-- pgmigrate:no-transaction
-- The same-subject answer arm finds a sender's later mail by address, kind and
-- normalised subject together. Keyed on the address alone, each inbound message
-- read every later attested mail to its sender and normalised each subject, so
-- the cost grew with the square of one sender's outbound history. The subject
-- expression is storekit.NormalisedSubjectSQL over the column, spelled the same
-- or the planner cannot match it. It keeps the old index's leading columns, so
-- it serves every read the old one did. Built concurrently, so writes are never
-- blocked for the build. The first drop clears an invalid index a failed build
-- left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_answer_mail_subject;
CREATE INDEX CONCURRENTLY idx_activity_answer_mail_subject
    ON activity (counterparty_email, kind,
        (lower(btrim(regexp_replace(regexp_replace(coalesce(subject, ''),
            '^(\s*(re|aw|antw)\s*:)+\s*', '', 'i'), '\s+', ' ', 'g')))),
        occurred_at)
    WHERE direction = 'outbound'
      AND counterparty_outbound_attested
      AND restricted_at IS NULL
      AND audience = 'workspace'
      AND archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_answer_mail;
