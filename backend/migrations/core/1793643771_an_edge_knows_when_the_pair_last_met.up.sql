SET LOCAL lock_timeout = '3s';

-- The newest meeting or call of a colleague and a contact. With the contact's
-- last message it is when the contact last took part, which the edge's recency
-- counts from, so a colleague's own unanswered send does not warm the edge.
-- NULL until the projection next folds the pair: the nightly reconcile
-- rebuilds every edge, and the score reads inbound mail alone until then.
ALTER TABLE graph_interaction_edge ADD COLUMN last_met_at timestamptz;
