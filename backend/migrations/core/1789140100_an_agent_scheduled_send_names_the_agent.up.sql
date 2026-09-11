SET LOCAL lock_timeout = '3s';

-- An agent-scheduled message must say WHICH agent scheduled it.
--
-- The constraint this replaces permitted an agent row with no provenance at
-- all. Its second arm — all three columns NULL — was written to keep rows from
-- before the provenance columns existed valid, and it did not exclude
-- `principal_kind = 'agent'`. So an agent row with nothing recorded satisfied
-- it, and two very different things looked identical in the table: a row
-- written before the columns existed, and a writer that forgot them.
--
-- The fire path could not tell them apart either, and resolved the ambiguity
-- the only way it could — by deriving `agent:<human-uuid>`, an actor that never
-- existed, which collapses every agent acting for one person into one identity
-- and is the attribution defect that derivation was removed for. A future
-- writer that forgot these columns would have degraded silently back into it.
--
-- There is nothing to grandfather. No installation predates the columns: a
-- fresh install runs the baseline, which already creates the table carrying
-- them, so every row on every install is a post-provenance row. The arm was
-- protecting a population that does not exist, and it was the hole.
--
-- WHAT COMPLETE MEANS, stated here because this is the moment to say it: the
-- ACTOR id is required and the other two are not. A passport records how an
-- agent's scopes were granted and NULL says none was presented; on_behalf_of
-- records the human behind the agent and NULL says there is none to name. Both
-- are facts an agent may legitimately lack, and requiring them would refuse
-- writes the store makes on purpose (activities/scheduledsendprovenance.go
-- states both rules). The actor id is the one thing no agent can lack, because
-- the actor is what "an agent scheduled this" means — and it has to be a NAME
-- rather than the empty string, which IS NOT NULL alone admits. The writers
-- pass principal.ID through unchecked, so a blank would satisfy the column and
-- then reach the fire path as an agent row naming nobody: the same hole with a
-- different spelling.
--
-- The all-NULL arm now says which rows it is for, which is the change.
-- Spelled as a plain disjunction rather than a CASE because pg_get_constraintdef
-- renders a CASE across five lines, and testdata/head_catalog.txt is one
-- constraint per line.
ALTER TABLE scheduled_send
    DROP CONSTRAINT IF EXISTS scheduled_send_agent_provenance_shape,
    ADD CONSTRAINT scheduled_send_agent_provenance_shape CHECK (
        ((principal_kind = 'agent'::text) AND (agent_actor_id IS NOT NULL)
            AND (length(btrim(agent_actor_id)) > 0))
        OR ((principal_kind <> 'agent'::text) AND (agent_actor_id IS NULL)
            AND (agent_passport_id IS NULL) AND (agent_on_behalf_of IS NULL)));
