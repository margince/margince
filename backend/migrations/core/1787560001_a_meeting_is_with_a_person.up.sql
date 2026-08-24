-- A meeting is with a person. A company is not somebody you can meet: it is
-- reached through the person who was in the room, and the company timeline is a
-- rollup of its people's activity rather than a place activity can be filed
-- directly.
--
-- activity_link did not say so. `organization` was a first-class entity_type,
-- peer to `person`, and nothing required a meeting or a call to name an
-- attendee at all. Any writer could file a meeting straight onto a company —
-- the MCP tool, a REST caller, and the web app alike, because they all write
-- this table. The result was a company timeline holding meetings nobody
-- attended, and two records that disagreed about who was in the room with no
-- constraint to settle it.
--
-- This forbids the direct link for the two kinds that are inherently personal,
-- `meeting` and `call`, and enforces it in the database rather than in a write
-- path, because the write path is not the only writer.
--
-- Email is deliberately NOT included. A mail can legitimately be addressed to
-- an account alias nobody owns personally, and 279 such rows exist here; that
-- is a separate question from who sat in a meeting. `note`, `task` and the
-- other kinds stay unrestricted for the same reason: they are about a record,
-- not with a human.

-- Repair before constraining, or the constraint cannot be added.
--
-- Where the meeting already names an attendee, the organization link is
-- redundant: it was verified to equal that attendee's employer in every case
-- present (39 of 39, no mismatch), so dropping it loses nothing that the
-- employment edge does not already carry.
DELETE FROM activity_link ol
WHERE ol.entity_type = 'organization'
  AND EXISTS (
        SELECT 1 FROM activity a
        WHERE a.id = ol.activity_id AND a.kind IN ('meeting', 'call'))
  AND EXISTS (
        SELECT 1 FROM activity_link pl
        WHERE pl.activity_id = ol.activity_id AND pl.entity_type = 'person');

-- What remains is a meeting linked to a company and to nobody. Dropping the
-- link outright would erase it from every timeline, so the org link is kept
-- and the activity is re-kinded to `note`: a record ABOUT the company, which
-- is what a meeting with no attendee actually is. The subject is left alone,
-- so the history still reads as the meeting it describes.
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
-- two tables: the kind lives on activity and the link on activity_link.
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
