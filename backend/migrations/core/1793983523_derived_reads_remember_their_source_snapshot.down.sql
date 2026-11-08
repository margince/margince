-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

ALTER TABLE contact_signature_enrich_state DROP COLUMN read_snapshot;
ALTER TABLE graph_interaction_edge DROP COLUMN read_snapshot;
ALTER TABLE graph_contact_edge DROP COLUMN read_snapshot;
ALTER TABLE audit_log DROP COLUMN source_xid;
