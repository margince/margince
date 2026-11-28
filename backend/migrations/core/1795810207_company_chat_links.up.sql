SET LOCAL lock_timeout = '5s';

ALTER TABLE chat_link ADD COLUMN company_id uuid REFERENCES company(id) ON DELETE CASCADE;
ALTER TABLE chat_link DROP CONSTRAINT chat_link_record_check;
ALTER TABLE chat_link ADD CONSTRAINT chat_link_record_check CHECK (
    (company_id IS NOT NULL AND deal_id IS NULL AND project_id IS NULL)
    OR (company_id IS NULL AND (deal_id IS NOT NULL OR project_id IS NOT NULL))
) NOT VALID;
