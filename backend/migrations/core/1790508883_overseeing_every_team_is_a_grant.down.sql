-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Nothing to undo, and nothing that COULD be undone honestly.
--
-- The up migration ADDS objects.team_oversight where a role has none. The row
-- keeps no record of which documents it wrote, so a rollback cannot tell a key
-- this migration added from one an operator has since set by hand, and
-- removing it wholesale would erase a decision the operator made — a denial on
-- management included, which the next up would then turn back into a grant.
--
-- Leaving the key is safe for the previous code: policy.Parse drops an object
-- outside the grantable vocabulary with a log line rather than refusing the
-- document, so a login under the older build reads the role as it always did.
SELECT 1;
