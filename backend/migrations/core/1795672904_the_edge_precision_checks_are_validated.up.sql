-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the CHECKs the previous migration added NOT VALID. It is a file of
-- its own so the scan takes only SHARE UPDATE EXCLUSIVE and writers go through.
SET LOCAL lock_timeout = '3s';

ALTER TABLE graph_interaction_edge VALIDATE CONSTRAINT graph_interaction_edge_last_precision_check;
ALTER TABLE graph_contact_edge VALIDATE CONSTRAINT graph_contact_edge_last_precision_check;
