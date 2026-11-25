-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The array checks the file before this one added NOT VALID are validated here,
-- in their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE. A
-- check is validated only when no row breaks it. Otherwise it is dropped and
-- the notice names it, because a NOT VALID check is tested again on every
-- update of the row that breaks it. The Go writer still refuses a word outside
-- the list either way.
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
         WHERE con.conname IN ('passport_scopes_vocabulary',
                               'oauth_grant_scopes_vocabulary',
                               'passport_work_areas_vocabulary',
                               'oauth_grant_work_areas_vocabulary',
                               'oauth_authorization_code_work_areas_vocabulary',
                               'custom_field_activity_kinds_vocabulary',
                               'deal_manager_request_reasons_vocabulary')
           AND NOT con.convalidated
    LOOP
        EXECUTE format('SELECT count(*) FROM %s WHERE NOT COALESCE(%s, true)', target.tbl, target.rule)
           INTO broken;
        IF broken = 0 THEN
            EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', target.tbl, target.conname);
        ELSE
            EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', target.tbl, target.conname);
            RAISE NOTICE 'constraint % dropped: % existing rows hold a word outside its list', target.conname, broken;
        END IF;
    END LOOP;
END
$$;
