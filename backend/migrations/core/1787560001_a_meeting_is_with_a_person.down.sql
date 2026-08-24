-- Removes the rule only. The data repair the up migration performed is not
-- reversed: the organization links it deleted were redundant with the
-- attendee's employment edge, and the activities it re-kinded to `note` had no
-- attendee to restore. Recreating either would put back exactly the state the
-- rule exists to forbid.
DROP TRIGGER IF EXISTS activity_no_company_meeting_on_rekind ON activity;
DROP FUNCTION IF EXISTS activity_refuses_becoming_a_company_meeting();
DROP TRIGGER IF EXISTS activity_link_no_company_meeting ON activity_link;
DROP FUNCTION IF EXISTS activity_link_refuses_a_company_meeting();
