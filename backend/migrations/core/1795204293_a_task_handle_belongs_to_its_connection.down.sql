SET LOCAL lock_timeout = '3s';

ALTER TABLE agent_task DROP COLUMN connection_id;
