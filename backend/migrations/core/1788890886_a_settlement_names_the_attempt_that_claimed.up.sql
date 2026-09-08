-- A settlement names the attempt that took the claim.
--
-- The claim row is identified by (principal_id, key, endpoint), and that is the
-- whole identity a settle or a release matched on. But the row is RE-CLAIMED in
-- place once it is past the replay window: the digest is overwritten and the
-- recorded response cleared, for a new attempt, under the same three columns.
--
-- So a first attempt still in flight when its claim expired could return and
-- settle the REPLACEMENT's row — overwriting a result a later replay would then
-- serve for the wrong call, or deleting a live claim and letting a third
-- attempt run beside the second.
--
-- attempt_id is what a settlement is predicated on. It is stamped fresh on both
-- the insert and the re-claim, so the two attempts never share one, and a
-- settlement whose attempt is gone writes nothing.
-- uuidv7() is volatile, so this ADD COLUMN rewrites the table under an ACCESS
-- EXCLUSIVE lock rather than taking the metadata-only path a constant default
-- gets. That is affordable HERE and nowhere by default: idempotency_key holds
-- only keys inside the replay window, because the retention sweep deletes the
-- rest, so the rewrite is over a bounded table and not a growing one. The
-- timeout bounds the wait for the lock, not the hold.
SET LOCAL lock_timeout = '3s';

ALTER TABLE idempotency_key
    ADD COLUMN attempt_id uuid NOT NULL DEFAULT uuidv7();
