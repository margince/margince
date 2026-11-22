-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A retired custom field keeps its column and its values but no longer holds its
-- slug, so a new field of an object can take the name a retired one had. Retiring
-- is a status flip and archived_at stays null, so the status is the predicate.
-- uq_custom_field_column stays unconditional: the retired field still owns its
-- physical column.
DROP INDEX IF EXISTS uq_custom_field_slug;
CREATE UNIQUE INDEX IF NOT EXISTS uq_custom_field_slug
    ON custom_field (object, slug) WHERE status <> 'retired';
