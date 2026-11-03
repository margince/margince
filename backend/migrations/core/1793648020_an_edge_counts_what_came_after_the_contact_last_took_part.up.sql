SET LOCAL lock_timeout = '3s';

-- The window's interactions on a colleague edge after the contact last took
-- part, and the outbound part of them. The score leaves them out, so a
-- colleague's own unanswered send neither warms the edge nor balances its
-- exchange. NULL means the pair was folded before this rule: the score reads
-- such a row by the earlier rule until the nightly reconcile refolds it.
ALTER TABLE graph_interaction_edge
  ADD COLUMN late_count_90d integer,
  ADD COLUMN late_out_count_90d integer;
