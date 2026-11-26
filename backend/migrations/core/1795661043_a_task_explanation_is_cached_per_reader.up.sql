-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A task's explanation is cached per reader like the record briefs, keyed on
-- the task itself. A deleted task takes its explanations with it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE generated_summary ADD COLUMN activity_id uuid;

ALTER TABLE generated_summary
    ADD CONSTRAINT generated_summary_activity_fkey
        FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE;

ALTER TABLE generated_summary
    DROP CONSTRAINT generated_summary_kind_check,
    ADD CONSTRAINT generated_summary_kind_check CHECK (kind IN (
        'company_brief', 'company_dossier', 'company_growth_fit', 'contact_brief', 'deal_status_card',
        'task_explanation'));

ALTER TABLE generated_summary
    DROP CONSTRAINT generated_summary_kind_names_its_record,
    ADD CONSTRAINT generated_summary_kind_names_its_record CHECK (
        num_nonnulls(company_id, contact_id, deal_id, activity_id) = 1
        AND (contact_id IS NOT NULL) = (kind = 'contact_brief')
        AND (deal_id IS NOT NULL) = (kind = 'deal_status_card')
        AND (activity_id IS NOT NULL) = (kind = 'task_explanation'));

ALTER TABLE generated_summary
    DROP CONSTRAINT generated_summary_reader_kind_record_key,
    ADD CONSTRAINT generated_summary_reader_kind_record_key
        UNIQUE NULLS NOT DISTINCT (user_id, kind, company_id, contact_id, deal_id, activity_id);

CREATE INDEX generated_summary_activity_ix ON generated_summary (activity_id) WHERE activity_id IS NOT NULL;
