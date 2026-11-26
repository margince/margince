-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- How to read an edge's last_at: 'day' when the newest admitted activity of
-- the pair names only a date, else 'instant'. The fold writes it from the same
-- rows and tie rule as last_at. NULL on a row folded before this column, which
-- reads as an instant until the next fold or the nightly rebuild fills it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE graph_interaction_edge ADD COLUMN IF NOT EXISTS last_precision text;
ALTER TABLE graph_contact_edge ADD COLUMN IF NOT EXISTS last_precision text;

ALTER TABLE graph_interaction_edge DROP CONSTRAINT IF EXISTS graph_interaction_edge_last_precision_check;
ALTER TABLE graph_interaction_edge ADD CONSTRAINT graph_interaction_edge_last_precision_check
    CHECK (last_precision IN ('day', 'instant')) NOT VALID;
ALTER TABLE graph_contact_edge DROP CONSTRAINT IF EXISTS graph_contact_edge_last_precision_check;
ALTER TABLE graph_contact_edge ADD CONSTRAINT graph_contact_edge_last_precision_check
    CHECK (last_precision IN ('day', 'instant')) NOT VALID;
