-- A colleague decides whether their reach at a company is shown to the team.
--
-- The count is a derived fact — how many of their LinkedIn connections work at
-- one account — and not access to their account, which no seat reads. It is
-- still a fact about them, so they may withhold it.
--
-- Visible by default: the product's answer to "who can get us in" is worth
-- nothing if it is empty until everybody opts in, and the count discloses no
-- name. A member who would rather not be asked switches it off, and is then
-- left out of everyone else's view while their own reach page is unchanged.
--
-- NOT NULL with a default, so a member who has never opened the setting is
-- visible, and a row written before this column existed reads the same way.
SET LOCAL lock_timeout = '3s';
ALTER TABLE linkedin_account
  ADD COLUMN reach_visible_to_team boolean NOT NULL DEFAULT true;
