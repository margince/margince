-- A further address or a postal address a signature filled stays while some
-- mail still states it. Each supporting mail is a key here, with the snippet
-- it quoted, so narrowing one mail drops only its own support.
--
-- A constant default adds the column without rewriting the table.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_profile_field
  ADD COLUMN IF NOT EXISTS signature_support jsonb NOT NULL DEFAULT '{}'::jsonb;
