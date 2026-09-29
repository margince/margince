-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The partner's margin tier is a field an administrator can now mask.
--
-- contacts/partnerfieldmask.go already withholds it on every partner read, but
-- the foreign key on field_mask refuses a pair this table does not offer, so
-- until this row exists the withholding is unreachable: a mask nobody can
-- configure enforces exactly as much as no mask at all.
--
-- The commission entry's margin_tier_at_accrual is deliberately NOT here. It is
-- withheld as a consequence of this pair, through the group closure in
-- platform/auth, and is never named by an administrator. Two configurations for
-- one fact is how the leak returns: an operator sets one, believes the tier is
-- hidden, and the other republishes it.
INSERT INTO maskable_field (object, field) VALUES ('partner', 'margin_tier');
