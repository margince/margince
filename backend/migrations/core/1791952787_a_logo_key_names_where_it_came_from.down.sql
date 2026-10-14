-- Bounded like the up beside it: a DROP CONSTRAINT takes the same lock.
SET LOCAL lock_timeout = '5s';

ALTER TABLE company DROP CONSTRAINT IF EXISTS company_logo_icon_names_its_origin;
ALTER TABLE company DROP CONSTRAINT IF EXISTS company_logo_names_its_origin;
