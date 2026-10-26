-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The last value each source offered for a company field, kept beside the
-- value a user typed. A company that is deleted outright takes its offered
-- values with it. An archived company keeps them for a restore and does not
-- serve them. Erasing or anonymizing a contact deletes the rows of the
-- companies they were employed by.
SET LOCAL lock_timeout = '3s';

CREATE TABLE company_offered_value (
    company_id uuid NOT NULL REFERENCES company(id) ON DELETE CASCADE,
    source_system text NOT NULL,
    field_key text NOT NULL,
    offered_value text NOT NULL,
    offered_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (company_id, source_system, field_key),
    CONSTRAINT company_offered_value_field_known CHECK (field_key IN ('legal_name', 'industry', 'description'))
);
