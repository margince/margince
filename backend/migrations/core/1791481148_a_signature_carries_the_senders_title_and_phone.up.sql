-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A workspace's signature template is filled in with each sender's own title
-- and phone, which the sender keeps beside their plain-text signature.
SET LOCAL lock_timeout = '3s';

ALTER TABLE email_signature ADD COLUMN title text DEFAULT '' NOT NULL;
ALTER TABLE email_signature ADD COLUMN phone text DEFAULT '' NOT NULL;
