-- Drops the refusal. The data repair above is not undone: the redundant links
-- it removed and the attendee-less meetings it re-kinded are readings of those
-- records, and nothing here remembers what they were before.
DROP TRIGGER IF EXISTS activity_link_no_company_meeting ON activity_link;
DROP TRIGGER IF EXISTS activity_no_company_meeting_on_rekind ON activity;
DROP FUNCTION IF EXISTS activity_link_refuses_a_company_meeting();
DROP FUNCTION IF EXISTS activity_refuses_becoming_a_company_meeting();
