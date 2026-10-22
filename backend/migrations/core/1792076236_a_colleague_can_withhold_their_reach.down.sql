-- Dropping the column makes every member visible again, including the ones who
-- had switched it off. That is the honest reverse and the reason the forward
-- direction is the safe one to run: nothing else records the choice.
SET LOCAL lock_timeout = '3s';
ALTER TABLE linkedin_account DROP COLUMN reach_visible_to_team;
