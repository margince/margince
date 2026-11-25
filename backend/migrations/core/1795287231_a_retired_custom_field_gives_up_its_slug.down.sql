-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- Fails while an object holds a retired and a live field with one slug; retire
-- or rename one of them first.
DROP INDEX IF EXISTS uq_custom_field_slug;
CREATE UNIQUE INDEX IF NOT EXISTS uq_custom_field_slug ON custom_field (object, slug);
