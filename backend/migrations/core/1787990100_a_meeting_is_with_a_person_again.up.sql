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
-- The premise now holds. search/graphactivity.go walks
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

-- Where the meeting already names an attendee, the organization link is
-- redundant: the company now reaches the prep through that attendee's employer.
DELETE FROM activity_link ol
WHERE ol.entity_type = 'organization'
  AND EXISTS (
        SELECT 1 FROM activity a
        WHERE a.id = ol.activity_id AND a.kind IN ('meeting', 'call'))
  AND EXISTS (
        SELECT 1 FROM activity_link pl
        WHERE pl.activity_id = ol.activity_id AND pl.entity_type = 'person');

-- What remains is a meeting linked to a company and to nobody. Dropping the
-- link outright would erase it from every timeline, so the org link is kept and
-- the activity is re-kinded to `note`: a record ABOUT the company, which is what
-- a meeting with no attendee actually is. The subject is left alone, so the
-- history still reads as the meeting it describes.
UPDATE activity a
SET kind = 'note'
WHERE a.kind IN ('meeting', 'call')
  AND EXISTS (
        SELECT 1 FROM activity_link ol
        WHERE ol.activity_id = a.id AND ol.entity_type = 'organization')
  AND NOT EXISTS (
        SELECT 1 FROM activity_link pl
        WHERE pl.activity_id = a.id AND pl.entity_type = 'person');

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
