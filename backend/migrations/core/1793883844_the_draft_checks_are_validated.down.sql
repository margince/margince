-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put the checks back present and unvalidated, as this migration found them.
-- Postgres cannot un-validate, so it is dropped and added again NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE mail_draft
    DROP CONSTRAINT mail_draft_schedule_pair,
    ADD CONSTRAINT mail_draft_schedule_pair
        CHECK ((scheduled_at IS NULL) = (scheduled_tz IS NULL)) NOT VALID,
    DROP CONSTRAINT mail_draft_communication_context,
    ADD CONSTRAINT mail_draft_communication_context
        CHECK (communication_context IS NULL OR communication_context IN (
            'reply_to_inbound', 'requested_followup', 'precontract_quote', 'active_deal_followup',
            'customer_service', 'account_notice', 'contract_notice', 'invoice_or_payment',
            'marketing')) NOT VALID;
