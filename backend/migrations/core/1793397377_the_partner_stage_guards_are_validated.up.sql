-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the guards the previous file added. A file of its own
-- because VALIDATE takes a lighter lock than the ADD, and only a separate
-- transaction lets writers through while it scans. None can fail: every stage
-- a partner held is seeded, every role is a kind, and every stamped row carries
-- the one basis that needs no date.
SET LOCAL lock_timeout = '3s';

ALTER TABLE partner VALIDATE CONSTRAINT partner_relationship_stage_fkey;
ALTER TABLE partner VALIDATE CONSTRAINT partner_partner_kinds_check;
ALTER TABLE partner VALIDATE CONSTRAINT partner_activation_basis_check;
ALTER TABLE partner VALIDATE CONSTRAINT partner_activation_pairing;
