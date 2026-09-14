SET LOCAL lock_timeout = '3s';

-- A link that tells somebody what is held about them, and asks for nothing.
--
-- The installation could already discharge an Art. 13 or Art. 14 disclosure
-- duty, through the record-confirmation mail. That message also asks whether
-- the reader wants to hear from us — a marketing question riding a legal
-- obligation, which is the arrangement a supervisory authority reads as consent
-- obtained under pressure.
--
-- It also could not reach the people who most needed it. A contact who asked us
-- to stop is still owed their disclosure: Art. 18(2) and the notice duties
-- survive a restriction. Only the privacy_notice CATEGORY survives that stop in
-- the engine (consent/authorizesuppression.go), and the record confirmation
-- carries a different one — so the duty was owed and undeliverable, with
-- nothing anywhere saying so.
ALTER TABLE confirm_token
    DROP CONSTRAINT confirm_token_kind;

ALTER TABLE confirm_token
    ADD CONSTRAINT confirm_token_kind
        CHECK (kind = ANY (ARRAY[
            'record_confirmation'::text,
            'consent_confirmation'::text,
            'privacy_notice'::text
        ]));

-- A privacy notice names no purpose, exactly as a record confirmation does not.
-- The existing shape constraint already says a purpose belongs to a consent
-- confirmation alone, so it needs no change — stated here because the next
-- reader will check.

-- EXISTING ART. 14 DUTIES GAIN THE ROUTE.
--
-- allowed_routes is stamped when a case is opened, so a duty recorded before
-- this migration names only the record confirmation. The new rule adds the
-- notice route for cases opened from now on; without this the existing ones
-- could never be discharged by a notice — and they are the ones that have been
-- owed longest.
--
-- The notice goes FIRST, matching the rule's own order: it is the route that
-- works for every contact, including one who has asked us to stop.
--
-- Art. 13 cases are untouched. Their route is the reply somebody is already
-- sending, and a notice mail is not that.
UPDATE privacy_notice_case
   SET allowed_routes = ARRAY['privacy_notice'] || allowed_routes,
       updated_at = now()
 WHERE rule = 'art14'
   AND NOT ('privacy_notice' = ANY(allowed_routes));
