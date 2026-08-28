-- The records a scheduled REPLY was told to file itself under, beyond the ones
-- it inherits from its anchor.
--
-- A separate column rather than a second meaning for origin_links, which on an
-- account row is the WHOLE set. One column answering two questions depending on
-- origin_kind is a column a reader has to know the kind to interpret, and the
-- origin_shape check exists precisely so each kind's columns say what they are.
--
-- Null on an account origin, and on a reply that named nothing: a reply with no
-- additions is every reply sent before this column existed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE scheduled_send
    ADD COLUMN also_links jsonb;

-- ONE validated ADD, not the NOT VALID + VALIDATE pair the tree uses
-- elsewhere, and this is the file to say why because the pair looks like the
-- careful choice.
--
-- It buys nothing HERE. dbmigrate runs each migration file inside one
-- transaction (dbmigrate.go's inTx), and Postgres holds ADD CONSTRAINT's ACCESS
-- EXCLUSIVE until that transaction commits — through the VALIDATE scan. The
-- two-step shortens the lock only when the ADD commits first and VALIDATE runs
-- in a LATER transaction, which this runner does not do. Writing the pair here
-- would have cost a statement and told the next reader something untrue about
-- what it bought.
--
-- The scan itself is cheap for a reason this file can point at: also_links was
-- added a statement ago, so every existing row holds NULL and satisfies the
-- constraint. scheduled_send is also small by construction — it holds messages
-- waiting for their moment, not history.
--
-- A table where that arithmetic came out differently would want the validation
-- in a migration of its own, which is a second file rather than a second
-- statement.
ALTER TABLE scheduled_send
    ADD CONSTRAINT scheduled_send_also_links_shape
    CHECK (also_links IS NULL
           OR (origin_kind = 'reply'::text AND jsonb_typeof(also_links) = 'array'::text));
