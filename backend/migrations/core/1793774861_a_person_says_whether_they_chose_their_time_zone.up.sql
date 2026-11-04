-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Whether a person chose their own time zone, or it is only the browser's
-- guess. A guess may be replaced by the next browser guess; a choice is kept.
-- People who saved the working-hours form picked a zone there, the only place
-- one could be picked until now, so they count as having chosen. The first
-- admin's zone came from the setup browser and stays a guess.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user
    ADD COLUMN timezone_chosen boolean NOT NULL DEFAULT false;

UPDATE app_user SET timezone_chosen = true WHERE work_start_minute IS NOT NULL;
