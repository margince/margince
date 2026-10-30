// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	memberScopeColumn    = "scope_id"
	memberSourceArgument = "source_id"
)

const (
	referenceRetain   = "retain"
	referenceHandover = "handover"
	referenceResolve  = "resolve"
	memberLiveRows    = "archived_at IS NULL"
)

type memberReference struct {
	disposition, live string
	remedy            crmcontracts.MemberRetirementBlocker
	guardColumn       string
}

// Historical attribution and personal settings remain on the retired identity;
// active work must move through its writer or be resolved before retirement.
var memberReferences = map[string]memberReference{
	"app_user.merged_into_user_id":             {disposition: referenceRetain},
	"activity.assignee_id":                     {disposition: referenceHandover, live: `archived_at IS NULL AND kind = 'task' AND NOT is_done`},
	"activity.host_user_id":                    {disposition: referenceRetain},
	"activity.invitation_sender":               {disposition: referenceRetain},
	"activity.sent_by_colleague":               {disposition: referenceRetain},
	"activity.source_author_id":                {disposition: referenceRetain},
	"activity_participant.roster_seat":         {disposition: referenceRetain},
	"activity_participant.user_id":             {disposition: referenceRetain},
	"activity_reader_state.reader_id":          {disposition: referenceRetain},
	"activity_retention_evidence.decided_by":   {disposition: referenceRetain},
	"agent_standing_grant.user_id":             {disposition: referenceRetain},
	"ai_task_run.actor_user_id":                {disposition: referenceRetain},
	"analytics_share.created_by":               {disposition: referenceRetain},
	"analytics_share.scope_user_id":            {disposition: referenceResolve, live: `scope_kind = 'owner' AND revoked_at IS NULL AND expires_at > now()`, remedy: "analytics_shares", guardColumn: memberScopeColumn},
	"approval.decided_by":                      {disposition: referenceRetain},
	"approval.on_behalf_of":                    {disposition: referenceRetain},
	"approval.requested_decider_id":            {disposition: referenceResolve, live: `status = 'pending'`, remedy: "approval_decisions"},
	"approval.staged_in_conversation_of":       {disposition: referenceRetain},
	"approval_autonomy_policy.user_id":         {disposition: referenceRetain},
	"approval_preference.user_id":              {disposition: referenceRetain},
	"assurance_resolution.actor_id":            {disposition: referenceRetain},
	"audit_log.on_behalf_of":                   {disposition: referenceRetain},
	"auth_token.user_id":                       {disposition: referenceRetain},
	"automation.owner_id":                      {disposition: referenceResolve, live: `enabled AND archived_at IS NULL`, remedy: "automations"},
	"booking_page.host_user_id":                {disposition: referenceResolve, live: jsonTrue, remedy: "booking_pages"},
	"brief_run.user_id":                        {disposition: referenceRetain},
	"capture_alias_refused.user_id":            {disposition: referenceRetain},
	"capture_alias_sighting.user_id":           {disposition: referenceRetain},
	"capture_connection.user_id":               {disposition: referenceRetain},
	"capture_counterparty_hold.user_id":        {disposition: referenceRetain},
	"capture_digest.user_id":                   {disposition: referenceRetain},
	"capture_exclusion.user_id":                {disposition: referenceRetain},
	"capture_freemail_domain.created_by":       {disposition: referenceRetain},
	"capture_import.user_id":                   {disposition: referenceRetain},
	"capture_owner_identity.user_id":           {disposition: referenceRetain},
	"capture_owner_identity_withdrawn.user_id": {disposition: referenceRetain},
	"capture_pending_counterparty.owner_id":    {disposition: referenceRetain},
	"capture_sender_override.user_id":          {disposition: referenceRetain},
	"capture_thread_verdict.user_id":           {disposition: referenceRetain},
	"channel_connection.connected_by":          {disposition: referenceRetain},
	"comms_outbound.user_id":                   {disposition: referenceRetain},
	"communication_instruction.directed_by":    {disposition: referenceRetain},
	"communication_instruction.revoked_by":     {disposition: referenceRetain},
	"communication_review.initiated_by":        {disposition: referenceRetain},
	"company.owner_id":                         {disposition: referenceHandover, live: memberLiveRows},
	"company.source_author_id":                 {disposition: referenceRetain},
	"company_brief.user_id":                    {disposition: referenceRetain},
	"company_domain_disposition.owner_id":      {disposition: referenceRetain},
	"company_domain_question_reader.reader_id": {disposition: referenceRetain},
	"company_dossier.user_id":                  {disposition: referenceRetain},
	"company_growth_fit.user_id":               {disposition: referenceRetain},
	"company_scan.user_id":                     {disposition: referenceRetain},
	"contact.owner_id":                         {disposition: referenceHandover, live: memberLiveRows},
	"contact.source_author_id":                 {disposition: referenceRetain},
	"contact_brief.user_id":                    {disposition: referenceRetain},
	"contact_moment_dismissal.user_id":         {disposition: referenceRetain},
	"conversation_claim.corrected_by_user_id":  {disposition: referenceRetain},
	"conversation_claim.taken_by_user_id":      {disposition: referenceRetain},
	"custom_field.created_by":                  {disposition: referenceRetain},
	"data_subject_request.assignee_id":         {disposition: referenceResolve, live: `status IN ('open','in_progress')`, remedy: "data_requests"},
	"deal.owner_id":                            {disposition: referenceHandover, live: memberLiveRows},
	"deal.source_author_id":                    {disposition: referenceRetain},
	"deal_move_dismissal.user_id":              {disposition: referenceRetain},
	"deal_room.steward_user_id":                {disposition: referenceResolve, live: `archived_at IS NULL AND state NOT IN ('closed','expired')`, remedy: "deal_rooms"},
	"deal_room_comment.author_user_id":         {disposition: referenceRetain},
	"deal_room_participant.invited_by":         {disposition: referenceRetain},
	"deal_room_thread.author_user_id":          {disposition: referenceRetain},
	"deal_room_thread.resolved_by_user_id":     {disposition: referenceRetain},
	"deal_stage_evidence.refuted_by":           {disposition: referenceRetain},
	"deal_status_card.user_id":                 {disposition: referenceRetain},
	"deal_suggestion.decided_by":               {disposition: referenceRetain},
	"dedupe_candidate.disposed_by":             {disposition: referenceRetain},
	"email_signature.owner_id":                 {disposition: referenceRetain},
	"extension_secret.user_id":                 {disposition: referenceRetain},
	"federated_identity.user_id":               {disposition: referenceRetain},
	"forecast_call.author_id":                  {disposition: referenceRetain},
	"graph_interaction_edge.user_id":           {disposition: referenceRetain},
	"intro_request.introducer_user_id":         {disposition: referenceResolve, live: `archived_at IS NULL AND closed_at IS NULL`, remedy: "introductions"},
	"intro_request.requester_user_id":          {disposition: referenceRetain},
	"intro_request.suggested_user_id":          {disposition: referenceRetain},
	"lead.owner_id":                            {disposition: referenceHandover, live: memberLiveRows},
	"lead.source_author_id":                    {disposition: referenceRetain},
	"lead_manual_signal.set_by":                {disposition: referenceRetain},
	"legal_entity_member.user_id":              {disposition: referenceRetain},
	"legal_review.decided_by":                  {disposition: referenceRetain},
	"legal_review.requested_by":                {disposition: referenceRetain},
	"linkedin_account.user_id":                 {disposition: referenceRetain},
	"linkedin_connection.owner_user_id":        {disposition: referenceRetain},
	"list.owner_id":                            {disposition: referenceResolve, live: memberLiveRows, remedy: "lists"},
	"list.steward_id":                          {disposition: referenceResolve, live: memberLiveRows, remedy: "stewarded_lists"},
	"list_visit.user_id":                       {disposition: referenceRetain},
	"mail_draft.author_id":                     {disposition: referenceResolve, live: jsonTrue, remedy: "mail_drafts"},
	"meeting_invitation.host_user_id":          {disposition: referenceResolve, live: `status NOT IN ('canceled','erased') OR NOT cleanup_complete`, remedy: "meeting_invitations"},
	"meeting_proposal.host_user_id":            {disposition: referenceResolve, live: jsonTrue, remedy: "meeting_proposals"},
	"meeting_readiness.attendee_id":            {disposition: referenceRetain},
	"notice.recipient_user_id":                 {disposition: referenceRetain},
	"notification_digest_run.user_id":          {disposition: referenceRetain},
	"notification_preference.user_id":          {disposition: referenceRetain},
	"oauth_authorization_code.user_id":         {disposition: referenceRetain},
	"oauth_grant.user_id":                      {disposition: referenceRetain},
	"onboarding_wizard_state.user_id":          {disposition: referenceRetain},
	"passport.granted_by":                      {disposition: referenceRetain},
	"passport.on_behalf_of":                    {disposition: referenceRetain},
	"privacy_notice_case.owner_user_id":        {disposition: referenceResolve, live: `state IN ('open','assigned','queued','delivery_failed','blocked')`, remedy: "privacy_notices"},
	"privacy_notice_case.resolved_by":          {disposition: referenceRetain},
	"project.owner_id":                         {disposition: referenceHandover, live: memberLiveRows},
	"project.source_author_id":                 {disposition: referenceRetain},
	"provider_connection.connected_by":         {disposition: referenceRetain},
	"provider_run.requested_by":                {disposition: referenceRetain},
	"record_assignment.user_id":                {disposition: referenceResolve, live: memberLiveRows, remedy: "record_responsibilities"},
	"record_grant.granted_by":                  {disposition: referenceRetain},
	"relationship_nudge_dismissal.reader_id":   {disposition: referenceRetain},
	"report_definition.owner_id":               {disposition: referenceResolve, live: memberLiveRows, remedy: "reports"},
	"report_definition_revision.created_by":    {disposition: referenceRetain},
	"report_execution.owner_id":                {disposition: referenceRetain},
	"report_run.asked_by":                      {disposition: referenceRetain},
	"report_schedule.owner_id":                 {disposition: referenceResolve, live: `enabled`, remedy: "report_schedules"},
	"reporting_framework_revision.created_by":  {disposition: referenceRetain},
	"role_assignment.user_id":                  {disposition: referenceRetain},
	"sales_target_revision.created_by":         {disposition: referenceRetain},
	"saved_view.owner_id":                      {disposition: referenceResolve, live: memberLiveRows, remedy: "saved_views"},
	"scheduled_send.agent_on_behalf_of":        {disposition: referenceRetain},
	"scheduled_send.scheduled_by":              {disposition: referenceResolve, live: `status IN ('scheduled','held','sending')`, remedy: "scheduled_mail"},
	"sdr_handoff.assigned_to":                  {disposition: referenceResolve, live: `status IN ('submitted','accepted')`, remedy: "handoffs"},
	"sdr_handoff.submitted_by":                 {disposition: referenceRetain},
	"session.user_id":                          {disposition: referenceRetain},
	"signal.owner_id":                          {disposition: referenceHandover, live: `archived_at IS NULL AND status = 'open'`},
	"signal_resolution.resolved_by":            {disposition: referenceRetain},
	"stage_progression_outcome.reversed_by":    {disposition: referenceRetain},
	"stage_progression_policy.enabled_by":      {disposition: referenceRetain},
	"suggestion_dismissal.user_id":             {disposition: referenceRetain},
	"system_log.on_behalf_of":                  {disposition: referenceRetain},
	"tag_suggestion.decided_by":                {disposition: referenceRetain},
	"team_membership.user_id":                  {disposition: referenceRetain},
	"user_mfa.user_id":                         {disposition: referenceRetain},
	"user_record_view.user_id":                 {disposition: referenceRetain},
	"voice_build.requested_by":                 {disposition: referenceRetain},
	"voice_profile.owner_id":                   {disposition: referenceResolve, live: memberLiveRows, remedy: "voice_profiles"},
	"web_form.owner_id":                        {disposition: referenceResolve, live: `archived_at IS NULL AND state = 'active'`, remedy: "web_forms"},
	"webhook_subscription.owner_id":            {disposition: referenceResolve, live: `archived_at IS NULL AND state = 'active'`, remedy: "webhooks"},
	"weekly_plan.owner_id":                     {disposition: referenceResolve, live: `status = 'open'`, remedy: "weekly_plans"},
	"weekly_plan_commitment.manager_user_id":   {disposition: referenceRetain},
	"weekly_review.user_id":                    {disposition: referenceRetain},
	"worklist_done.reader_id":                  {disposition: referenceRetain},
	"worklist_pin.reader_id":                   {disposition: referenceRetain},
	"worklist_snapshot.reader_id":              {disposition: referenceRetain},
}

func memberRetirementBlockers(ctx context.Context, tx pgx.Tx, source ids.UUID) ([]crmcontracts.MemberRetirementBlocker, error) {
	blockers := []crmcontracts.MemberRetirementBlocker{}
	for ref, policy := range memberReferences {
		switch policy.disposition {
		case referenceRetain, referenceHandover:
			continue
		case referenceResolve:
		default:
			return nil, fmt.Errorf("unknown member-reference policy %q", ref)
		}
		table, column, ok := strings.Cut(ref, ".")
		if !ok {
			return nil, fmt.Errorf("invalid member-reference policy %q", ref)
		}
		args := []any{source}
		query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE %s = $%d AND (%s))", pgx.Identifier{table}.Sanitize(), pgx.Identifier{column}.Sanitize(), len(args), policy.live)
		var remains bool
		if err := tx.QueryRow(ctx, query, args...).Scan(&remains); err != nil {
			return nil, err
		}
		if remains {
			blockers = append(blockers, policy.remedy)
		}
	}
	slices.Sort(blockers)
	return blockers, nil
}
