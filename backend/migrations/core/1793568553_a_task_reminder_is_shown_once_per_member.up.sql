-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- The reminder instant and assignee a task's browser notification was last
-- claimed for, so a member signed in on two browsers sees one notification. A
-- moved remind_at or a new assignee differs from the claim, and arms it again.
CREATE TABLE activity_reminder_delivery (
    activity_id   uuid PRIMARY KEY REFERENCES activity(id) ON DELETE CASCADE,
    delivered_for timestamptz NOT NULL,
    delivered_to  uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE
);

CREATE INDEX activity_reminder_delivery_delivered_to_idx ON activity_reminder_delivery (delivered_to);
