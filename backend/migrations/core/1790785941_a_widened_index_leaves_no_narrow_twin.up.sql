-- The cascade-index migration (1790344400) wrote its own rule: where a partial
-- covered the same columns as the wide index it needed, the partial is WIDENED
-- rather than joined by a second index, because the full one answers every
-- query the partial did. It followed that nine times and left four standing.
--
-- Two more of the same shape are not that migration's doing and are dropped for
-- their own reasons.
--
-- project: nothing in this tree filters `phase <> 'closed'`, in any spelling,
-- so the narrow index has no caller at all and idx_project_company beside it
-- serves every query that reaches company_id.
--
-- company_relationship_type: the issue asked which of the pair arrived first.
-- Neither — both were created in the 0001 baseline two lines apart, so this is
-- not residue from a widening but the same duplication written twice at the
-- start. The unpredicated twin is the cascade index and stays.
--
-- deal_room_thread keeps its narrow index: `state = 'open' AND required_change`
-- is genuinely selective, so it is small and the wide one would scan more heap.
-- That is a measurement question rather than a rule, and the rule does not reach
-- it.
SET LOCAL lock_timeout = '3s';

DROP INDEX idx_booking_page_host;
DROP INDEX communication_suppression_live_contact;
DROP INDEX conversation_claim_activity_ix;
DROP INDEX idx_passport_obo;
DROP INDEX idx_project_company_open;
DROP INDEX idx_company_rel_type_company;
