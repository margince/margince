-- One mailed link, two questions it can carry.
--
-- confirm_token already does the thing a double opt-in needs and does it
-- correctly: the server picks the address off the person's own record, only the
-- hash is stored, the plaintext is mailed and never returned, and spending the
-- link is what proves the mailbox. The retired double-opt-in endpoint had none
-- of those properties — it handed the plaintext to an operator who could paste
-- it back, so one person could complete both halves of a round trip whose only
-- value is that the SUBJECT completed it.
--
-- So the second question rides the first mechanism rather than growing a second
-- token table beside it. kind says which question the link asks, and purpose_id
-- names the marketing purpose when it asks about one.

SET LOCAL lock_timeout = '3s';

ALTER TABLE confirm_token
    ADD COLUMN kind text NOT NULL DEFAULT 'record_confirmation',
    ADD COLUMN purpose_id uuid REFERENCES consent_purpose(id) ON DELETE RESTRICT;

ALTER TABLE confirm_token
    ADD CONSTRAINT confirm_token_kind
        CHECK (kind = ANY (ARRAY['record_confirmation'::text, 'consent_confirmation'::text])),
    -- A consent link names the purpose it confirms, and a record link never
    -- does. Without this a consent link could be minted with no purpose and
    -- spent into a grant for nothing in particular.
    ADD CONSTRAINT confirm_token_purpose_is_consents
        CHECK ((kind = 'consent_confirmation') = (purpose_id IS NOT NULL));

-- Supersession is per question. A fresh record-confirmation link must not
-- expire somebody's pending consent link, and the other way round: they answer
-- different things and arrive in different mails.
CREATE INDEX confirm_token_live_by_kind
    ON confirm_token (person_id, kind, purpose_id) WHERE consumed_at IS NULL;
