-- A withdrawal link keeps working after the read link it rode with has gone.
--
-- THE DEFECT. preference_token is one credential doing two jobs. It opens the
-- preference centre, which READS the recipient's per-purpose consent state and
-- can GRANT, and it is also the credential the RFC 8058 one-click unsubscribe
-- POST carries. Those two want opposite lifetimes:
--
--   the READ authority should be short, because it is a bearer credential over
--   somebody's consent record sitting in a mailbox forever;
--
--   the WITHDRAWAL should last as long as the mail does, because a person
--   who unsubscribes from a two-year-old newsletter is exercising a right
--   that does not expire.
--
-- Today the short lifetime wins for both: the token slides 30 days, is revoked
-- on rotation, and an expired one answers 404. So a recipient pressing
-- unsubscribe on older mail is told the link is invalid, and the only remaining
-- way to stop the mail is to ask a human — which is precisely the friction
-- Art. 21(3) and RFC 8058 exist to remove.
--
-- THE SPLIT. This table holds withdrawal authority and NOTHING else. It cannot
-- read a consent state, cannot grant, and names a scope it may never exceed. It
-- is therefore safe to keep for years, which is what makes the withdrawal
-- honest.
--
-- HASHED, unlike preference_token, which stores its value in plaintext. This
-- one outlives every rotation and sits in mail archives and proxy logs for
-- years, so a database read must not hand somebody a working withdrawal link
-- for every recipient. The hash is the same construction confirm_token uses.
--
-- LEADS TOO. preference_token.person_id is NOT NULL, so a lead-only recipient
-- gets no unsubscribe surface at all — the send simply carries no header.
-- Exactly one of person_id and lead_id is set here, and neither is required,
-- because an address that resolves to nobody still deserves a working opt-out.
SET LOCAL lock_timeout = '3s';

CREATE TABLE withdrawal_credential (
  id uuid PRIMARY KEY DEFAULT uuidv7(),

  -- The hash, never the token. See the note above.
  token_hash text NOT NULL UNIQUE,

  -- The address the link was written to, kept even when no record holds it:
  -- an opt-out is about where the mail went, and a person may be merged,
  -- archived or erased between the send and the press.
  address text NOT NULL,

  person_id uuid REFERENCES person (id) ON DELETE SET NULL,
  lead_id uuid REFERENCES lead (id) ON DELETE SET NULL,

  -- WHAT THIS LINK MAY DO, and it may never do more. A named-purpose link
  -- stops the one subscription it was minted for; an all-marketing link
  -- performs the same sweep the preference centre's "stop all marketing"
  -- does. Neither reaches business correspondence, which is not a
  -- subscription anybody opted into and so not a thing to unsubscribe from.
  scope text NOT NULL CHECK (scope IN ('named_purpose', 'all_marketing')),
  purpose_id uuid REFERENCES consent_purpose (id) ON DELETE CASCADE,

  issued_at timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz,

  -- A ceiling rather than a sliding window. The link is meant to outlive the
  -- mail, and 24 months is the retention this installation states for the mail
  -- itself; a credential that outlives every copy of the message protects
  -- nobody and only widens the window in which a leaked link works.
  expires_at timestamptz NOT NULL,

  revoked_at timestamptz,
  -- WHY it died, because the four reasons need different answers on the page.
  -- An erased subject must never be told "that address had a subscription";
  -- a superseded credential simply points at a newer one.
  revoked_reason text CHECK (revoked_reason IN
    ('erasure', 'compromise', 'merged_predecessor', 'superseded')),

  -- A revocation states its reason or it is not a revocation. Without this
  -- the honest-answer arms above degrade silently to the generic refusal.
  CONSTRAINT withdrawal_credential_revocation_is_reasoned
    CHECK ((revoked_at IS NULL) = (revoked_reason IS NULL)),

  -- A named-purpose link names its purpose; an all-marketing link must not,
  -- or the scope and the target disagree and the page has two answers.
  CONSTRAINT withdrawal_credential_scope_names_its_target
    CHECK ((scope = 'named_purpose') = (purpose_id IS NOT NULL)),

  -- At most one subject, and a bare address is a legitimate third case.
  CONSTRAINT withdrawal_credential_names_at_most_one_subject
    CHECK (NOT (person_id IS NOT NULL AND lead_id IS NOT NULL))
);

-- ONE LIVE CREDENTIAL per address and scope, so re-sending a newsletter reuses
-- the link the last mail carried instead of minting a second one that works
-- just as well. Two live links for one subscription is two bearer credentials
-- to leak, and revoking one would leave the other working.
CREATE UNIQUE INDEX uq_withdrawal_credential_live
  ON withdrawal_credential (lower(address), scope, coalesce(purpose_id, '00000000-0000-0000-0000-000000000000'::uuid))
  WHERE revoked_at IS NULL;

-- The erasure and merge sweeps reach this table by subject.
CREATE INDEX idx_withdrawal_credential_person ON withdrawal_credential (person_id)
  WHERE person_id IS NOT NULL;
CREATE INDEX idx_withdrawal_credential_lead ON withdrawal_credential (lead_id)
  WHERE lead_id IS NOT NULL;

COMMENT ON TABLE withdrawal_credential IS
  'Withdrawal-only bearer credentials for public unsubscribe links. Holds no read or grant authority, so it can safely outlive the preference token that opens the preference centre.';

-- preference_token gains the same reasoned revocation, because "revoked" alone
-- cannot distinguish an ordinary rotation from an erasure — and the withdrawal
-- adapter below has to refuse the second while still honouring the first.
ALTER TABLE preference_token
  ADD COLUMN revoked_reason text CHECK (revoked_reason IN
    ('rotated', 'expired', 'erasure', 'compromise', 'merged_predecessor'));

-- Backfilled as 'rotated', which is what every existing revocation was: the
-- only writer that revokes today is the mint, replacing a live token with a
-- fresh one. Erasure deletes the rows outright rather than revoking them, so
-- no existing row is being mislabelled here.
UPDATE preference_token SET revoked_reason = 'rotated' WHERE revoked_at IS NOT NULL;

ALTER TABLE preference_token
  ADD CONSTRAINT preference_token_revocation_is_reasoned
    CHECK ((revoked_at IS NULL) = (revoked_reason IS NULL));

COMMENT ON COLUMN preference_token.revoked_reason IS
  'Why the token was retired. A withdrawal made with a legacy preference token is honoured for rotated/expired and refused for erasure/compromise/merged_predecessor.';
