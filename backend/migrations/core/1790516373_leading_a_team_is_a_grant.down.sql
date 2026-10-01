-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Nothing to undo, and nothing that COULD be undone honestly.
--
-- The up migration ADDS objects.team_lead where a system role has none. The row
-- keeps no record of which documents it wrote, so a rollback cannot tell a key
-- this migration added from one an operator has since set by hand, and
-- removing it wholesale would erase a decision the operator made — a denial on
-- manager included, which the next up would then turn back into a grant.
--
-- Leaving the key is safe for the previous code: it decided leading by role
-- key and never read this object, and policy.Parse drops an object outside the
-- grantable vocabulary with a log line rather than refusing the document.
SELECT 1;
