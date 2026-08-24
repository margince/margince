-- The company-meeting refusal is withdrawn until a prep can reach the company
-- through the person.
--
-- 1787560001 forbade activity_link.entity_type = 'organization' on a meeting or
-- call, on the reasoning that a company is reached through the person who was in
-- the room and that "dropping it loses nothing that the employment edge does not
-- already carry".
--
-- That last part is not true of this tree. The ONLY path by which an
-- organization reaches a meeting prep is activity_link: search/graphactivity.go
-- says so where it reads the parties —
--
--     "There is no project or organization half of activity_participant —
--      those reach a prep through activity_link like everything else."
--
-- and its activityLinkArms carries an organization arm for exactly that reason.
-- There is no employment hop in the assembly path, so with the refusal in place
-- a meeting can never name its company again, on any surface. That is a
-- behaviour change nobody asked for, and it is what turned main red: five
-- integration tests in compose/integration assert the company IS named, and they
-- were failing on the seed rather than on the assertion.
--
-- So the triggers go, additively — 1787560001 has shipped and is not edited.
-- What it did to DATA is deliberate and stays: the redundant org links it
-- removed where an attendee was already named, and the meetings with no attendee
-- it re-kinded to `note`, are both defensible readings of the records and are
-- not reversible from here anyway.
--
-- The rule itself is worth having. It needs the other half first: the prep must
-- reach the company through the attendee's employer, so that forbidding the
-- direct link removes a redundancy rather than a capability. That is
-- https://github.com/margince/margince/issues/2580.

DROP TRIGGER IF EXISTS activity_link_no_company_meeting ON activity_link;
DROP TRIGGER IF EXISTS activity_no_company_meeting_on_rekind ON activity;
DROP FUNCTION IF EXISTS activity_link_refuses_a_company_meeting();
DROP FUNCTION IF EXISTS activity_refuses_becoming_a_company_meeting();
