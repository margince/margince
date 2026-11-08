-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

ALTER TABLE audit_log ADD COLUMN source_xid xid8;
ALTER TABLE audit_log ALTER COLUMN source_xid SET DEFAULT pg_current_xact_id();
ALTER TABLE graph_contact_edge ADD COLUMN read_snapshot pg_snapshot;
ALTER TABLE graph_interaction_edge ADD COLUMN read_snapshot pg_snapshot;
ALTER TABLE contact_signature_enrich_state ADD COLUMN read_snapshot pg_snapshot;
