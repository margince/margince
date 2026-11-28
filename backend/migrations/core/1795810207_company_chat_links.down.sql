SET LOCAL lock_timeout = '5s';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM chat_link WHERE company_id IS NOT NULL) THEN
        RAISE EXCEPTION 'Export and remove company channel associations before downgrading.';
    END IF;
END
$$;
ALTER TABLE chat_link DROP CONSTRAINT chat_link_record_check;
ALTER TABLE chat_link DROP COLUMN company_id;
ALTER TABLE chat_link ADD CONSTRAINT chat_link_record_check CHECK (deal_id IS NOT NULL OR project_id IS NOT NULL) NOT VALID;
