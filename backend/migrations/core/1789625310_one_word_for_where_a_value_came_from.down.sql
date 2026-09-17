-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

ALTER TABLE company
    DROP CONSTRAINT IF EXISTS company_logo_pair_is_whole,
    DROP CONSTRAINT IF EXISTS company_logo_icon_pair_is_whole;

ALTER TABLE company RENAME COLUMN logo_source TO logo_origin;
ALTER TABLE company RENAME COLUMN logo_icon_source TO logo_icon_origin;
ALTER TABLE company RENAME COLUMN geocode_source TO geocode_provider;
