SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS sales_initiative_owner_id_retirement_guard ON sales_initiative;
ALTER TABLE deal DROP CONSTRAINT IF EXISTS deal_amount_minor_base_js_safe;

UPDATE role SET permissions = permissions #- '{objects,sales_initiative}'
    WHERE permissions ? 'objects' AND (permissions -> 'objects') ? 'sales_initiative';

DROP TABLE sales_initiative_baseline;
DROP TABLE sales_initiative_member;
DROP TABLE sales_initiative_revision;
DROP TABLE sales_initiative;
