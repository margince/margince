-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A saved draft keeps the files the rep chose, the reason for contact they
-- claimed and the moment they picked, so a reopened composer has all of it.
--
-- The files are ids and not a foreign key: a draft must never hold back the
-- archive or erasure of a document, so each read resolves them and leaves out
-- one that has gone or fallen out of the reader's sight.
--
-- A moment is never kept without the zone it was picked in, and a reason is
-- one a send may claim. Both checks are added NOT VALID and validated by the
-- next migration, under a lighter lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE mail_draft
    ADD COLUMN attachment_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    ADD COLUMN communication_context text,
    ADD COLUMN scheduled_at timestamptz,
    ADD COLUMN scheduled_tz text,
    ADD CONSTRAINT mail_draft_schedule_pair
        CHECK ((scheduled_at IS NULL) = (scheduled_tz IS NULL)) NOT VALID,
    ADD CONSTRAINT mail_draft_communication_context
        CHECK (communication_context IS NULL OR communication_context IN (
            'reply_to_inbound', 'requested_followup', 'precontract_quote', 'active_deal_followup',
            'customer_service', 'account_notice', 'contract_notice', 'invoice_or_payment',
            'marketing')) NOT VALID;
