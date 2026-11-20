-- A task handle belongs to the connected agent, not to one passport row. A token
-- refresh retires the passport and mints another under the same OAuth grant, so
-- the grant is what lets the agent still read its handle afterwards.
SET LOCAL lock_timeout = '3s';

ALTER TABLE agent_task
    ADD COLUMN connection_id uuid
        CONSTRAINT agent_task_connection_fk REFERENCES oauth_grant(id) ON DELETE RESTRICT;
