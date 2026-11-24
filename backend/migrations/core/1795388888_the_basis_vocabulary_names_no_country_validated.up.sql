-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Settle the two basis checks the migration before this one added NOT VALID.
-- Its own file, so a scan runs under the lighter lock and not under the
-- ACCESS EXCLUSIVE that the ALTER held to its commit.
--
-- PostgreSQL checks a NOT VALID rule again on every update of a row, so a row
-- that holds the retired value could no longer change. A rule that no row
-- breaks is validated, and the closed list stands. On a table that holds the
-- retired value, the rule is replaced by the same list plus that one value,
-- which the existing rows pass and any other string fails. No row is touched,
-- and the notice names the rule and the count. The Go code refuses the retired
-- value either way, because `commsauthz.Basis` has no constant for it.
SET LOCAL lock_timeout = '5s';

DO $$
DECLARE
    target record;
    broken bigint;
BEGIN
    FOR target IN
        SELECT con.conrelid::regclass AS tbl, con.conname,
               pg_get_expr(con.conbin, con.conrelid) AS rule,
               CASE con.conname WHEN 'communication_basis_kind' THEN 'kind' ELSE 'basis' END AS col
          FROM pg_constraint con
         WHERE (con.conrelid, con.conname) IN (
                   ('communication_basis'::regclass, 'communication_basis_kind'),
                   ('communication_decision'::regclass, 'communication_decision_basis'))
           AND NOT con.convalidated
    LOOP
        EXECUTE format('SELECT count(*) FROM %s WHERE NOT COALESCE(%s, true)', target.tbl, target.rule)
           INTO broken;
        IF broken = 0 THEN
            EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', target.tbl, target.conname);
        ELSE
            EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', target.tbl, target.conname);
            EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I CHECK ((%s) OR %I = %L)',
                           target.tbl, target.conname, target.rule, target.col, 'vn_subject_agreement');
            RAISE NOTICE 'constraint % admits the retired basis: % existing rows hold it', target.conname, broken;
        END IF;
    END LOOP;
END
$$;
