SET LOCAL lock_timeout = '3s';

ALTER TABLE analytics_share
	DROP COLUMN scope_team_id,
	DROP COLUMN scope_user_id;
