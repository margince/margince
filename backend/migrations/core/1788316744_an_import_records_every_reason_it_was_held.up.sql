-- A held import records every rule that matched it, not only the first.
--
-- decideBirthTx runs five rules strictest-first and returns on the first hold,
-- which is right for DECIDING — the strictest answer is the answer. It is wrong
-- for REMEMBERING. A [Vertraulich] message from a counterparty this seat holds
-- matches rule 2 and never reaches rule 3, so the row records 'counterparty'
-- and nothing anywhere records that the confidential marker also applied.
--
-- That gap is what stops a hold ever being lifted. "Re-open everything my
-- counterparty hold caught" cannot be answered from a single reason: the rows
-- reading 'counterparty' include ones a marker or the workspace floor would
-- hold anyway, and widening those publishes a message the sender explicitly
-- asked us not to share.
--
-- So the column becomes a list. Nothing about which audience a message is BORN
-- with changes — decideBirthTx still returns the strictest rule's answer — only
-- what is written down about why.

-- Bounded: adding a column takes an ACCESS EXCLUSIVE lock on capture_import,
-- which every mail sync writes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_import ADD COLUMN verdict_reasons text[];

COMMENT ON COLUMN capture_import.verdict_reasons IS
	'Every birth rule that matched this import, strictest first. verdict_reason '
	'stays as the first of them — the one that decided the audience — because '
	'the recompute and the API both read a single reason and neither is asking '
	'this question. NULL on a row written before this column existed, which is '
	'not the same as a one-element list: a backfilled row is not PROVABLY '
	'counterparty-only, so the widening path refuses it.';

-- Backfilled to NULL, deliberately, and not to ARRAY[verdict_reason].
--
-- A pre-existing row records the first rule that matched and says nothing about
-- the rest. Writing ARRAY['counterparty'] would assert that no other rule
-- applied, which is exactly the claim the old code could not make — and the
-- widening path would then believe it and publish a marked message. NULL says
-- "unknown", the widening path refuses it, and the cost is that mail captured
-- before this migration cannot be re-opened in bulk. That is the safe direction
-- of a wrong guess.
