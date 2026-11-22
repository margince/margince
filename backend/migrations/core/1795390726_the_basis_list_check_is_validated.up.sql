-- Validate the two basis checks the migration before this one added NOT VALID.
-- Its own file, so a scan runs under the lighter lock and not under the
-- ACCESS EXCLUSIVE that the ALTER held to its commit.
--
-- PostgreSQL checks a NOT VALID rule again on every update of a row, so a row
-- that holds the retired value could no longer change. Each rule is validated
-- only when no row breaks it. Otherwise it is dropped with a notice naming it
-- and the count, and no row is touched; the Go code refuses the value from then
-- on, because `commsauthz.Basis` has no constant for it.
SET LOCAL lock_timeout = '5s';

DO $$
DECLARE
    target record;
    broken bigint;
BEGIN
    FOR target IN
        SELECT con.conrelid::regclass AS tbl, con.conname,
               pg_get_expr(con.conbin, con.conrelid) AS rule
          FROM pg_constraint con
         WHERE con.conname IN ('communication_basis_kind', 'communication_decision_basis')
           AND NOT con.convalidated
    LOOP
        EXECUTE format('SELECT count(*) FROM %s WHERE NOT COALESCE(%s, true)', target.tbl, target.rule)
           INTO broken;
        IF broken = 0 THEN
            EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', target.tbl, target.conname);
        ELSE
            EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', target.tbl, target.conname);
            RAISE NOTICE 'constraint % dropped: % existing rows hold a basis outside its list', target.conname, broken;
        END IF;
    END LOOP;
END
$$;
