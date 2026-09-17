-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A company has a status, like every other record that moves through a funnel.
--
-- This schema already has a record lifecycle — archived_at, legal_hold,
-- merged_into_id — so a bare `lifecycle` reads as part of THAT, when it means
-- where the account sits commercially. deal, lead, contract, offer and
-- intro_request all spell the same idea `status`, and company.lifecycle has
-- lead.status's exact shape: a fixed CHECK list naming a position in a funnel,
-- sharing even the `disqualified` value.
--
-- Not `lifecycle_stage` or `relationship_stage`: `stage` is a first-class
-- concept here — a configurable stage table with exit criteria and progression
-- policy, reached by deal.stage_id. This column is none of those. deal carries
-- both and shows the distinction: status is the coarse fixed outcome, stage_id
-- the configurable position.
--
-- geocode_status is on this table too and does not collide; it is prefixed and
-- says something about a coordinate pair, not an account.
ALTER TABLE company RENAME COLUMN lifecycle TO status;
ALTER TABLE company RENAME CONSTRAINT company_lifecycle_check TO company_status_check;
ALTER INDEX idx_company_lifecycle RENAME TO idx_company_status;

-- Rows that STORE the word move with it.
--
-- A saved view holds its sort and filter keys as jsonb, so a view sorting on
-- lifecycle would silently stop sorting. field_provenance names the column each
-- authorship claim is about, so a claim on lifecycle would stop being found and
-- an agent would be free to overwrite a value a human authored.
UPDATE saved_view
   SET query = replace(query::text, '"lifecycle"', '"status"')::jsonb
 WHERE resource = 'companies'
   AND query::text LIKE '%"lifecycle"%';

UPDATE field_provenance
   SET field_name = 'status'
 WHERE field_name = 'lifecycle' AND object_type = 'company';

-- audit_log is NOT touched, because it cannot be: trg_audit_no_mutate refuses
-- an UPDATE on it, which is the property that makes the trail worth having.
-- Entries written before this migration name `lifecycle` forever, so the
-- history labels on the frontend read both spellings and say why.
