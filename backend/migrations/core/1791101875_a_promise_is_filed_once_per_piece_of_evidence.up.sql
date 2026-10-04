-- A claim is filed once per contact, message and piece of evidence.
--
-- An extractor reads the same conversation again whenever it moves, and the
-- claim it files is how a human's dismissal is remembered. Without this index
-- a second reading files a second claim beside the dismissed one, and the
-- promise comes back. Live duplicates already stored are archived first,
-- keeping the oldest, which is the one any correction was made against.
SET LOCAL lock_timeout = '3s';

UPDATE conversation_claim c
   SET archived_at = now()
  FROM (SELECT id, row_number() OVER (
                 PARTITION BY contact_id, source_activity_id, evidence_fingerprint
                 ORDER BY created_at, id) AS rank
          FROM conversation_claim
         WHERE archived_at IS NULL) ranked
 WHERE c.id = ranked.id AND ranked.rank > 1;

CREATE UNIQUE INDEX conversation_claim_evidence_ux
    ON conversation_claim (contact_id, source_activity_id, evidence_fingerprint)
 WHERE archived_at IS NULL;
