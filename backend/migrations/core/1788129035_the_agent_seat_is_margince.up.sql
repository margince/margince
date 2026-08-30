-- The seeded agent identity carries the product's name, and the product is
-- Margince. Every installation seeded before this shows "Gradion Agent" in its
-- user list, next to a page that says Margince everywhere else.
--
-- Scoped to the SEEDED row: an agent seat whose name an operator has already
-- changed is their text, not ours, and renaming it would be editing somebody's
-- data to win an argument about vocabulary. `is_agent` alone would be too wide
-- for the same reason.
--
-- The seat's email is deliberately untouched. It is a uniqueness key on a row
-- that holds no password and receives no mail, the screen no longer prints it,
-- and rewriting an address to change a word nobody reads is churn with a
-- collision risk attached.
SET LOCAL lock_timeout = '3s';
UPDATE app_user SET display_name = 'Margince Agent', updated_at = now()
 WHERE is_agent AND display_name = 'Gradion Agent';
