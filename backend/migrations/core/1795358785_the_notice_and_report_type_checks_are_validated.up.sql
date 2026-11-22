-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The constraints added NOT VALID by the file before this one are validated here,
-- in their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
-- A constraint is validated only when no row breaks it. An installed row that
-- holds an older type keeps its constraint NOT VALID, which still refuses every
-- new bad write, and the upgrade goes on. The notice names what is left.
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
         WHERE con.conname IN ('notice_target_type_check',
                               'report_edition_contribution_source_type_check')
           AND NOT con.convalidated
    LOOP
        EXECUTE format('SELECT count(*) FROM %s WHERE NOT COALESCE(%s, true)', target.tbl, target.rule)
           INTO broken;
        IF broken = 0 THEN
            EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', target.tbl, target.conname);
        ELSE
            RAISE NOTICE 'constraint % stays NOT VALID: % existing rows hold a type outside its list', target.conname, broken;
        END IF;
    END LOOP;
END
$$;
