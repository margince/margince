-- Bounded because every statement below takes an ACCESS EXCLUSIVE lock on
-- capture_exclusion, which blocks the sink's own reads: a migration that waited
-- behind a long transaction would stall capture rather than fail.
SET LOCAL lock_timeout = '3s';

-- A capture exclusion may name a CONTAINER — a Gmail label, a Graph folder, an
-- IMAP mailbox — and not only an address or a domain.
--
-- The value is provider-qualified ("gmail:private", "graph:<folderId>",
-- "imap:INBOX/Family") because a label id means nothing without the provider
-- whose namespace it belongs to, and one person may have connected two.
ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_kind_check;
ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_kind_check
    CHECK (kind IN ('address', 'domain', 'container'));

-- The lowercase rule stops applying to a container, and that is the point
-- rather than a relaxation.
--
-- An address and a domain are case-insensitive by their own specifications, so
-- folding them is lossless and makes the uniqueness index answer the question a
-- reader means. A provider's container id is an OPAQUE token in that provider's
-- namespace: a Graph folder id is base64url, where two distinct folders can
-- differ only in case, and an IMAP mailbox name is case-sensitive except for
-- INBOX. Folding those would silently merge two containers into one rule and
-- exclude mail the owner never asked to exclude.
ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_value_check;
ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_value_check
    CHECK (value <> '' AND (kind = 'container' OR value = lower(value)));

-- Only the mailbox's owner has a label list, so a container rule is theirs.
-- A workspace-wide one would name a container in one person's mailbox and bind
-- everybody else's connections to an id that means nothing there.
ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_container_is_personal
    CHECK (kind <> 'container' OR scope = 'user');
