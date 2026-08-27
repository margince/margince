-- Narrowing the vocabulary again would orphan any row written under one of the
-- three new names, and a CHECK cannot be re-added while such a row exists.
--
-- There is no lossless way down: a register court read off an imprint has no
-- earlier spelling to be restored to, so removing the name means deleting the
-- fact. This migration therefore REFUSES rather than deleting. What it would
-- destroy is legal identity a person typed or confirmed off a company's own
-- legal notice, and a rollback is not a mandate to throw that away silently.
--
-- To go down deliberately, empty the three fields first and then re-run:
--
--   DELETE FROM organization_profile_field
--    WHERE field IN ('legal_form', 'register_court', 'register_number');
--
-- Doing it by hand is the point: the operator states the loss instead of
-- discovering it afterwards.

-- Same bound as the up migration, and for the same reason.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE
    held bigint;
BEGIN
    SELECT count(*) INTO held
      FROM organization_profile_field
     WHERE field IN ('legal_form', 'register_court', 'register_number');

    IF held > 0 THEN
        RAISE EXCEPTION
            'refusing to roll back: % organization profile row(s) state a legal form, register court or register number',
            held
        USING HINT =
            'delete those rows first if the loss is intended — this migration will not discard legal identity on its own';
    END IF;
END
$$;

ALTER TABLE organization_profile_field
    DROP CONSTRAINT organization_profile_field_field_check;

ALTER TABLE organization_profile_field
    ADD CONSTRAINT organization_profile_field_field_check CHECK (field IN (
        'display_name', 'offer_summary', 'icp', 'value_proposition', 'usp',
        'customer_pains', 'desired_outcomes', 'buying_center', 'buying_intents',
        'common_objections', 'sales_motion', 'legal_name', 'registered_address',
        'register_vat', 'industry', 'history'
    ));
