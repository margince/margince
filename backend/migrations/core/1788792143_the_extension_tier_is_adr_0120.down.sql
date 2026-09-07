SET LOCAL lock_timeout = '3s';

COMMENT ON SCHEMA ext IS 'Extension tables (ADR-0069): ext_<name>_<table>, applied by the migrate role from each enabled unit''s own migrations. A unit is bounded by the tables its migrations create and the grants they carry — there is no tenant column and no policy here, because an installation holds one workspace (ADR-0061) and no schema in this database carries either. A per-unit owner role would make the bound a grant rather than a convention; it exists today only in the pre-merge migration gate (issue #628). The core owns public; nothing here is core data.';

COMMENT ON COLUMN extension_secret.vault_ref IS 'An opaque keyvault handle (ADR-0069): safe to log, never the secret itself.';
