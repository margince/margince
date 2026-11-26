SET LOCAL lock_timeout = '3s';
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM sdr_handoff h JOIN app_user u ON u.id = h.assigned_to
        WHERE h.status = 'accepted' AND u.removed_at IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'Accepted handoffs retain retired receivers; this upgrade cannot be reversed safely'
            USING ERRCODE = '23514';
    END IF;
END $$;
DROP TRIGGER sdr_handoff_assigned_to_retirement_guard ON sdr_handoff;
CREATE TRIGGER sdr_handoff_assigned_to_retirement_guard BEFORE INSERT OR UPDATE ON sdr_handoff
    FOR EACH ROW WHEN (NEW.assigned_to IS NOT NULL AND (NEW.status IN ('submitted','accepted')))
    EXECUTE FUNCTION refuse_retired_member_work('assigned_to');
