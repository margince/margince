-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the reader key and the producer check the link suggestion queue
-- gained NOT VALID, under SHARE UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_link_suggestion VALIDATE CONSTRAINT activity_link_suggestion_for_user_fkey;

ALTER TABLE activity_link_suggestion VALIDATE CONSTRAINT activity_link_suggestion_producer;
