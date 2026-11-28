// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/retirementblockers"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	memberScopeColumn    = "scope_id"
	memberSourceArgument = "source_id"
	referenceRetain      = "retain"
	referenceHandover    = "handover"
	referenceResolve     = "resolve"
	memberLiveRows       = "archived_at IS NULL"
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
	"activity_reminder_delivery.delivered_to":  {disposition: referenceRetain},
	"activity.source_author_id":                {disposition: referenceRetain},
	"activity_participant.roster_seat":         {disposition: referenceRetain},
	"activity_participant.user_id":             {disposition: referenceRetain},
	"activity_reader_state.reader_id":          {disposition: referenceRetain},
	"activity_link_suggestion.for_user":        {disposition: referenceRetain},
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
	"auth_token.counterpart_user_id":           {disposition: referenceRetain},
	"auth_token.user_id":                       {disposition: referenceRetain},
	"automation.owner_id":                      {disposition: referenceResolve, live: `enabled AND archived_at IS NULL`, remedy: "automations"},
	"booking_page.host_user_id":                {disposition: referenceResolve, live: jsonTrue, remedy: "booking_pages"},
	"booking_alias.host_user_id":               {disposition: referenceRetain},
	"brief_run.user_id":                        {disposition: referenceRetain},
	"capture_alias_refused.user_id":            {disposition: referenceRetain},
	"capture_alias_sighting.user_id":           {disposition: referenceRetain},
	"capture_connection.backup_owner_id":       {disposition: referenceRetain},
	"capture_connection.connected_by":          {disposition: referenceRetain},
	"capture_connection.user_id":               {disposition: referenceRetain},
	"capture_counterparty_hold.user_id":        {disposition: referenceRetain},
	"capture_exclusion.user_id":                {disposition: referenceRetain},
	"capture_party_source.user_id":             {disposition: referenceRetain},
	"capture_import.user_id":                   {disposition: referenceRetain},
	"capture_owner_identity.user_id":           {disposition: referenceRetain},
	"capture_owner_identity_withdrawn.user_id": {disposition: referenceRetain},
	"capture_pending_counterparty.owner_id":    {disposition: referenceRetain},
	"capture_sender_override.user_id":          {disposition: referenceRetain},
	"capture_thread_verdict.user_id":           {disposition: referenceRetain},
	"channel_connection.connected_by":          {disposition: referenceRetain},
	"channel_connection.owner_user_id":         {disposition: referenceRetain},
	"chat_member_grant.user_id":                {disposition: referenceRetain},
	"chat_send_attempt.user_id":                {disposition: referenceRetain},
	"chat_link.linked_by":                      {disposition: referenceRetain},
	"comms_outbound.user_id":                   {disposition: referenceRetain},
	"communication_instruction.directed_by":    {disposition: referenceRetain},
	"communication_instruction.revoked_by":     {disposition: referenceRetain},
	"communication_review.initiated_by":        {disposition: referenceRetain},
	"company.owner_id":                         {disposition: referenceHandover, live: memberLiveRows},
	"company.source_author_id":                 {disposition: referenceRetain},
	"company_domain_disposition.owner_id":      {disposition: referenceRetain},
	"company_scan.user_id":                     {disposition: referenceRetain},
	"contact.owner_id":                         {disposition: referenceHandover, live: memberLiveRows},
	"contact.source_author_id":                 {disposition: referenceRetain},
	"contact_moment_dismissal.user_id":         {disposition: referenceRetain},
	"conversation_claim.corrected_by_user_id":  {disposition: referenceRetain},
	"conversation_claim.taken_by_user_id":      {disposition: referenceRetain},
	"custom_field.created_by":                  {disposition: referenceRetain},
	"data_subject_request.assignee_id":         {disposition: referenceResolve, live: `status IN ('open','in_progress')`, remedy: "data_requests"},
	"deal.owner_id":                            {disposition: referenceHandover, live: memberLiveRows},
	"deal.source_author_id":                    {disposition: referenceRetain},
	"deal_move_dismissal.user_id":              {disposition: referenceRetain},
	"deal_room.steward_user_id":                {disposition: referenceResolve, live: `archived_at IS NULL AND state NOT IN ('closed','expired') AND (expires_at IS NULL OR expires_at > now())`, remedy: "deal_rooms"},
	"deal_room_comment.author_user_id":         {disposition: referenceRetain},
	"deal_room_participant.invited_by":         {disposition: referenceRetain},
	"deal_room_thread.author_user_id":          {disposition: referenceRetain},
	"deal_room_thread.resolved_by_user_id":     {disposition: referenceRetain},
	"deal_stage_evidence.refuted_by":           {disposition: referenceRetain},
	"dedupe_candidate.disposed_by":             {disposition: referenceRetain},
	"email_signature.owner_id":                 {disposition: referenceRetain},
	"extension_secret.user_id":                 {disposition: referenceRetain},
	"federated_identity.user_id":               {disposition: referenceRetain},
	"forecast_call.author_id":                  {disposition: referenceRetain},
	"generated_summary.user_id":                {disposition: referenceRetain},
	"graph_interaction_edge.user_id":           {disposition: referenceRetain},
	"intro_request.introducer_user_id":         {disposition: referenceResolve, live: `archived_at IS NULL AND closed_at IS NULL`, remedy: "introductions"},
	"intro_request.requester_user_id":          {disposition: referenceRetain},
	"intro_request.suggested_user_id":          {disposition: referenceRetain},
	"lawful_basis_confirmation.confirmed_by":   {disposition: referenceRetain},
	"lead.owner_id":                            {disposition: referenceHandover, live: memberLiveRows},
	"lead.source_author_id":                    {disposition: referenceRetain},
	"lead_manual_signal.set_by":                {disposition: referenceRetain},
	"legal_entity_member.user_id":              {disposition: referenceRetain},
	"legal_review.decided_by":                  {disposition: referenceRetain},
	"legal_review.requested_by":                {disposition: referenceRetain},
	"linkedin_account.user_id":                 {disposition: referenceRetain},
	"linkedin_connection.owner_user_id":        {disposition: referenceRetain},
	"list.owner_id":                            {disposition: referenceHandover, live: memberLiveRows},
	"list.steward_id":                          {disposition: referenceHandover, live: memberLiveRows},
	"list_visit.user_id":                       {disposition: referenceRetain},
	"mail_draft.author_id":                     {disposition: referenceHandover, live: jsonTrue},
	"meeting_calendar_episode.host_user_id":    {disposition: referenceRetain},
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
	"privacy_notice_case.owner_user_id":        {disposition: referenceResolve, live: `state IN ('open','assigned','queued','delivery_failed','blocked','source_review')`, remedy: "privacy_notices"},
	"privacy_notice_case.resolved_by":          {disposition: referenceRetain},
	"project.owner_id":                         {disposition: referenceHandover, live: memberLiveRows},
	"project.source_author_id":                 {disposition: referenceRetain},
	"provider_connection.connected_by":         {disposition: referenceRetain},
	"provider_run.requested_by":                {disposition: referenceRetain},
	"record_assignment.user_id":                {disposition: referenceHandover, live: memberLiveRows},
	"record_grant.granted_by":                  {disposition: referenceRetain},
	"relationship_nudge_dismissal.reader_id":   {disposition: referenceRetain},
	"report_definition.owner_id":               {disposition: referenceResolve, live: memberLiveRows, remedy: "reports"},
	"report_definition_revision.created_by":    {disposition: referenceRetain},
	"report_edition.owner_id":                  {disposition: referenceRetain},
	"report_execution.owner_id":                {disposition: referenceRetain},
	"report_run.asked_by":                      {disposition: referenceRetain},
	"report_schedule.owner_id":                 {disposition: referenceResolve, live: `enabled`, remedy: "report_schedules"},
	"reporting_framework_revision.created_by":  {disposition: referenceRetain},
	"role_assignment.user_id":                  {disposition: referenceRetain},
	"sales_goal_plan.owner_id":                 {disposition: referenceRetain},
	"sales_goal_plan_revision.created_by":      {disposition: referenceRetain},
	"sales_initiative.owner_id":                {disposition: referenceResolve, live: jsonTrue, remedy: "reports"},
	"sales_target_revision.created_by":         {disposition: referenceRetain},
	"saved_view.owner_id":                      {disposition: referenceRetain, live: memberLiveRows},
	"scheduled_send.agent_on_behalf_of":        {disposition: referenceRetain},
	"scheduled_send.scheduled_by":              {disposition: referenceResolve, live: `status IN ('scheduled','held','sending')`, remedy: "scheduled_mail"},
	"sdr_handoff.assigned_to":                  {disposition: referenceHandover, live: `status = 'submitted'`},
	"sdr_handoff.submitted_by":                 {disposition: referenceRetain},
	"session.user_id":                          {disposition: referenceRetain},
	"signal.owner_id":                          {disposition: referenceHandover, live: `archived_at IS NULL AND status = 'open'`},
	"signal_resolution.resolved_by":            {disposition: referenceRetain},
	"suggestion_dismissal.user_id":             {disposition: referenceRetain},
	"system_log.on_behalf_of":                  {disposition: referenceRetain},
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
	"home_read_cache.reader_id":                {disposition: referenceRetain},
	"home_read_forgotten.reader_id":            {disposition: referenceRetain},
	"task_explanation.user_id":                 {disposition: referenceRetain},
}

// memberRetirementBlockers reads every resolve reference to the member. A record
// link is built on the origin the call bound, so both doors link alike.
func memberRetirementBlockers(ctx context.Context, tx pgx.Tx, source ids.UUID) (retirementblockers.Found, error) {
	refs := []retirementblockers.Reference{}
	for _, ref := range slices.Sorted(maps.Keys(memberReferences)) {
		policy := memberReferences[ref]
		switch policy.disposition {
		case referenceRetain, referenceHandover:
			continue
		case referenceResolve:
		default:
			return retirementblockers.Found{}, fmt.Errorf("unknown member-reference policy %q", ref)
		}
		table, column, ok := strings.Cut(ref, ".")
		if !ok {
			return retirementblockers.Found{}, fmt.Errorf("invalid member-reference policy %q", ref)
		}
		refs = append(refs, retirementblockers.Reference{Table: table, Column: column, Live: policy.live, Blocker: policy.remedy})
	}
	return retirementblockers.Find(ctx, tx, source, refs, agents.RecordLinkOrigin(ctx))
}
