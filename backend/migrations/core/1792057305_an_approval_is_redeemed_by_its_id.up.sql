-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An agent redeems an approval by its id, which the server checks against the
-- stored row. The signed token the approve call minted was accepted by no door,
-- so the key it was signed with goes with it.
SET LOCAL lock_timeout = '3s';

DROP TABLE signing_key;

COMMENT ON COLUMN approval.diff_hash IS
	'Hash of the staged proposed_change. A redemption whose call hashes differently is refused, '
	'so an approval cannot authorize a different effect.';
