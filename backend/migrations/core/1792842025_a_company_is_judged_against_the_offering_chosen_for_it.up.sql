-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A further legal entity describes what it sells, and a company can be judged
-- against that offering instead of the main one.
--
-- The default entity keeps its offering on the anchor company's profile, so it
-- stores none of these columns. The checks and the key are added NOT VALID and
-- validated by the file beside this one. A company with no choice stays NULL and is
-- judged against the main offering, as before.
SET LOCAL lock_timeout = '3s';

ALTER TABLE legal_entity
    ADD COLUMN offer_summary text,
    ADD COLUMN icp text,
    ADD COLUMN value_proposition text,
    ADD COLUMN usp text,
    ADD CONSTRAINT legal_entity_default_has_no_offering CHECK (NOT is_default OR (
        offer_summary IS NULL AND icp IS NULL AND value_proposition IS NULL AND usp IS NULL)) NOT VALID,
    ADD CONSTRAINT legal_entity_offering_bounded CHECK (
        length(offer_summary) <= 4000 AND length(icp) <= 4000
        AND length(value_proposition) <= 4000 AND length(usp) <= 4000) NOT VALID;

ALTER TABLE company
    ADD COLUMN assess_against_entity_id uuid,
    ADD CONSTRAINT company_assess_against_entity_id_fkey
        FOREIGN KEY (assess_against_entity_id) REFERENCES legal_entity(id) ON DELETE RESTRICT NOT VALID;

CREATE INDEX idx_company_assess_against_entity ON company (assess_against_entity_id)
    WHERE assess_against_entity_id IS NOT NULL;
