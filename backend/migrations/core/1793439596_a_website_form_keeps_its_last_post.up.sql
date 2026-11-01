-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- When a website form last landed a post, so its settings row can say whether
-- the pasted code works. A table of its own rather than a column on web_form,
-- because every web_form update bumps the version an admin's edit holds.
SET LOCAL lock_timeout = '3s';

CREATE TABLE web_form_last_post (
    form_id      uuid PRIMARY KEY REFERENCES web_form (id) ON DELETE CASCADE,
    last_post_at timestamptz NOT NULL
);
