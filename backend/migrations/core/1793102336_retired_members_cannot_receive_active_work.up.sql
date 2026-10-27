SET LOCAL lock_timeout = '3s';
CREATE FUNCTION refuse_retired_member_work() RETURNS trigger LANGUAGE plpgsql AS $$DECLARE
    member_id uuid;
    retired boolean;
BEGIN
    member_id := (to_jsonb(NEW)->>TG_ARGV[0])::uuid;
    SELECT removed_at IS NOT NULL INTO retired FROM app_user WHERE id = member_id FOR KEY SHARE;
    IF retired THEN
        RAISE EXCEPTION 'A retired member cannot receive active work'
            USING ERRCODE = '23514', CONSTRAINT = 'member_work_reference_retired';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER activity_assignee_id_retirement_guard BEFORE INSERT OR UPDATE ON activity
    FOR EACH ROW WHEN (NEW.assignee_id IS NOT NULL AND (NEW.archived_at IS NULL AND NEW.kind = 'task' AND NOT NEW.is_done))
    EXECUTE FUNCTION refuse_retired_member_work('assignee_id');
CREATE TRIGGER analytics_share_scope_user_id_retirement_guard BEFORE INSERT OR UPDATE ON analytics_share
    FOR EACH ROW WHEN (NEW.scope_id IS NOT NULL AND (NEW.scope_kind = 'owner' AND NEW.revoked_at IS NULL AND NEW.expires_at > now()))
    EXECUTE FUNCTION refuse_retired_member_work('scope_id');
CREATE TRIGGER approval_requested_decider_id_retirement_guard BEFORE INSERT OR UPDATE ON approval
    FOR EACH ROW WHEN (NEW.requested_decider_id IS NOT NULL AND (NEW.status = 'pending'))
    EXECUTE FUNCTION refuse_retired_member_work('requested_decider_id');
CREATE TRIGGER automation_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON automation
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.enabled AND NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER booking_page_host_user_id_retirement_guard BEFORE INSERT OR UPDATE ON booking_page
    FOR EACH ROW WHEN (NEW.host_user_id IS NOT NULL AND (true))
    EXECUTE FUNCTION refuse_retired_member_work('host_user_id');
CREATE TRIGGER company_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON company
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER contact_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON contact
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER data_subject_request_assignee_id_retirement_guard BEFORE INSERT OR UPDATE ON data_subject_request
    FOR EACH ROW WHEN (NEW.assignee_id IS NOT NULL AND (NEW.status IN ('open','in_progress')))
    EXECUTE FUNCTION refuse_retired_member_work('assignee_id');
CREATE TRIGGER deal_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON deal
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER deal_room_steward_user_id_retirement_guard BEFORE INSERT OR UPDATE ON deal_room
    FOR EACH ROW WHEN (NEW.steward_user_id IS NOT NULL AND (NEW.archived_at IS NULL AND NEW.state NOT IN ('closed','expired')))
    EXECUTE FUNCTION refuse_retired_member_work('steward_user_id');
CREATE TRIGGER intro_request_introducer_user_id_retirement_guard BEFORE INSERT OR UPDATE ON intro_request
    FOR EACH ROW WHEN (NEW.introducer_user_id IS NOT NULL AND (NEW.archived_at IS NULL AND NEW.closed_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('introducer_user_id');
CREATE TRIGGER lead_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON lead
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER list_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON list
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER list_steward_id_retirement_guard BEFORE INSERT OR UPDATE ON list
    FOR EACH ROW WHEN (NEW.steward_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('steward_id');
CREATE TRIGGER mail_draft_author_id_retirement_guard BEFORE INSERT OR UPDATE ON mail_draft
    FOR EACH ROW WHEN (NEW.author_id IS NOT NULL AND (true))
    EXECUTE FUNCTION refuse_retired_member_work('author_id');
CREATE TRIGGER meeting_invitation_host_user_id_retirement_guard BEFORE INSERT OR UPDATE ON meeting_invitation
    FOR EACH ROW WHEN (NEW.host_user_id IS NOT NULL AND (NEW.status NOT IN ('canceled','erased') OR NOT NEW.cleanup_complete))
    EXECUTE FUNCTION refuse_retired_member_work('host_user_id');
CREATE TRIGGER meeting_proposal_host_user_id_retirement_guard BEFORE INSERT OR UPDATE ON meeting_proposal
    FOR EACH ROW WHEN (NEW.host_user_id IS NOT NULL AND (true))
    EXECUTE FUNCTION refuse_retired_member_work('host_user_id');
CREATE TRIGGER privacy_notice_case_owner_user_id_retirement_guard BEFORE INSERT OR UPDATE ON privacy_notice_case
    FOR EACH ROW WHEN (NEW.owner_user_id IS NOT NULL AND (NEW.state IN ('open','assigned','queued','delivery_failed','blocked')))
    EXECUTE FUNCTION refuse_retired_member_work('owner_user_id');
CREATE TRIGGER project_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON project
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER record_assignment_user_id_retirement_guard BEFORE INSERT OR UPDATE ON record_assignment
    FOR EACH ROW WHEN (NEW.user_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('user_id');
CREATE TRIGGER report_definition_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON report_definition
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER report_schedule_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON report_schedule
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.enabled))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER saved_view_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON saved_view
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER scheduled_send_scheduled_by_retirement_guard BEFORE INSERT OR UPDATE ON scheduled_send
    FOR EACH ROW WHEN (NEW.scheduled_by IS NOT NULL AND (NEW.status IN ('scheduled','held','sending')))
    EXECUTE FUNCTION refuse_retired_member_work('scheduled_by');
CREATE TRIGGER sdr_handoff_assigned_to_retirement_guard BEFORE INSERT OR UPDATE ON sdr_handoff
    FOR EACH ROW WHEN (NEW.assigned_to IS NOT NULL AND (NEW.status IN ('submitted','accepted')))
    EXECUTE FUNCTION refuse_retired_member_work('assigned_to');
CREATE TRIGGER signal_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON signal
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL AND NEW.status = 'open'))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER voice_profile_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON voice_profile
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER web_form_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON web_form
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL AND NEW.state = 'active'))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER webhook_subscription_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON webhook_subscription
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL AND NEW.state = 'active'))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
CREATE TRIGGER weekly_plan_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON weekly_plan
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.status = 'open'))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_merged_into_user_id_fkey;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_merge_is_retired;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_removed_is_deactivated;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_does_not_merge_into_itself;
