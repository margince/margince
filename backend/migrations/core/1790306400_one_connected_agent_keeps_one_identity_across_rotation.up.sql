-- ALTER TABLE takes ACCESS EXCLUSIVE on a table this migration did not create,
-- so it queues behind every open transaction touching approval and blocks every
-- writer while it waits. Bounded: failing fast is recoverable, an unbounded
-- stall on the approvals table is not.
SET LOCAL lock_timeout = '3s';

-- Which CONNECTION staged a proposal, alongside which passport did.
--
-- passport_id alone cannot answer "was this me?": a refresh spends the token,
-- retires the passport and mints a replacement under the same grant, so one
-- connected agent's id changes while nothing about the agent does. Three rules
-- read that comparison and a rotation breaks all three -- the proposer stops
-- recognising its own proposal at redemption and at poll, and stops being
-- refused when it approves it.
--
-- NULL is a proposal no connection staged: a human's, the server's, or one made
-- on a passport a human minted directly (those carry no oauth_grant_id and are
-- already identified by passport_id alone).
ALTER TABLE approval ADD COLUMN staged_by_connection uuid;

ALTER TABLE approval
    ADD CONSTRAINT approval_staged_by_connection_fkey
    FOREIGN KEY (staged_by_connection) REFERENCES oauth_grant(id) ON DELETE SET NULL;

-- Backfill from the passport that staged each row, so the rule binds proposals
-- that are ALREADY pending rather than only ones staged from here on. The
-- passport row survives its own revocation (rotation stamps revoked_at rather
-- than deleting), so a rotated-away credential still names its connection.
UPDATE approval a
   SET staged_by_connection = p.oauth_grant_id
  FROM passport p
 WHERE p.id = a.passport_id
   AND p.oauth_grant_id IS NOT NULL;
