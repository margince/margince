-- Narrowing the vocabulary again would orphan any row written under one of the
-- three new names, and a CHECK cannot be re-added while such a row exists. So
-- the rows go first, and that is a real deletion rather than a reversal: a
-- register court read off an imprint is gone, not restored to some earlier
-- spelling, because there was no earlier spelling to hold it.

-- Same bound as the up migration, and for the same reason.
SET LOCAL lock_timeout = '3s';

DELETE FROM organization_profile_field
 WHERE field IN ('legal_form', 'register_court', 'register_number');

ALTER TABLE organization_profile_field
    DROP CONSTRAINT organization_profile_field_field_check;

ALTER TABLE organization_profile_field
    ADD CONSTRAINT organization_profile_field_field_check CHECK (field IN (
        'display_name', 'offer_summary', 'icp', 'value_proposition', 'usp',
        'customer_pains', 'desired_outcomes', 'buying_center', 'buying_intents',
        'common_objections', 'sales_motion', 'legal_name', 'registered_address',
        'register_vat', 'industry', 'history'
    ));
