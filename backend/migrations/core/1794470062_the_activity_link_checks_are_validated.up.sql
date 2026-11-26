-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- VALIDATE takes only SHARE UPDATE EXCLUSIVE, and it runs in a file of its own
-- so the ACCESS EXCLUSIVE lock of the one before is already released.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_link VALIDATE CONSTRAINT activity_link_entity_type_check;
ALTER TABLE activity_link VALIDATE CONSTRAINT activity_link_shape;
