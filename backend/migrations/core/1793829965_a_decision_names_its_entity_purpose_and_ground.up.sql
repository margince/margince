-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- What each send decision was taken for, so transmit asks about the same
-- message staging judged: the legal entity it goes out for, the one purpose
-- key it was judged under, and the sale and declaration it rests on when it
-- went out on the existing-customer route. A row written before these columns
-- names none; transmit then lets nothing scoped to an entity allow it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_decision
    ADD COLUMN legal_entity_id uuid,
    ADD COLUMN purpose_key text,
    ADD COLUMN existing_customer_notice_id uuid,
    ADD COLUMN existing_customer_offer_id uuid,
    ADD COLUMN existing_customer_sold_at timestamptz,
    ADD CONSTRAINT communication_decision_existing_customer_shape CHECK (
        (existing_customer_notice_id IS NULL) = (existing_customer_offer_id IS NULL)
        AND (existing_customer_notice_id IS NULL) = (existing_customer_sold_at IS NULL)) NOT VALID;
