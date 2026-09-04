-- communication_decision and consent_event are evidence, and the runtime role
-- could rewrite both.
--
-- A consent_event says what a person was shown and what they answered. A
-- communication_decision says why a message was allowed to go out. Both are
-- served to a data subject as proof and to a regulator as the controller's
-- Art. 5(2) accountability record. A proof row the application can silently
-- edit is not a proof: any defect, any injection, any mistaken repair script
-- rewrites the history that was supposed to settle the question.
--
-- Neither table has ever needed a general UPDATE. Nothing in this tree DELETEs
-- from either one at all.
--
-- COLUMN-SCOPED rather than a blanket REVOKE, because three legitimate writers
-- do exist and every one of them touches only the columns that IDENTIFY the
-- subject, never the finding:
--
--   privacy/erasure_consent.go, erasure_leadtwins.go and retentionactions.go
--   tombstone recipient_address and null subject_id/subject_kind. Art. 17 has
--   to reach the address a message went to; the verdict, category, reason and
--   ruleset survive as an unattributed statistic about a send that happened.
--
--   people/consentcarry.go re-points consent_event onto the surviving record
--   when a lead is promoted or two records merge. It moves the proof, and
--   changes nothing the proof says.
--
-- So the identity columns stay writable and the evidence columns stop being
-- writable, which is the invariant stated as a permission rather than as a
-- comment somebody has to keep obeying. A SECURITY DEFINER function would have
-- been the alternative and is not needed: no legitimate writer touches a
-- finding column, so there is nothing to define around.

REVOKE UPDATE, DELETE ON TABLE communication_decision FROM margince_app;
REVOKE UPDATE, DELETE ON TABLE consent_event FROM margince_app;

-- Exactly what erasure and the lead carry need, and nothing else.
GRANT UPDATE (recipient_address, subject_id, subject_kind)
    ON TABLE communication_decision TO margince_app;
GRANT UPDATE (person_id, lead_id)
    ON TABLE consent_event TO margince_app;
