-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The reminder instant a task's browser notification was last claimed for, so
-- a member signed in on two browsers sees one notification. A moved remind_at
-- differs from the claimed one, which arms the reminder again.
CREATE TABLE activity_reminder_delivery (
    activity_id   uuid PRIMARY KEY REFERENCES activity(id) ON DELETE CASCADE,
    delivered_for timestamptz NOT NULL
);
