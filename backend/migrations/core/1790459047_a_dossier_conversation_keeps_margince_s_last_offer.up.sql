-- The one value Margince's latest reply in a dossier conversation offered to
-- apply, recorded by the server when it made the offer. A bare yes from the same
-- administrator, as their very next message, grants exactly that field and value;
-- the client's copy of the conversation carries no offer and is never asked for one.
--
-- One slot per read, replaced by every answered message, so an offer outlives
-- nothing but the turn that follows it. NULL is no standing offer.
SET LOCAL lock_timeout = '3s';

ALTER TABLE site_read ADD COLUMN conversation_offer jsonb;
