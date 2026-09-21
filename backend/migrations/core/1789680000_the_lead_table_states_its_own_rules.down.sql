SET LOCAL lock_timeout = '3s';

ALTER TABLE lead
    DROP CONSTRAINT IF EXISTS lead_promoted_names_its_contact,
    DROP CONSTRAINT IF EXISTS lead_score_range,
    DROP CONSTRAINT IF EXISTS lead_score_computed_range,
    DROP CONSTRAINT IF EXISTS lead_override_retains_the_computed_score,
    DROP CONSTRAINT IF EXISTS lead_first_response_follows_creation,
    DROP CONSTRAINT IF EXISTS lead_sla_breach_follows_creation;

ALTER TABLE lead DROP CONSTRAINT lead_status_set_by_check;

ALTER TABLE lead
    ADD CONSTRAINT lead_status_set_by_check
        CHECK (status_set_by IN ('human', 'system'));
