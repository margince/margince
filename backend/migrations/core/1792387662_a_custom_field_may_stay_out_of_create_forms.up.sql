SET LOCAL lock_timeout = '3s';

-- Default true: every field that exists keeps showing in the create dialogs it
-- showed in before. A constant default is a catalog-only change, so the ALTER
-- rewrites no rows.
ALTER TABLE custom_field ADD COLUMN show_in_create boolean NOT NULL DEFAULT true;
