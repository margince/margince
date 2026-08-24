-- Restore the refusal exactly as 1787560001 declared it.
--
-- Rolling back to a schema that carries the triggers means carrying the
-- behaviour that made main red, so this is reversible in the schema sense only.

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
