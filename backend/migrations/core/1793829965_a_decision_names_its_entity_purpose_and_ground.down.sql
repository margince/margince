-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_decision
    DROP CONSTRAINT communication_decision_existing_customer_shape,
    DROP COLUMN existing_customer_sold_at,
    DROP COLUMN existing_customer_offer_id,
    DROP COLUMN existing_customer_notice_id,
    DROP COLUMN purpose_key,
    DROP COLUMN legal_entity_id;
