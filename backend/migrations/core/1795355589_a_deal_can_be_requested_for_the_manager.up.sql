-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A deal can be requested for the team's manager. The seller accepts "bring in
-- the manager", and the deal then carries when it was asked and by whom. NULL
-- is a deal nobody asked for, which is every row today. The reasons are codes the
-- client words: big_deal, single_contact, stuck. The request clears when
-- the manager marks it handled or the deal leaves the open pipeline.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal
    ADD COLUMN IF NOT EXISTS manager_requested_at timestamptz,
    ADD COLUMN IF NOT EXISTS manager_requested_by uuid,
    ADD COLUMN IF NOT EXISTS manager_request_reasons text[];
