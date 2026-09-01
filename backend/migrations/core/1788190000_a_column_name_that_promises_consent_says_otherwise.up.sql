-- capture_connection.share_acknowledged_at is a grant timestamp, and its name
-- says something stronger than it means.
--
-- "Acknowledged" reads as a person having been shown something and agreed to
-- it. Nothing shows them anything: the column records the moment the OAuth
-- grant landed, and a rebind re-stamps it. A reader auditing this installation
-- for evidence of consent would find a timestamp per connection and reasonably
-- conclude it was that evidence.
--
-- The name is on the public contract and cannot be changed without a breaking
-- window, so the correction goes where anybody reading the schema will meet it.
-- The consent that German law expects is a document the customer holds, not a
-- column: docs/handbook/compliance.md says so and says the product checks none
-- of it.
SET LOCAL lock_timeout = '3s';

COMMENT ON COLUMN capture_connection.share_acknowledged_at IS
    'When the mail-provider grant landed for this connection; re-stamped on rebind. NOT a record of consent: nothing is shown to the user at connect time and no agreement is captured. Consent, where the law requires it, is a document the customer holds — see docs/handbook/compliance.md.';
