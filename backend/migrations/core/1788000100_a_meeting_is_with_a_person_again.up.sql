-- A meeting is with a person. A company is not somebody you can meet: it is
-- reached through the person who was in the room, and the company timeline is a
-- rollup of its people's activity rather than a place activity can be filed
-- directly.
--
-- This restores what 1787560001 asserted and 1787570000 withdrew. The rule was
-- right and its premise was not yet true: the only path by which an
-- organization reached a meeting prep was activity_link itself, so forbidding
-- the direct link removed the company from every surface that assembles
-- context rather than removing a redundancy.
--
-- The premise now holds. search/graphactivitysubjects.go walks
-- activity_participant -> person -> employment -> organization
-- (employerSubjects), under the same RBAC and row-scope probe every other
-- candidate goes through, and ranks the inferred company below a directly
-- linked one and above the attendee it was reached through. A meeting linked
-- to its attendee names that attendee's company, which is what "reached
-- through the person" has to mean before the link can be refused.
--
-- Email is deliberately NOT included. A mail can legitimately be addressed to
-- an account alias nobody owns personally; that is a separate question from who
-- sat in a meeting. `note`, `task` and the other kinds stay unrestricted for the
-- same reason: they are about a record, not with a human.
--
-- The DATA repair 1787560001 performed is not repeated. It ran once, its
-- readings are not reversible from here, and the estate has been unconstrained
-- since 1787570000 — so any company meeting written in between is repaired the
-- same way rather than blocking the constraint.

