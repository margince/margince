-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the CHECKs the previous migration added NOT VALID. It is a file of
-- its own so the scan takes only SHARE UPDATE EXCLUSIVE and writers go through.
--
-- A table that still keeps rows of an extension kind keeps its CHECK NOT VALID.
-- The CHECK refuses new such rows either way, and no stored row is deleted to
-- make it pass. The NOTICE names the table, so an operator can see why.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE
    target record;
BEGIN
    FOR target IN
        SELECT * FROM (VALUES
            ('attachment', 'attachment_entity_type_check'),
            ('embedding', 'embedding_entity_type_check'),
            ('field_provenance', 'field_provenance_object_type_check'),
            ('user_record_view', 'user_record_view_entity_type_check'),
            ('signal', 'signal_entity_type_check'),
            ('dedupe_candidate', 'dedupe_candidate_entity_type_check'),
            ('dedupe_candidate', 'dedupe_candidate_shape')
        ) AS t(tbl, con)
    LOOP
        BEGIN
            EXECUTE format('ALTER TABLE %I VALIDATE CONSTRAINT %I', target.tbl, target.con);
        EXCEPTION WHEN check_violation THEN
            RAISE NOTICE '% keeps rows of an extension kind, so % stays NOT VALID and refuses new ones',
                target.tbl, target.con;
        END;
    END LOOP;
END $$;
