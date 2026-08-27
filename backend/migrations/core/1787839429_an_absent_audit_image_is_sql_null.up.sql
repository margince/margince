-- An audit image that says nothing reaches the column as SQL NULL, not as the
-- JSON scalar `null`.
--
-- The two look alike and answer differently. Every "there was no prior state"
-- query in this tree is `WHERE before IS NULL`, and a column holding the four
-- bytes `null` is not null — it misses the row silently. `coalesce(before,
-- after)` is wrong for the same reason: it returns the JSON null rather than
-- falling through to the after-image.
--
-- storekit.AbsentImage is the chokepoint and it already answers this correctly,
-- for an untyped nil and for a typed one alike. What it cannot cover is
-- everything that does not go through it: the approvals module INSERTs its own
-- audit row, and a caller that marshals an image itself hands over bytes this
-- seam never sees. A constraint binds the table rather than the door.
--
-- NOT VALID, and it stays that way. audit_log is append-only, so the rows
-- written before storekit was hardened cannot be rewritten — and validating
-- would fail the migration on exactly those installations, blocking an upgrade
-- over history nobody can change. What this constraint is for is the next
-- write, and NOT VALID binds every one of them.
--
-- The predicate reads as false only for the JSON scalar: `before <> 'null'` is
-- NULL when before IS NULL, and a CHECK admits NULL. So an absent image passes
-- and a `null` image is refused, which is the whole distinction.
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log ADD CONSTRAINT audit_log_images_are_absent_or_present
    CHECK (before <> 'null'::jsonb AND after <> 'null'::jsonb)
    NOT VALID;
