-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A first-response stamp written by a person moving the lead off New, with no
-- activity behind it, names that person. It lets the lead page say "Marked
-- contacted by …" instead of claiming a reply, and lets a move back to New
-- take the stamp back. Stamps written before this column read as replies.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead ADD COLUMN IF NOT EXISTS first_response_marked_by uuid REFERENCES app_user(id) ON DELETE SET NULL;
