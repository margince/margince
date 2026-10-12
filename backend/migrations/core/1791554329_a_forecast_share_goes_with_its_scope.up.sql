-- A forecast share link goes when the team or the person it is scoped to goes.
--
-- scope_id names a team or an app_user, and is empty for the workspace scope.
-- No foreign key can span the two tables, so a deleted team left live links
-- scoped to nothing. Each branch now has a key column the database derives from
-- the pair, so every writer fills it and every existing row has it, with
-- nothing to backfill.
--
-- The keys are NOT VALID: an installation may hold a link whose team was
-- deleted before this, and a validating scan would fail the migration there.
-- They bind every new row, and a delete cascades for every link whose scope is
-- still present.
--
-- Adding a stored generated column rewrites the table under ACCESS EXCLUSIVE.
-- A share link is a hand-made row, so the table is small.
SET LOCAL lock_timeout = '3s';

ALTER TABLE analytics_share
	ADD COLUMN scope_team_id uuid GENERATED ALWAYS AS (CASE WHEN scope_kind = 'team'  THEN scope_id END) STORED,
	ADD COLUMN scope_user_id uuid GENERATED ALWAYS AS (CASE WHEN scope_kind = 'owner' THEN scope_id END) STORED;

ALTER TABLE analytics_share
	ADD CONSTRAINT analytics_share_scope_team_id_fkey FOREIGN KEY (scope_team_id) REFERENCES team(id)     ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT analytics_share_scope_user_id_fkey FOREIGN KEY (scope_user_id) REFERENCES app_user(id) ON DELETE CASCADE NOT VALID;

-- A scope id lands in exactly one key column, which refuses a scope kind with
-- no column of its own. The workspace scope has no id and so no key. Validated
-- in a file of its own.
ALTER TABLE analytics_share
	ADD CONSTRAINT analytics_share_scope_shape
	CHECK (scope_id IS NULL OR num_nonnulls(scope_team_id, scope_user_id) = 1) NOT VALID;

CREATE INDEX idx_analytics_share_scope_team ON analytics_share (scope_team_id) WHERE scope_team_id IS NOT NULL;
CREATE INDEX idx_analytics_share_scope_user ON analytics_share (scope_user_id) WHERE scope_user_id IS NOT NULL;
