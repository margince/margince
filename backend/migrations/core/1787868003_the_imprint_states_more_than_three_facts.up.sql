-- The legal-notice page states more identity than the profile could hold.
--
-- §5 DDG obliges a German site to print the legal form, the register court and
-- register number, and the VAT ID. The profile carried three fields for the
-- whole block: legal_name, registered_address, and register_vat.
--
-- Three new fields are added: legal_form, register_court, register_number.
--
-- register_vat is NOT split and NOT retired. Its contract description admits
-- both a VAT ID and a register entry ("DE123456789, HRB 12345 B"), so rows
-- already written under it hold one or the other with nothing recording which.
-- A migration cannot tell them apart without parsing a value a human entered,
-- and guessing would corrupt the field it claims to clean. So it keeps its
-- meaning, register_number takes the register entry going forward, and the
-- ambiguity ends at the read rather than being rewritten backwards.

-- Swapping the CHECK takes a lock that blocks writers, so the wait is bounded:
-- rather than queueing behind a long-open transaction and stalling every write
-- to the table, this fails and is retried.
SET LOCAL lock_timeout = '3s';

ALTER TABLE organization_profile_field
    DROP CONSTRAINT organization_profile_field_field_check;

ALTER TABLE organization_profile_field
    ADD CONSTRAINT organization_profile_field_field_check CHECK (field IN (
        'display_name', 'offer_summary', 'icp', 'value_proposition', 'usp',
        'customer_pains', 'desired_outcomes', 'buying_center', 'buying_intents',
        'common_objections', 'sales_motion', 'legal_name', 'registered_address',
        'register_vat', 'industry', 'history',
        -- The rest of what §5 DDG makes an imprint state.
        'legal_form', 'register_court', 'register_number'
    ));
