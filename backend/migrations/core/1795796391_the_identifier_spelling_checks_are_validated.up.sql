-- Validate the spelling checks the migration before this one added NOT VALID.
-- Its own file, so the scan runs under VALIDATE's lighter lock. A check that an
-- existing row breaks is dropped with a notice naming it and the count: PostgreSQL
-- re-checks a NOT VALID constraint on every update, so that row could not change.
-- values still writes the one spelling, and the gate keeps the check declared.
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
         WHERE con.conname IN ('app_user_email_one_spelling',
                               'capture_freemail_domain_domain_one_spelling',
                               'capture_pending_counterparty_email_one_spelling',
                               'capture_sender_override_address_one_spelling',
                               'communication_suppression_address_one_spelling',
                               'company_domain_domain_one_spelling',
                               'company_domain_disposition_domain_one_spelling',
                               'contact_email_email_one_spelling',
                               'deal_room_participant_email_one_spelling',
                               'lead_email_one_spelling',
                               'workspace_email_domain_domain_one_spelling')
           AND NOT con.convalidated
    LOOP
        EXECUTE format('SELECT count(*) FROM %s WHERE NOT COALESCE(%s, true)', target.tbl, target.rule)
           INTO broken;
        IF broken = 0 THEN
            EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', target.tbl, target.conname);
        ELSE
            EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', target.tbl, target.conname);
            RAISE NOTICE 'constraint % dropped: % existing rows hold another spelling', target.conname, broken;
        END IF;
    END LOOP;
END
$$;
