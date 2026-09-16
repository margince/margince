-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded, because ALTER TABLE takes ACCESS EXCLUSIVE on a table this
-- migration did not create: a writer holding a row lock would otherwise queue
-- every reader behind this statement for as long as it cared to wait. Two
-- seconds is the tree's own number for an additive column.
SET LOCAL lock_timeout = '2s';
-- Dropping the columns loses every recorded answer, which is the point of the
-- additive rule: this direction exists for a rollback that has not yet run in
-- production, not as a way back from one that has.
ALTER TABLE ai_call
    DROP CONSTRAINT IF EXISTS ai_call_secrets_removed_check,
    DROP COLUMN IF EXISTS secret_kinds,
    DROP COLUMN IF EXISTS secrets_removed;
