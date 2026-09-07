-- A record that says 'owner' and names nobody must not exist.
--
-- The row-scope arm reads the pair as `visibility <> 'owner' OR owner_id = me`,
-- so such a row satisfies neither side and is readable by NO seat: its author,
-- an admin, and an unbounded row_scope=all principal alike. It is a record
-- destroyed rather than a record made private, and no patch through the
-- endpoint that produced it can reach it to repair it.
--
-- people/personvisibility.go refuses the one-step case — a patch setting
-- visibility to 'owner' while clearing or omitting the owner. It cannot refuse
-- the two-step one, because it is a rule about a PATCH and this is a property
-- of the ROW: narrow with an owner named, clear the owner in a second patch,
-- and each write is individually admissible while the pair they leave is not.
-- A CHECK cannot be got around by splitting a write in two, which is the whole
-- difference between a rule about a request and a rule about a record.
--
-- `signal` has carried this constraint since it gained the columns; person and
-- organization are the two that did not.
--
-- project is deliberately NOT here: project_visibility_check admits only
-- 'workspace', which is strictly stronger than this pairing and makes it dead
-- weight. The day that widens, this rule has to come with it —
-- TestEveryOwnerPrivateTableNamesItsOwner is what says so rather than a comment
-- nobody reads.
--
-- THE BACKFILL PUBLISHES. Any row already in this state is readable by nobody,
-- so 'workspace' is not a disclosure of something currently private — it is the
-- recovery of something currently invisible, and the only value that makes the
-- row reachable at all. Leaving it would keep the record lost forever. No row
-- is expected: the state is only creatable through the gap this closes.
-- Bounded: adding a validated CHECK takes a lock that blocks writers on tables
-- this migration did not create, and an open transaction holding a conflicting
-- lock would otherwise stall every write to them indefinitely.
SET LOCAL lock_timeout = '3s';

UPDATE person       SET visibility = 'workspace' WHERE visibility = 'owner' AND owner_id IS NULL;
UPDATE organization SET visibility = 'workspace' WHERE visibility = 'owner' AND owner_id IS NULL;

ALTER TABLE person
    ADD CONSTRAINT person_owner_private_names_its_owner
    CHECK (visibility <> 'owner' OR owner_id IS NOT NULL);

ALTER TABLE organization
    ADD CONSTRAINT organization_owner_private_names_its_owner
    CHECK (visibility <> 'owner' OR owner_id IS NOT NULL);
