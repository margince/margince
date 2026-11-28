-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- own_seat is the seat a run is kept to when its importer imports only their
-- own records; NULL is a workspace import.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run ADD COLUMN IF NOT EXISTS own_seat uuid;
