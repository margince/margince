SET LOCAL lock_timeout = '3s';

UPDATE role SET permissions = permissions #- '{objects,sales_initiative}'
    WHERE permissions ? 'objects' AND (permissions -> 'objects') ? 'sales_initiative';

DROP TABLE sales_initiative_baseline;
DROP TABLE sales_initiative_member;
DROP TABLE sales_initiative_revision;
DROP TABLE sales_initiative;
