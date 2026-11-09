SET LOCAL lock_timeout = '3s';

-- How many messages each colleague sent the contact, not just how many outbound
-- mails they were on. NULL means the pair was folded before this column: the
-- nightly reconcile or the next activity event for the pair fills it, and
-- until then readers keep the outbound count's reading.
ALTER TABLE graph_interaction_edge ADD COLUMN sent_count_90d integer;
