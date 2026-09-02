-- A broker between us and the model picks the upstream per request, and the
-- same model id can be served by hosts differing in quantization, output
-- ceiling and tail latency. Without these two columns a call cannot be
-- attributed to what actually ran: a degraded upstream reads as a degraded
-- model, and a truncated answer reads as a malformed one.
--
-- served_identity_source is deliberately untouched. It says how far to trust
-- served_model, and a broker's answer there stays 'echo' — it reflects our own
-- request back while naming the upstream separately. We learn who served
-- without learning what they served, so the two facts stay two columns.
-- ALTER TABLE takes a lock that blocks writers on a table this migration did
-- not create, so the wait is bounded: without a timeout, one open transaction
-- holding a conflicting lock stalls every ai_call write for as long as this
-- migration is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  ADD COLUMN served_provider text DEFAULT ''::text NOT NULL,
  ADD COLUMN finish_reason text DEFAULT ''::text NOT NULL;
