-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The two report_edition keys the file before this one added NOT VALID are
-- validated here, in their own transaction, so the scan runs under SHARE
-- UPDATE EXCLUSIVE. A key is validated only when every row names a row that
-- exists. Otherwise it is dropped and the notice names it, because a NOT VALID
-- key is checked again on every update of the row that breaks it.
SET LOCAL lock_timeout = '5s';

DO $$
DECLARE
    target record;
    broken bigint;
BEGIN
    FOR target IN
        SELECT conname, col, parent
          FROM (VALUES ('report_edition_owner_id_fkey', 'owner_id', 'app_user'),
                       ('report_edition_publication_team_id_fkey', 'publication_team_id', 'team')) AS k(conname, col, parent)
         WHERE EXISTS (SELECT 1 FROM pg_constraint con
                        WHERE con.conrelid = 'report_edition'::regclass AND con.conname = k.conname
                          AND NOT con.convalidated)
    LOOP
        EXECUTE format('SELECT count(*) FROM report_edition e WHERE e.%I IS NOT NULL
                          AND NOT EXISTS (SELECT 1 FROM %I p WHERE p.id = e.%I)', target.col, target.parent, target.col)
           INTO broken;
        IF broken = 0 THEN
            EXECUTE format('ALTER TABLE report_edition VALIDATE CONSTRAINT %I', target.conname);
        ELSE
            EXECUTE format('ALTER TABLE report_edition DROP CONSTRAINT %I', target.conname);
            RAISE NOTICE 'constraint % dropped: % existing editions name a row that does not exist', target.conname, broken;
        END IF;
    END LOOP;
END
$$;
