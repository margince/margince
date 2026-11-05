SET LOCAL lock_timeout = '3s';

DELETE FROM attachment WHERE category = 'link';
ALTER TABLE attachment DROP CONSTRAINT attachment_link_shape;
ALTER TABLE attachment DROP CONSTRAINT IF EXISTS attachment_category_check_v2;
ALTER TABLE attachment DROP COLUMN url;
