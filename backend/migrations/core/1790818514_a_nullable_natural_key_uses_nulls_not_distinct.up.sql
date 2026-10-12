-- A NULL is distinct from every other NULL in a unique index, so a natural key
-- with a nullable column does not constrain the rows where that column is null.
-- Eight indexes solved that by COALESCEing to a sentinel; Postgres 15 gave the
-- engine NULLS NOT DISTINCT, and retention_policy already uses it.
--
-- The sentinel is the cost. Six of the eight reached for the nil UUID — eight
-- columns in all, since activity_participant and scheduled_send each default two.
-- It is the value the contract calls forbidden as a placeholder on
-- deal.pipeline_id, so a row that actually stored it would collide with the null
-- row and be refused with a uniqueness error naming neither cause. The '' sentinels have the same shape:
-- COALESCE(normalized_company, '') cannot tell "no company" from a company whose
-- name is empty, and nothing forbids the empty string there.
--
-- FOUR are left alone, because their COALESCE is not a null sentinel:
--
--   activity_link, dedupe_candidate and provider_applied_field coalesce across
--   SEVERAL REAL COLUMNS — "whichever one is set", which NULLS NOT DISTINCT
--   cannot express. activity_link_shape already guarantees exactly one is.
--
--   relationship's uq_rel_employment carries its COALESCE in the PREDICATE, not
--   in the column list, so there is no sentinel column to convert.
--
-- sales_target converts although its sentinels are strings: it coalesces
-- (scope_id)::text to 'workspace' and (pipeline_id)::text to 'all', and a uuid
-- column cannot hold either word — so the grouping is the same and the index
-- stops casting to text to say so.
--
-- scheduled_send keeps its two md5(COALESCE(…jsonb)) columns. Those compute a
-- digest of a payload that may be absent; md5 of anything is never null, so
-- NULLS NOT DISTINCT has nothing to do there and the default belongs inside the
-- expression.
-- Not CONCURRENTLY: a migration runs in one transaction and CONCURRENTLY forbids
-- that (see 1787320004's note on the same point), so each build here is
-- write-blocking for its duration, and capture_trace is written on every capture
-- the deployment evaluates. The builds are over whole tables rather than bounded
-- by a predicate, so the cost is the row count — and the timeout is what keeps a
-- queued build from stalling those writes for as long as it is willing to wait.
--
-- The file is one transaction, so a timeout aborts the whole conversion and
-- applies none of it. That is the safe direction: a half-converted schema would
-- leave some upserts matching an index and some not.
SET LOCAL lock_timeout = '3s';

DROP INDEX uq_role_assignment;
CREATE UNIQUE INDEX uq_role_assignment ON role_assignment
    USING btree (role_id, user_id, team_id) NULLS NOT DISTINCT;

DROP INDEX uq_capture_exclusion;
CREATE UNIQUE INDEX uq_capture_exclusion ON capture_exclusion
    USING btree (scope, user_id, kind, value) NULLS NOT DISTINCT;

DROP INDEX capture_trace_natural_key;
CREATE UNIQUE INDEX capture_trace_natural_key ON capture_trace
    USING btree (user_id, source_system, source_id, stage, outcome) NULLS NOT DISTINCT;

DROP INDEX uq_activity_participant;
CREATE UNIQUE INDEX uq_activity_participant ON activity_participant
    USING btree (activity_id, role, user_id, contact_id, address, channel_user_id) NULLS NOT DISTINCT;

DROP INDEX intro_request_open_route;
CREATE UNIQUE INDEX intro_request_open_route ON intro_request
    USING btree (contact_id, introducer_user_id, through_contact_id) NULLS NOT DISTINCT
    WHERE status = ANY (ARRAY['requested'::text, 'accepted'::text, 'name_drop_approved'::text])
      AND archived_at IS NULL;

DROP INDEX uq_linkedin_connection_natural;
CREATE UNIQUE INDEX uq_linkedin_connection_natural ON linkedin_connection
    USING btree (owner_user_id, normalized_name, normalized_company, connected_on) NULLS NOT DISTINCT
    WHERE provider_member_ref IS NULL;

DROP INDEX sales_target_identity;
CREATE UNIQUE INDEX sales_target_identity ON sales_target
    USING btree (metric, scope_kind, scope_id, pipeline_id, period_kind, period_start) NULLS NOT DISTINCT;

DROP INDEX scheduled_send_one_held_message_per_seat;
CREATE UNIQUE INDEX scheduled_send_one_held_message_per_seat ON scheduled_send
    USING btree (held_reason, scheduled_by, principal_kind, payload_version, origin_kind,
                 agent_actor_id, agent_passport_id, anchor_activity_id,
                 md5((COALESCE(origin_links, '[]'::jsonb))::text),
                 md5((COALESCE(also_links, '[]'::jsonb))::text),
                 md5((payload)::text)) NULLS NOT DISTINCT
    WHERE status = 'held'::text AND held_reason = 'send_refused'::text;
