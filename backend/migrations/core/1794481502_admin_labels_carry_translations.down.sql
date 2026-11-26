SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field DROP COLUMN option_translations, DROP COLUMN label_translations;
ALTER TABLE stage DROP COLUMN name_translations;
