-- Seventeen index sets covered exactly the columns another index on the same
-- table already covered. Each was a second B-tree written on every insert and
-- update of the row, a second set of pages competing for cache, and a line of
-- DDL the next reader had to account for before concluding it does nothing.
--
-- Nine here are one scar. Each was UNIQUE (workspace_id, id); when workspace_id
-- was dropped the constraint collapsed to UNIQUE (id), which the primary key
-- already enforces. `lead` was cleared earlier; `import_run` is the fork's own
-- table and its twin goes in the custom namespace, which is applied after this
-- one and is the only namespace allowed to name it.
--
-- Two are a hand-named unique constraint beside the one Postgres named for the
-- same columns. The auto-named one is kept where the pair is a `_key`, because
-- a name nobody chose is a name nobody is citing.
--
-- Five are a plain index over a unique constraint's own columns. Postgres backs
-- every unique constraint with an index, so the second one answers no query the
-- first does not.
--
-- No constraint dropped here is named as an ON CONFLICT ON CONSTRAINT target:
-- the seven statements in this tree that name one name a different constraint,
-- and dropping a constraint cited that way breaks the statement at runtime
-- rather than at migration.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call DROP CONSTRAINT uq_ai_call_ws_id;
ALTER TABLE capture_connection DROP CONSTRAINT uq_capture_connection_ws_id;
ALTER TABLE oauth_grant DROP CONSTRAINT oauth_grant_ws_id_key;
ALTER TABLE offer DROP CONSTRAINT uq_offer_ws_id;
ALTER TABLE offer_template DROP CONSTRAINT uq_offer_template_ws_id;
ALTER TABLE passport DROP CONSTRAINT uq_passport_ws_id;
ALTER TABLE product DROP CONSTRAINT uq_product_ws_id;
ALTER TABLE project DROP CONSTRAINT uq_project_ws_id;
ALTER TABLE site_read DROP CONSTRAINT uq_site_read_ws_id;

ALTER TABLE oauth_client DROP CONSTRAINT oauth_client_unique;
ALTER TABLE voice_profile_version DROP CONSTRAINT uq_voice_profile_version_profile_number;

DROP INDEX idx_brief_item_run;
DROP INDEX deal_correction_by_audit;
DROP INDEX idx_fx_rate_lookup;
DROP INDEX idx_oli_offer;
DROP INDEX idx_weekly_review_learning_review;
