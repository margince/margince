-- A grant names the question it answered.
--
-- consent_event.policy_text is the sentence a proof row quotes as what the
-- subject agreed to, and on the confirm-link door it came from the SUBMISSION
-- BODY — whatever the subject's own browser said it had shown them. The
-- evidence a controller produces to show consent was freely given therefore
-- quotes a string the client chose, and a client that chose a different one
-- would be believed. consent_event.consent_text_version_id has existed since
-- 1788669014 with nothing writing it.
--
-- The question is now published server-side (consent/marketingquestion.go), in
-- every language this build speaks. Binding it means knowing WHICH published
-- row the subject read, and a published wording is identified by key, version
-- AND locale.
--
-- BOTH GO ON THE TOKEN, because the token is what the subject comes back
-- holding: the submit has it in hand and no delivery row, and reaching through
-- comms_outbound would make the proof depend on a row the erasure engine is
-- entitled to scrub.
--
-- THE VERSION IS PINNED AT MINT, not read from the running build when the link
-- is answered. A link mailed under v1 and clicked after a v2 deploy would
-- otherwise record the subject agreeing to wording they were never shown —
-- which is the same defect as quoting the client, arriving from the other side.
--
-- NULLABLE, and existing rows stay NULL. A link minted before this was recorded
-- was answered against a question we did not pin, and choosing one now — the
-- current version, say — would state as fact something we would be guessing, in
-- the evidence column this exists to make honest. NULL says we do not know, and
-- the writer treats it that way: those grants keep exactly the weaker evidence
-- they already had.

SET LOCAL lock_timeout = '3s';

-- AND WHICH QUESTION, because the two links show different pages. A
-- record-confirmation link asks the generic marketing question with a yes/no; a
-- dedicated consent link names the purpose it was minted for. A subject answers
-- exactly one, and binding either door to the other's sentence would record an
-- agreement to something nobody was asked.
ALTER TABLE confirm_token
    ADD COLUMN question_key text,
    ADD COLUMN question_locale text,
    ADD COLUMN question_version integer;

COMMENT ON COLUMN confirm_token.question_key IS
    'Which published question the confirm page will ask — the generic marketing one, or the dedicated subscription one that names its purpose. NULL on links minted before this was recorded.';

COMMENT ON COLUMN confirm_token.question_locale IS
    'The language the confirm page will ask its subscription question in, recorded when the link is minted so a grant can name the published wording the subject actually read. NULL on links minted before this was recorded.';
COMMENT ON COLUMN confirm_token.question_version IS
    'The version of that question, pinned at mint so a link answered after a redeploy names what the subject saw rather than what is deployed when they click. NULL on links minted before this was recorded.';
