-- Checks the rows already stored against the constraints the previous migration
-- added NOT VALID, then lets the widened category check take the old one's name.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment VALIDATE CONSTRAINT attachment_category_check_v2;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_link_shape;
ALTER TABLE attachment DROP CONSTRAINT attachment_category_check;
ALTER TABLE attachment RENAME CONSTRAINT attachment_category_check_v2 TO attachment_category_check;
