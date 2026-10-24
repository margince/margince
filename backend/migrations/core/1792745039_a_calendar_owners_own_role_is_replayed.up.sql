-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The attendee repair now also records the calendar owner's own organizer or
-- attendee role. Meetings settled under the first rule carry version 1, so
-- the repair offers them once more; a new marker is written at version 2.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_meeting_attendee_repair ADD COLUMN rule_version integer NOT NULL DEFAULT 1;
