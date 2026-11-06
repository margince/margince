-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The country whose data-protection rules a send decision was taken under: the
-- country of the legal entity the message went out for. ruleset_codes already
-- names the marketing rules, which follow the recipient. NULL on a row written
-- before this column, or one that names no entity.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_decision ADD COLUMN data_protection_country text;
