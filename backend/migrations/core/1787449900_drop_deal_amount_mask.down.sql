-- Put the rep deal-amount mask back, exactly as the baseline seeded it.
--
-- This migration and 1787449829 belong to ONE ruling and are rolled back
-- together or not at all. Reverting this one alone restores the mask onto
-- own-scoped reps, which hides the amount on every deal a rep does not
-- personally own — a blackout neither the old model nor the new one has, and
-- the exact state the up migration exists to prevent.
INSERT INTO field_mask (role_key, object, field, condition)
VALUES ('rep', 'deal', 'amount_minor', 'outside_write_authority')
ON CONFLICT DO NOTHING;
