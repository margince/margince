-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DELETE FROM generated_summary WHERE kind = 'task_explanation';

DROP INDEX generated_summary_activity_ix;

ALTER TABLE generated_summary
    DROP CONSTRAINT generated_summary_reader_kind_record_key,
    ADD CONSTRAINT generated_summary_reader_kind_record_key
        UNIQUE NULLS NOT DISTINCT (user_id, kind, company_id, contact_id, deal_id);

ALTER TABLE generated_summary
    DROP CONSTRAINT generated_summary_kind_names_its_record,
    ADD CONSTRAINT generated_summary_kind_names_its_record CHECK (
        num_nonnulls(company_id, contact_id, deal_id) = 1
        AND (contact_id IS NOT NULL) = (kind = 'contact_brief')
        AND (deal_id IS NOT NULL) = (kind = 'deal_status_card'));

ALTER TABLE generated_summary
    DROP CONSTRAINT generated_summary_kind_check,
    ADD CONSTRAINT generated_summary_kind_check CHECK (kind IN (
        'company_brief', 'company_dossier', 'company_growth_fit', 'contact_brief', 'deal_status_card'));

ALTER TABLE generated_summary DROP COLUMN activity_id;
