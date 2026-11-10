-- pgmigrate:no-transaction
-- A send from Margince proves the workspace wrote to every address it carried.
-- The proof read found such a send through the address's participant rows, and
-- a colleague's address is To on every message in their own mailbox. So each
-- message from a colleague read their whole mailbox. This index finds the sends
-- by address directly. The predicate is the proof's sent-by-a-user test, spelled
-- the same or the planner cannot use it. The second index holds only attested
-- outbound mail, so a colleague's address no longer walks every message they
-- sent us. The third holds only rows whose counterparty was stored before
-- capture folded it. Each first drop clears an invalid index a failed build
-- left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_comms_outbound_addressed;
CREATE INDEX CONCURRENTLY idx_comms_outbound_addressed
    ON comms_outbound USING gin (comms_outbound_addressed(recipients, cc))
    WHERE sender_kind = 'user'
      AND status = 'sent'
      AND user_id IS NOT NULL
      AND channel_user_id IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_unfolded_counterparty;
CREATE INDEX CONCURRENTLY idx_activity_unfolded_counterparty
    ON activity (activity_unfolded_counterparty(counterparty_email))
    WHERE activity_unfolded_counterparty(counterparty_email) IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_attested_counterparty;
CREATE INDEX CONCURRENTLY idx_activity_attested_counterparty
    ON activity (counterparty_email)
    WHERE counterparty_outbound_attested;