-- The organization link is redundant only where the company is ACTUALLY
-- reachable without it — and "reachable" is not a judgement this migration is
-- free to make its own. It is whatever activityReachesOrg finds, because that
-- walk is what has to carry the meeting to the account once the link is gone,
-- and the delete cannot be undone.
--
-- So the predicate below is that walk's employment arm, exactly:
--
--   activity_link.person_id -> relationship(kind='employment',
--                                           ended_at IS NULL,
--                                           archived_at IS NULL) -> organization
--
-- Deliberately NARROWER than either the truth or the prep. The walk does not
-- follow activity_participant (#3130) and reads `ended_at IS NULL` rather than
-- the date comparison people.EmploymentIsCurrentSQL makes (#2360), so a link
-- deleted on the wider reading would be a company the timeline cannot recover.
-- Kept where the walk cannot reach it, the activity simply becomes a `note`
-- below and keeps the link that carries it — the recoverable answer.
--
-- 1787560001 took ANY person link as proof of redundancy, which it could: that
-- workspace had been checked by hand, 39 of 39. This one cannot — the estate
-- has been unconstrained since 1787570000, and a linked contact may work
-- somewhere else or nowhere.
--
-- The person must also be LIVE, which narrows it further than the walk. That is
-- the safe direction: an archived contact carries their employer into no prep
-- (employerSubjects skips them), so treating their employment as redundancy
-- would be deleting on a reading nothing else makes.
DELETE FROM activity_link ol
WHERE ol.entity_type = 'organization'
  AND EXISTS (
        SELECT 1 FROM activity a
        WHERE a.id = ol.activity_id AND a.kind IN ('meeting', 'call'))
  AND EXISTS (
        SELECT 1
          FROM activity_link pl
          JOIN person p ON p.id = pl.person_id
          JOIN relationship r ON r.person_id = pl.person_id
         WHERE pl.activity_id = ol.activity_id
           AND pl.person_id IS NOT NULL
           AND p.archived_at IS NULL
           AND r.kind = 'employment' AND r.ended_at IS NULL AND r.archived_at IS NULL
           AND r.organization_id = ol.organization_id);

-- What remains is a meeting whose company nothing else reaches. Dropping the
-- link would erase it from that company's timeline, so the link is kept and the
-- activity is re-kinded to `note`: a record ABOUT the company, which is what a
-- meeting nobody at that company attended actually is. The subject is left
-- alone, so the history still reads as the meeting it describes.
--
-- Wider than 1787560001's version, which re-kinded only the meetings with no
-- person link at all. A meeting linked to somebody who does not work there is
-- the same case: the company is not reached through anyone, and the link is
-- carrying it alone.
-- host_user_id goes with the kind: activity_meeting_host admits it only on a
-- meeting, so a re-kind that left it set would abort the whole migration on the
-- first hosted meeting it reached. 1787560001 carried the same statement and
-- survived only because no such row existed in the workspace it ran on.
UPDATE activity a
SET kind = 'note', host_user_id = NULL
WHERE a.kind IN ('meeting', 'call')
  AND EXISTS (
        SELECT 1 FROM activity_link ol
        WHERE ol.activity_id = a.id AND ol.entity_type = 'organization');

-- The rule itself. A trigger rather than a CHECK, because the condition spans
-- two tables: the kind lives on activity and the link on activity_link. In the
-- database rather than in a write path, because the write path is not the only
-- writer — the MCP tool, a REST caller and the web app all insert here.
CREATE OR REPLACE FUNCTION activity_link_refuses_a_company_meeting()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    activity_kind text;
BEGIN
    IF NEW.entity_type <> 'organization' THEN
        RETURN NEW;
    END IF;

    -- LOCKED BEFORE THE CHECK, and the same row the other trigger locks. Read
    -- unlocked, one transaction can insert this link while the activity is
    -- still a `note` (the check below passes) while another re-kinds that same
    -- activity to `meeting` before the link is visible (the check in
    -- activity_refuses_becoming_a_company_meeting passes too). Both commit, and
    -- the row neither of them was allowed to make exists.
    --
    -- The ORDER is the whole of it: locked first, checked second, and the same
    -- row the re-kind trigger locks. A BEFORE ROW trigger runs ahead of the
    -- UPDATE's own row lock, so a re-kind whose check has already run cannot be
    -- made to look again by blocking it afterwards — it would wake and write on
    -- the answer it took before waiting. Locking first is what makes each
    -- check happen after the other transaction has finished or not started.
    --
    -- SHARE here and UPDATE there, which is the pairing that conflicts where it
    -- should: a link and a re-kind queue, while two links onto one activity do
    -- not — they are not in conflict and have no reason to wait for each other.
    PERFORM 1 FROM activity WHERE id = NEW.activity_id FOR SHARE;
    SELECT kind INTO activity_kind FROM activity WHERE id = NEW.activity_id;

    IF activity_kind IN ('meeting', 'call') THEN
        RAISE EXCEPTION
            'a % is with a person, not with a company: link it to the person who was there, and the company sees it through them',
            activity_kind
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER activity_link_no_company_meeting
    BEFORE INSERT OR UPDATE ON activity_link
    FOR EACH ROW
    EXECUTE FUNCTION activity_link_refuses_a_company_meeting();

-- The kind can also be changed AFTER the link exists, which would smuggle a
-- company meeting past the link-side trigger.
CREATE OR REPLACE FUNCTION activity_refuses_becoming_a_company_meeting()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.kind NOT IN ('meeting', 'call') OR OLD.kind = NEW.kind THEN
        RETURN NEW;
    END IF;

    -- The other half of the pairing, taken BEFORE the check for the same
    -- reason: this trigger runs ahead of the UPDATE's own row lock, so waiting
    -- afterwards would leave it writing on an answer it read before a
    -- concurrent link landed. FOR UPDATE, so it conflicts with the link side's
    -- SHARE.
    PERFORM 1 FROM activity WHERE id = NEW.id FOR UPDATE;

    IF EXISTS (SELECT 1 FROM activity_link
               WHERE activity_id = NEW.id AND entity_type = 'organization') THEN
        RAISE EXCEPTION
            'this activity is linked to a company, so it cannot become a %: a % is with a person',
            NEW.kind, NEW.kind
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER activity_no_company_meeting_on_rekind
    BEFORE UPDATE OF kind ON activity
    FOR EACH ROW
    EXECUTE FUNCTION activity_refuses_becoming_a_company_meeting();
