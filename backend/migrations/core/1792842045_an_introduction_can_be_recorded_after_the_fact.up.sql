SET LOCAL lock_timeout = '3s';

-- An introduction that already happened outside Margince has no ask behind it,
-- so it cannot pass through requested, accepted and introduced. `origin` is a
-- separate axis from `status`: a recorded row is born `introduced` and is told
-- apart from an ask that was marked, which every status-driven reader needs.
-- In a recorded row requester_user_id is the colleague who was introduced and
-- introducer_user_id the colleague who introduced.
ALTER TABLE intro_request
  ADD COLUMN origin text NOT NULL DEFAULT 'asked',
  ADD COLUMN recorded_channel text;

-- A recorded row has no reason to give and no deadline to meet. A placeholder
-- would read as something a colleague typed.
ALTER TABLE intro_request ALTER COLUMN internal_reason DROP NOT NULL;
ALTER TABLE intro_request ALTER COLUMN due_at DROP NOT NULL;
ALTER TABLE intro_request DROP CONSTRAINT intro_request_reason_present;

-- NOT VALID because every existing row is an ask and satisfies each check; the
-- validation runs in the migration beside this one.
ALTER TABLE intro_request
  ADD CONSTRAINT intro_request_origin_known
  CHECK (origin IN ('asked', 'recorded')) NOT VALID,
  ADD CONSTRAINT intro_request_recorded_channel_known
  CHECK (recorded_channel IS NULL OR recorded_channel IN ('in_person', 'email', 'call', 'other')) NOT VALID,
  ADD CONSTRAINT intro_request_channel_iff_recorded
  CHECK ((origin = 'recorded') = (recorded_channel IS NOT NULL)) NOT VALID,
  ADD CONSTRAINT intro_request_reason_by_origin
  CHECK (
    (origin = 'asked' AND internal_reason IS NOT NULL AND internal_reason <> '')
    OR (origin = 'recorded' AND internal_reason IS NULL)) NOT VALID,
  ADD CONSTRAINT intro_request_asked_has_due
  CHECK (origin <> 'asked' OR due_at IS NOT NULL) NOT VALID,
  ADD CONSTRAINT intro_request_recorded_is_introduced
  CHECK (origin <> 'recorded' OR (
    status = 'introduced'
    AND introduced_at IS NOT NULL
    AND through_contact_id IS NULL
    AND decision_reason IS NULL
    AND suggested_user_id IS NULL
    AND decided_at IS NULL
    AND name_dropped_at IS NULL
    AND replied_at IS NULL
    AND source_activity_id IS NULL
    AND closed_at IS NULL)) NOT VALID;

-- A double click or a retry answers a conflict instead of writing a second row.
-- A recorded row is never in an open status, so intro_request_open_route does
-- not see it.
CREATE UNIQUE INDEX intro_request_recorded_pair
  ON intro_request (contact_id, introducer_user_id, requester_user_id)
  WHERE origin = 'recorded' AND archived_at IS NULL;
