-- Back to the sentinels. Each definition is the one the committed catalog
-- recorded before the conversion, so a down-and-up round trip lands on the shape
-- the gate measures rather than on a retyped approximation of it.
--
-- THIS CAN FAIL, and failing is the honest outcome. The up migration leaves the
-- sentinel values sayable — nothing forbids a real row storing the nil UUID in
-- team_id, or '' in normalized_company — so a database that stored one alongside
-- a NULL in the same key holds two rows these COALESCE indexes map to one. The
-- CREATE then reports a duplicate key.
--
-- It is not papered over with a repair, because there is no safe guess available:
-- which of the two rows is the real one is a question about the records, and
-- picking one here would delete a row somebody entered. The migration runs in one
-- transaction, so a failure leaves the schema as it was and nothing half-dropped;
-- the pair has to be resolved by hand before the old shape can stand again.
SET LOCAL lock_timeout = '3s';

DROP INDEX uq_role_assignment;
CREATE UNIQUE INDEX uq_role_assignment ON role_assignment
    USING btree (role_id, user_id, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'::uuid));

DROP INDEX uq_capture_exclusion;
CREATE UNIQUE INDEX uq_capture_exclusion ON capture_exclusion
    USING btree (scope, COALESCE(user_id, '00000000-0000-0000-0000-000000000000'::uuid), kind, value);

DROP INDEX capture_trace_natural_key;
CREATE UNIQUE INDEX capture_trace_natural_key ON capture_trace
    USING btree (COALESCE(user_id, '00000000-0000-0000-0000-000000000000'::uuid), source_system, source_id, stage, outcome);

DROP INDEX uq_activity_participant;
CREATE UNIQUE INDEX uq_activity_participant ON activity_participant
    USING btree (activity_id, role, COALESCE(user_id, '00000000-0000-0000-0000-000000000000'::uuid), COALESCE(contact_id, '00000000-0000-0000-0000-000000000000'::uuid), COALESCE(address, ''::text), COALESCE(channel_user_id, ''::text));

DROP INDEX intro_request_open_route;
CREATE UNIQUE INDEX intro_request_open_route ON intro_request
    USING btree (contact_id, introducer_user_id, COALESCE(through_contact_id, '00000000-0000-0000-0000-000000000000'::uuid)) WHERE ((status = ANY (ARRAY['requested'::text, 'accepted'::text, 'name_drop_approved'::text])) AND (archived_at IS NULL));

DROP INDEX uq_linkedin_connection_natural;
CREATE UNIQUE INDEX uq_linkedin_connection_natural ON linkedin_connection
    USING btree (owner_user_id, normalized_name, COALESCE(normalized_company, ''::text), COALESCE(connected_on, '1970-01-01'::date)) WHERE (provider_member_ref IS NULL);

DROP INDEX sales_target_identity;
CREATE UNIQUE INDEX sales_target_identity ON sales_target
    USING btree (metric, scope_kind, COALESCE((scope_id)::text, 'workspace'::text), COALESCE((pipeline_id)::text, 'all'::text), period_kind, period_start);

DROP INDEX scheduled_send_one_held_message_per_seat;
CREATE UNIQUE INDEX scheduled_send_one_held_message_per_seat ON scheduled_send
    USING btree (held_reason, scheduled_by, principal_kind, payload_version, origin_kind, COALESCE(agent_actor_id, ''::text), COALESCE(agent_passport_id, '00000000-0000-0000-0000-000000000000'::uuid), COALESCE(anchor_activity_id, '00000000-0000-0000-0000-000000000000'::uuid), md5((COALESCE(origin_links, '[]'::jsonb))::text), md5((COALESCE(also_links, '[]'::jsonb))::text), md5((payload)::text)) WHERE ((status = 'held'::text) AND (held_reason = 'send_refused'::text));
