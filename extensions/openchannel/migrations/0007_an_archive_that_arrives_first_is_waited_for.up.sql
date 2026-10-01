-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- An activity id whose archive reached this unit before the landing that named
-- it, so the landing can answer for it.
--
-- WHY A MARKER RATHER THAN A RE-READ. `activity_id` is written atomically with
-- `state = landed`, so the archive subscription's UPDATE matches nothing while
-- the landing transaction is still open. Asking the activity's own state at
-- landing time would be the smaller answer and is not available: the extension
-- SDK reaches this unit's tables and offers no read of a core record, and
-- widening that for one connector's race is a decision of a different size.
--
-- WHY IT HOLDS EVERY ARCHIVE THIS UNIT DID NOT MATCH. `activity.archived` is
-- workspace-wide and carries no payload, so the handler cannot tell an activity
-- it is about to land from one that was never its business. It records both and
-- the landing consumes the one that was its own; the rest expire.
--
-- The expiry is for THE LANDING THAT NEVER CAME, not a retention policy: a row
-- here is one uuid and a timestamp, with no subject data to age out.

CREATE TABLE ext.ext_openchannel_archived_before_landing (
    activity_id uuid PRIMARY KEY,
    noticed_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE ext.ext_openchannel_archived_before_landing IS
    'Activity ids archived before this unit claimed them, consumed by the landing that names one.';

COMMENT ON COLUMN ext.ext_openchannel_archived_before_landing.noticed_at IS
    'When the archive arrived, so a marker whose landing never came can expire.';

CREATE INDEX ext_openchannel_archived_before_landing_noticed_at_idx
    ON ext.ext_openchannel_archived_before_landing (noticed_at);

-- The app role runs the unit's handlers: the archive subscription inserts here
-- and the landing deletes. TRUNCATE is deliberately absent, as it is on this
-- unit's other tables — nothing empties this in one shot, and a privilege
-- nothing reaches for is one more thing a compromised unit could.
--
-- Conditional because a throwaway database applying this under its owning role
-- alone has no margince_app role at all.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_openchannel_archived_before_landing TO margince_app;
  END IF;
END $$;
