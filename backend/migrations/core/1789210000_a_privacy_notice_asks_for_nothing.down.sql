SET LOCAL lock_timeout = '3s';

-- Tokens minted for a privacy notice have no representation in the old
-- vocabulary. They are DELETED rather than reinterpreted: a notice link is a
-- read-only page that grants nothing, so destroying one costs a reader a page
-- they can be sent again, while relabelling it record_confirmation would turn a
-- link that asks nothing into one that asks for a subscription.
DELETE FROM confirm_token WHERE kind = 'privacy_notice';

-- The route goes back out of every case that carries it, or a duty would name
-- a way to discharge it that no writer honours any more — which is the exact
-- defect the discharge path was built to end.
UPDATE privacy_notice_case
   SET allowed_routes = array_remove(allowed_routes, 'privacy_notice'),
       updated_at = now()
 WHERE 'privacy_notice' = ANY(allowed_routes);

ALTER TABLE confirm_token
    DROP CONSTRAINT confirm_token_kind;

ALTER TABLE confirm_token
    ADD CONSTRAINT confirm_token_kind
        CHECK (kind = ANY (ARRAY[
            'record_confirmation'::text,
            'consent_confirmation'::text
        ]));
