-- A company carries one main phone number and one general email address.
--
-- They are columns of the company, as a lead's are, because a company has one
-- switchboard and one general mailbox. A contact's sidecar tables hold numbers
-- of a human, and a switchboard there read as that human's.
--
-- No format CHECK, which matches lead.phone and contact_phone.phone. E.164 and
-- the address rule are held by the writers through values.ParsePhone and
-- values.ParseEmail.
--
-- Nullable columns with no default, so existing rows are untouched and the
-- table is not rewritten.
SET LOCAL lock_timeout = '3s';
ALTER TABLE company ADD COLUMN phone text, ADD COLUMN email text;
