-- Checks the stored rows against the shape the previous migration added NOT
-- VALID, then retires the shape that tied an address to the `link` category.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment VALIDATE CONSTRAINT attachment_link_shape_v2;
ALTER TABLE attachment DROP CONSTRAINT attachment_link_shape;
ALTER TABLE attachment RENAME CONSTRAINT attachment_link_shape_v2 TO attachment_link_shape;
