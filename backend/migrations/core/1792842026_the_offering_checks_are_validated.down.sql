-- Put the constraints back the way the file beside this one found them:
-- present, unvalidated. Postgres has no ALTER that un-validates, so each is
-- dropped and re-added NOT VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE legal_entity
    DROP CONSTRAINT legal_entity_default_has_no_offering,
    DROP CONSTRAINT legal_entity_offering_bounded,
    ADD CONSTRAINT legal_entity_default_has_no_offering CHECK (NOT is_default OR (
        offer_summary IS NULL AND icp IS NULL AND value_proposition IS NULL AND usp IS NULL)) NOT VALID,
    ADD CONSTRAINT legal_entity_offering_bounded CHECK (
        length(offer_summary) <= 4000 AND length(icp) <= 4000
        AND length(value_proposition) <= 4000 AND length(usp) <= 4000) NOT VALID;

ALTER TABLE company
    DROP CONSTRAINT company_assess_against_entity_id_fkey,
    ADD CONSTRAINT company_assess_against_entity_id_fkey
        FOREIGN KEY (assess_against_entity_id) REFERENCES legal_entity(id) ON DELETE RESTRICT NOT VALID;
