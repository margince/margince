-- A receipt says the system decided it, rather than leaving it to be inferred
-- from an empty column.
--
-- The "done for you" lane means: this ran, and nobody was asked. It selected
-- rows with status 'approved' and decided_by IS NULL, reading the absent decider
-- as "no human decided". Two things are wrong with inferring it.
--
-- First, no writer produces that pair. Staging inserts 'pending'; a human
-- decision writes decided_by; the expiry sweep leaves decided_by NULL but
-- writes 'expired', not 'approved'. The lane's own test matches nothing the
-- product writes, so it has been reporting an empty night on every
-- installation.
--
-- Second, and worse, the column can BECOME null under a row nobody touched:
-- approval_decided_by_fkey is ON DELETE SET NULL, so deleting an app_user
-- empties decided_by on every approval that person ever decided. Their
-- decisions would then read as things the system did on its own — the product
-- telling a team "nobody was asked" about work a departed colleague was in fact
-- asked about, on the one surface whose whole job is to report honestly what
-- happened.
--
-- decided_by_system is written by the decider and never inferred, so a deleted
-- user changes who is named and not what happened.
-- Deliberately a column and NO check constraint.
--
-- The obvious constraint — "a decided approval names the system or a person" —
-- would be enforced on UPDATE, and the ON DELETE SET NULL above IS an update.
-- Adding it would make deleting an app_user fail against their own decision
-- history, turning an honesty fix into a blocked erasure path. Whether a
-- decided row still names its decider is a question about a person who may have
-- been erased on purpose; whether the SYSTEM decided it is a fact about the
-- decision, and that is what this column holds.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval
    ADD COLUMN IF NOT EXISTS decided_by_system boolean NOT NULL DEFAULT false;
