-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validate the checks the migration before this one added NOT VALID. Its own
-- file, so each scan runs under SHARE UPDATE EXCLUSIVE. None can fail: every
-- existing row holds the defaults or the looser rule.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal VALIDATE CONSTRAINT deal_commercial_mode_check;
ALTER TABLE deal VALIDATE CONSTRAINT deal_commercial_revision_js_safe;
ALTER TABLE deal_product VALIDATE CONSTRAINT deal_product_source_check;
ALTER TABLE deal_product VALIDATE CONSTRAINT deal_product_amount_currency_pair;
ALTER TABLE deal_product VALIDATE CONSTRAINT deal_product_known_amount_minor_check;
ALTER TABLE deal_product VALIDATE CONSTRAINT deal_product_known_amount_minor_exact;
ALTER TABLE deal_product VALIDATE CONSTRAINT deal_product_commercial_revision_js_safe;
ALTER TABLE deal_product VALIDATE CONSTRAINT deal_product_line_shape;
ALTER TABLE offer VALIDATE CONSTRAINT offer_source_commercial_revision_js_safe;
