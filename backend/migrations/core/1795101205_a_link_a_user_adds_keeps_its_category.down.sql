SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment DROP CONSTRAINT IF EXISTS attachment_link_shape_v2;
