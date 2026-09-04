REVOKE UPDATE (recipient_address, subject_id, subject_kind)
    ON TABLE communication_decision FROM margince_app;
REVOKE UPDATE (person_id, lead_id)
    ON TABLE consent_event FROM margince_app;

GRANT UPDATE, DELETE ON TABLE communication_decision TO margince_app;
GRANT UPDATE, DELETE ON TABLE consent_event TO margince_app;
