-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

ALTER TABLE graph_contact_edge
 ADD COLUMN requested_generation bigint NOT NULL DEFAULT 0,
 ADD COLUMN processed_generation bigint NOT NULL DEFAULT 0,
 ADD COLUMN deferred_run_id uuid,
 ADD CONSTRAINT graph_contact_edge_work_generations CHECK (processed_generation >= 0 AND requested_generation >= processed_generation) NOT VALID,
 ADD CONSTRAINT graph_contact_edge_deferred_run_id_fkey FOREIGN KEY (deferred_run_id) REFERENCES capture_backfill(id) ON DELETE SET NULL NOT VALID;
CREATE INDEX idx_graph_contact_edge_work ON graph_contact_edge (contact_a, contact_b)
 WHERE requested_generation > processed_generation AND deferred_run_id IS NULL;
CREATE INDEX idx_graph_contact_edge_deferred ON graph_contact_edge (deferred_run_id) WHERE deferred_run_id IS NOT NULL;

ALTER TABLE graph_interaction_edge
 ADD COLUMN requested_generation bigint NOT NULL DEFAULT 0,
 ADD COLUMN processed_generation bigint NOT NULL DEFAULT 0,
 ADD COLUMN deferred_run_id uuid,
 ADD CONSTRAINT graph_interaction_edge_work_generations CHECK (processed_generation >= 0 AND requested_generation >= processed_generation) NOT VALID,
 ADD CONSTRAINT graph_interaction_edge_deferred_run_id_fkey FOREIGN KEY (deferred_run_id) REFERENCES capture_backfill(id) ON DELETE SET NULL NOT VALID;
CREATE INDEX idx_graph_interaction_edge_work ON graph_interaction_edge (user_id, contact_id)
 WHERE requested_generation > processed_generation AND deferred_run_id IS NULL;
CREATE INDEX idx_graph_interaction_edge_deferred ON graph_interaction_edge (deferred_run_id) WHERE deferred_run_id IS NOT NULL;
