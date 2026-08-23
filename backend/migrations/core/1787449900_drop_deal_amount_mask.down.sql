-- Put the rep deal-amount mask back, exactly as the baseline seeded it.
INSERT INTO field_mask (role_key, object, field, condition)
VALUES ('rep', 'deal', 'amount_minor', 'outside_write_authority')
ON CONFLICT DO NOTHING;
