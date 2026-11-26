SET LOCAL lock_timeout = '3s';
DROP TRIGGER sdr_handoff_assigned_to_retirement_guard ON sdr_handoff;
CREATE TRIGGER sdr_handoff_assigned_to_retirement_guard BEFORE INSERT OR UPDATE ON sdr_handoff
    FOR EACH ROW WHEN (NEW.assigned_to IS NOT NULL AND (NEW.status = 'submitted'))
    EXECUTE FUNCTION refuse_retired_member_work('assigned_to');
