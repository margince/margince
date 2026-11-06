-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

DROP INDEX idx_graph_contact_edge_work;
DROP INDEX idx_graph_contact_edge_deferred;
ALTER TABLE graph_contact_edge DROP CONSTRAINT graph_contact_edge_work_generations,
 DROP COLUMN requested_generation, DROP COLUMN processed_generation, DROP COLUMN deferred_run_id;

DROP INDEX idx_graph_interaction_edge_work;
DROP INDEX idx_graph_interaction_edge_deferred;
ALTER TABLE graph_interaction_edge DROP CONSTRAINT graph_interaction_edge_work_generations,
 DROP COLUMN requested_generation, DROP COLUMN processed_generation, DROP COLUMN deferred_run_id;
