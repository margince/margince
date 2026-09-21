-- The undo window a move was made under, frozen onto the move.
--
-- Read live from stage_progression_policy, the window is whatever the rule
-- says NOW: an admin shortening it from 72 hours to 1 retroactively closes the
-- window on every move already made, and lengthening it reopens moves people
-- were told they could no longer take back. Neither is a change anybody asked
-- for, and the second is worse — a deal moves back days after its window was
-- described as closed.
--
-- The window is a promise made to the person the move was made for, so it is
-- recorded when the promise is made. A policy edit then governs the next move
-- and not the last one.
-- Bounded, like every migration that takes a table lock here: an ADD COLUMN
-- waits behind whatever is holding the table, and an unbounded wait blocks
-- every writer behind it rather than failing the deploy.
SET LOCAL lock_timeout = '3s';

ALTER TABLE stage_progression_outcome
  ADD COLUMN undo_window_hours integer;

COMMENT ON COLUMN stage_progression_outcome.undo_window_hours IS
  'The undo window in force when this move was applied, frozen so a later policy edit cannot extend or close it. NULL on rows written before this column existed, which read the rule live as they always did.';
