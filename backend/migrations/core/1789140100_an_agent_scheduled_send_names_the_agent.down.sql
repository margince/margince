SET LOCAL lock_timeout = '3s';

-- Back to the arm that admitted an agent row carrying no provenance. Nothing
-- has to move first: every row the narrowed constraint admits is also admitted
-- by the wider one it is restored to.
ALTER TABLE scheduled_send
    DROP CONSTRAINT IF EXISTS scheduled_send_agent_provenance_shape,
    ADD CONSTRAINT scheduled_send_agent_provenance_shape CHECK (
        ((principal_kind = 'agent'::text) AND (agent_actor_id IS NOT NULL))
        OR ((agent_actor_id IS NULL) AND (agent_passport_id IS NULL)
            AND (agent_on_behalf_of IS NULL)));
