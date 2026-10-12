-- A lead carries a phone number.
--
-- A lead already carries one email and one LinkedIn URL as columns of its own,
-- rather than through the sidecar tables a contact uses: a lead is one inbound
-- enquiry and has one of each until it is promoted. The phone follows that
-- shape.
--
-- No format CHECK, which matches contact_phone.phone beside it. E.164 is a
-- contract the writers hold through values.ParsePhone, and a constraint here
-- that the contact's own number does not carry would refuse an import the
-- contact tables accept.
--
-- A nullable column with no default, so existing rows are untouched and the
-- table is not rewritten.
SET LOCAL lock_timeout = '3s';
ALTER TABLE lead ADD COLUMN phone text;
