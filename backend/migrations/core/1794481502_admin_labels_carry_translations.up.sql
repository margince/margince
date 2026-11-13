-- An admin-named label may carry its name in further UI languages. Each map is
-- language code to text, for example {"vi": "Đã thắng"}; the stored label stays
-- the fallback. Adding a column with a constant default rewrites no rows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE stage ADD COLUMN name_translations jsonb NOT NULL DEFAULT '{}'::jsonb;

-- option_translations maps each picklist option to its own language map.
ALTER TABLE custom_field
    ADD COLUMN label_translations jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN option_translations jsonb NOT NULL DEFAULT '{}'::jsonb;
