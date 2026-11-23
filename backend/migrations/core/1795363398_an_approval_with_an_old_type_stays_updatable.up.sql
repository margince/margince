-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The three constraints that the target type migration validated only over clean
-- rows can still be NOT VALID: an installed row held an older type. PostgreSQL
-- checks a NOT VALID constraint again on every update of a row, so that row could
-- no longer change status. This file drops each one that is still NOT VALID, and
-- validates it if no row breaks it after all.
-- The Go code still refuses a new staging of an unknown type, and the notice
-- names each constraint that was dropped.
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
         WHERE con.conname IN ('approval_target_entity_type_check',
                               'approval_co_target_entity_type_check',
                               'contact_acquisition_evidence_source_entity_type_check')
           AND NOT con.convalidated
    LOOP
        EXECUTE format('SELECT count(*) FROM %s WHERE NOT COALESCE(%s, true)', target.tbl, target.rule)
           INTO broken;
        IF broken = 0 THEN
            EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', target.tbl, target.conname);
        ELSE
            EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', target.tbl, target.conname);
            RAISE NOTICE 'constraint % dropped: % existing rows hold a type outside its list', target.conname, broken;
        END IF;
    END LOOP;
END
$$;
