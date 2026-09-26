-- A contact kept visible to its owner alone now records WHY.
--
-- Four decisions narrow a contact, and only one of them should end when the
-- address answers: mail we sent that nobody has replied to yet. A confidentiality
-- hold, an advisor verdict and a human's own "make private" must survive a reply.
-- Visibility alone cannot tell them apart, so a widening could not ask whether
-- THIS narrowing is one it may lift. The reason is written by the decision that
-- narrowed the row, because re-deriving it later reads present state and gets a
-- lifted hold wrong.
--
-- NULL on an owner-scoped row means no decision is on record: a contact capture
-- minted while its sender was unjudged, or one narrowed before this column
-- existed. A reply lifts neither.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    ADD COLUMN IF NOT EXISTS narrowing_reason text,
    ADD CONSTRAINT contact_narrowing_reason_check CHECK (narrowing_reason IN (
        'outbound_no_answer', 'confidentiality_hold', 'advisor', 'human_decided')),
    -- A reason describes a row that is narrowed. Every widening clears it, and a
    -- widening that forgets to fails here rather than leaving a stale reason for
    -- the next narrowing to be read by.
    ADD CONSTRAINT contact_narrowing_reason_only_when_owner CHECK (
        narrowing_reason IS NULL OR visibility = 'owner');

COMMENT ON COLUMN contact.narrowing_reason IS
    'Why an owner-scoped contact is its owner''s alone. An inbound reply widens only outbound_no_answer; NULL is no recorded decision and widens on no reply.';
