// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Where domain logic branches on a typed Go enum, its constant set equals the
// schema's CHECK set for the column it mirrors. A typo'd Go literal compiles
// and misbehaves silently, and a Go set drifting from the CHECK 500s at insert.
// The registry below names each pair. Both sets are derived from the head
// catalog and the Go const declarations, never restated here.
// vocabularycensus_test.go finds a pair that has no line here yet.

import (
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// enumBindings maps "table.column" to the Go type that mirrors it.
//
// The entity vocabulary is restated by a dozen polymorphic columns, so it
// is bound here rather than grepped: adding a record type means widening
// one Go const set and every CHECK that mirrors it, and this gate is what
// makes widening eleven of twelve a failure instead of a silent half-job.
// Two sets, because they are genuinely two vocabularies — a reference TO a
// record (datasource.RecordType) never names an activity, while a thing
// hung OFF an object (datasource.EntityType) can.
var enumBindings = map[string]struct{ pkgDir, typeName string }{
	// Which check the ingress grammar refused a record on. The core writes the
	// class and the column holds the vocabulary, so a class Go learned and the
	// schema did not must fail the insert rather than land a value nothing can
	// name. The published surface is where the type lives, because a unit reads
	// the same class off its Result.
	"extension_ingest_refusal.refusal":           {"pkg/extension", "RecordRefusal"},
	"lead.status":                                {"internal/modules/contacts", "LeadStatus"},
	"deal.status":                                {"internal/modules/deals", "DealStatus"},
	"app_user.avatar_origin":                     {"internal/modules/identity", "AvatarOrigin"},
	"ai_model_rate.source":                       {"internal/modules/ai", "RateSource"},
	"stage.semantic":                             {"internal/modules/deals", "StageSemantic"},
	"partner_stage.meaning":                      {"internal/contracts", "PartnerStageMeaning"},
	"contact_consent.state":                      {"internal/modules/consent", "ConsentState"},
	"communication_suppression.recorded_through": {"internal/shared/ports/commsauthz", "RecordedThrough"},
	"consent_purpose.class":                      {"internal/modules/consent", "Class"},
	"offer_line_item.proposal_state":             {"internal/modules/deals", "ProposalState"},
	"stage_exit_criterion.kind":                  {"internal/modules/deals", "CriterionKind"},
	"knowledge_document.ingest_status":           {"internal/contracts", "KnowledgeDocumentIngestStatus"},
	// The tag palette is spelled in five places: this CHECK, the contract enum,
	// the MCP tool schemas, the frontend's TAG_TONES and its dot stylesheet.
	// Widening four of the five is SILENT — the pill drops a tone it does not
	// know, so a half-widened palette reaches a reader as a tag with no dot,
	// which is exactly how an uncoloured tag looks.
	"tag.color": {"internal/contracts", "TagColor"},
	// Where a saved draft was opened. The wire enum is what the composer sends,
	// so a value the contract admits and the CHECK refuses would be a save that
	// fails with a constraint error instead of a validation answer.
	"mail_draft.anchor_type": {"internal/contracts", "MailDraftAnchorType"},
	// The reason a draft keeps. A reason the contract admits and the CHECK
	// refuses would lose an autosave to a constraint error.
	"mail_draft.communication_context": {"internal/contracts", "CommunicationContext"},
	// How to read an activity's time. A precision the contract admits and the
	// CHECK refuses would be a 500 on a valid create.
	"activity.occurred_precision": {"internal/contracts", "OccurredPrecision"},

	"activity_link.entity_type": {"internal/shared/ports/datasource", "RecordType"},
	"list.entity_type":          {"internal/shared/ports/datasource", "RecordType"},
	"list_member.entity_type":   {"internal/shared/ports/datasource", "RecordType"},
	"taggable.entity_type":      {"internal/shared/ports/datasource", "RecordType"},
	"record_grant.record_type":  {"internal/shared/ports/datasource", "RecordType"},

	// A suggested link names a record kind and a state a reader can act on. A
	// value the contract admits and the CHECK refuses is a 500 on a valid answer.
	"activity_link_suggestion.entity_type": {"internal/contracts", "LinkSuggestionEntityType"},
	"activity_link_suggestion.status":      {"internal/contracts", "LinkSuggestionStatus"},
	"signal.entity_type":                   {"internal/contracts", "SignalEntityType"},
	"signal.kind":                          {"internal/contracts", "ListSignalsParamsKind"},
	"record_link.from_type":                {"internal/contracts", "LinkedRecordType"},
	"record_link.to_type":                  {"internal/contracts", "LinkedRecordType"},

	"attachment.entity_type":       {"internal/shared/ports/datasource", "EntityType"},
	"embedding.entity_type":        {"internal/shared/ports/datasource", "EntityType"},
	"field_provenance.object_type": {"internal/shared/ports/datasource", "EntityType"},
	// NOT EntityType, and this is the one binding where the difference is the
	// point. A custom field hangs off a TARGET; an EntityType is something a
	// record provider can be asked about. Contract is the first member that is
	// the former without being the latter, so the CHECK mirrors
	// fieldcatalog.Target and the gate compares it against that.
	"custom_field.object": {"internal/shared/ports/fieldcatalog", "Target"},

	// The CONTRACT-BACKED columns: a column whose CHECK is mirrored by a named
	// schema in crm.yaml, which the generator turns into a Go const set.
	//
	// These are the instance #1496 names — "a contract enum wider than the
	// constraint behind it" — and what makes them different from the bindings
	// above is where the drift hurts. A Go set drifting from its CHECK 500s at
	// insert, which is loud. A CONTRACT set drifting wider is quiet and worse:
	// the document promises a value, a caller sends it, and the database
	// refuses a request the published contract said was well-formed.
	//
	// Bound by hand, because a match by value alone is often coincidence:
	// MeetingPlanTier and assurance_exception.severity share [high, low, medium]
	// and mean different things. The census finds every value match; this line
	// records which type the column actually means.
	"activity.audience":                       {"internal/contracts", "ActivityAudience"},
	"activity.task_type":                      {"internal/contracts", "TaskType"},
	"capture_counterparty_hold.kind":          {"internal/contracts", "CaptureCounterpartyHoldKind"},
	"capture_exclusion.kind":                  {"internal/contracts", "CaptureExclusionKind"},
	"capture_exclusion.scope":                 {"internal/contracts", "CaptureExclusionScope"},
	"capture_owner_identity.kind":             {"internal/contracts", "CaptureOwnerIdentityKind"},
	"capture_owner_identity.source":           {"internal/contracts", "CaptureOwnerIdentitySource"},
	"capture_owner_identity_withdrawn.kind":   {"internal/contracts", "CaptureOwnerIdentityKind"},
	"capture_owner_identity_withdrawn.source": {"internal/contracts", "CaptureOwnerIdentitySource"},
	"commission_entry.status":                 {"internal/contracts", "CommissionStatus"},
	"conversation_claim.kind":                 {"internal/contracts", "ConversationClaimKind"},
	"deal_room.state":                         {"internal/contracts", "DealRoomState"},
	"deal_stage_evidence.author_side":         {"internal/contracts", "StageEvidenceAuthorSide"},
	"deal_stage_evidence.commitment":          {"internal/contracts", "StageEvidenceCommitment"},
	"deal_stage_evidence.source_type":         {"internal/contracts", "StageEvidenceSource"},
	"import_run.status":                       {"internal/contracts", "ImportRunStatus"},
	"intro_request.fallback_policy":           {"internal/contracts", "IntroFallbackPolicy"},
	"intro_request.note_generated_by":         {"internal/contracts", "IntroNoteOrigin"},
	"intro_request.route_type":                {"internal/contracts", "ContactGraphRouteType"},
	"intro_request.status":                    {"internal/contracts", "IntroRequestStatus"},
	"lead_manual_signal.signal_kind":          {"internal/contracts", "LeadManualSignalKind"},
	"lead_source.intent":                      {"internal/contracts", "LeadSourceIntent"},
	"project_health_assessment.state":         {"internal/contracts", "ProjectHealthState"},
	"provider_connection.mode":                {"internal/contracts", "ProviderConnectionMode"},
	"provider_connection.status":              {"internal/contracts", "ProviderConnectionStatus"},
	"retention_policy.action":                 {"internal/contracts", "RetentionAction"},

	// One vocabulary, six columns. Every derived narrative row records whether a
	// human template or a model wrote it, and WrittenBy is the word the contract
	// uses for that everywhere, so widening the contract without widening all
	// six CHECKs fails here rather than at the first insert of the new value.
	//
	// A seventh narrative table needs its OWN line: this registry is keyed by
	// table.column and has no wildcard, so reusing the vocabulary is free and
	// being bound to it is not. That is the hand-kept cost #1496 names, and it
	// is worth paying here for the reason the block above gives — which pairs
	// are real is a judgement, and a wildcard would bind the next column called
	// generated_by whether or not it holds this vocabulary.
	"company_brief.generated_by":      {"internal/contracts", "WrittenBy"},
	"company_dossier.generated_by":    {"internal/contracts", "WrittenBy"},
	"company_growth_fit.generated_by": {"internal/contracts", "WrittenBy"},
	"company_scan.generated_by":       {"internal/contracts", "WrittenBy"},
	"contact_brief.generated_by":      {"internal/contracts", "WrittenBy"},
	"deal_status_card.generated_by":   {"internal/contracts", "WrittenBy"},

	// Found by the vocabulary census in vocabularycensus_test.go. Each type is the one
	// whose name says what the column means, so a match by value alone did not decide it.
	"activity.body_completeness":                   {"internal/contracts", "BodyCompleteness"},
	"activity.calendar_method":                     {"internal/contracts", "PublicEventActivityChangedFieldsCalendarMethod"},
	"activity.owed_verdict":                        {"internal/contracts", "HomeRepairTaskAfterVerdict"},
	"activity_audience_member.subject_type":        {"internal/contracts", "RecordGrantSubjectType"},
	"activity_calendar_method_backfill.direction":  {"internal/contracts", "ActivityDirection"},
	"activity_meeting_history.status":              {"internal/contracts", "ActivityMeetingStatus"},
	"activity_participant.response":                {"internal/contracts", "MeetingResponse"},
	"activity_reader_state.reopen_on":              {"internal/shared/kernel/values", "ReopenCondition"},
	"activity_review_response.outcome":             {"internal/contracts", "ActivityReviewTemplateOutcome"},
	"agent_task.status":                            {"internal/modules/agents", "TaskStatus"},
	"ai_feedback.claim_kind":                       {"internal/contracts", "AIFeedbackInputClaimKind"},
	"ai_feedback.subject_type":                     {"internal/contracts", "AIFeedbackInputSubjectType"},
	"ai_feedback.verdict":                          {"internal/contracts", "AIFeedbackInputVerdict"},
	"analytics_share.kind":                         {"internal/contracts", "ForecastShareKind"},
	"analytics_share.scope_kind":                   {"internal/contracts", "ForecastCallScopeKind"},
	"app_user.locale":                              {"internal/contracts", "SaveMyLocaleRequestLocale"},
	"app_user.morning_brief_delivery":              {"internal/contracts", "BriefDeliveryMorningBriefDelivery"},
	"app_user.routine_mail_scope":                  {"internal/contracts", "BriefDeliveryRoutineMailScope"},
	"app_user.seat_type":                           {"internal/shared/kernel/principal", "SeatType"},
	"app_user.status":                              {"internal/contracts", "UserStatus"},
	"app_user.weekly_delivery":                     {"internal/contracts", "BriefDeliveryWeeklyDelivery"},
	"approval.requested_decider_source":            {"internal/modules/approvals", "deciderSource"},
	"approval_autonomy_policy.mode":                {"internal/contracts", "KindAutonomyMode"},
	"assurance_deal_receipt.outcome":               {"internal/shared/kernel/viability", "Outcome"},
	"assurance_exception.delivery_state":           {"internal/contracts", "DealViabilityReviewDeliveryState"},
	"assurance_exception.subject_kind":             {"internal/contracts", "InputCheckSubjectKind"},
	"assurance_exception.severity":                 {"internal/contracts", "AssuranceFindingCountSeverity"},
	"assurance_resolution.outcome":                 {"internal/contracts", "PublicEventForecastExceptionResolvedOutcome"},
	"assurance_run.readiness":                      {"internal/contracts", "ForecastAssuranceReadiness"},
	"assurance_source_coverage.source":             {"internal/contracts", "ForecastAssuranceSourceSource"},
	"assurance_source_coverage.state":              {"internal/contracts", "ForecastAssuranceSourceState"},
	"audit_log.action":                             {"internal/contracts", "AuditAction"},
	"audit_log.actor_type":                         {"internal/shared/kernel/principal", "PrincipalType"},
	"automation.tier":                              {"pkg/extension", "Tier"},
	"brief_item.reopen_on":                         {"internal/shared/kernel/values", "ReopenCondition"},
	"brief_item.state":                             {"internal/contracts", "MorningBriefItemState"},
	"bulk_operation.record_type":                   {"internal/contracts", "BulkRecordType"},
	"bulk_operation.verb":                          {"internal/contracts", "BulkVerb"},
	"capture_freemail_domain.kind":                 {"internal/contracts", "ConsumerMailDomainKind"},
	"capture_import.posture_at_import":             {"internal/contracts", "CaptureConnectionMailPosture"},
	"capture_pending_counterparty.status":          {"internal/contracts", "CaptureTraceResolutionStatus"},
	"capture_sender_override.decision":             {"internal/contracts", "CaptureSenderDecisionDecision"},
	"capture_sweep_run.sweep":                      {"internal/modules/capture", "Sweep"},
	"capture_sync_state.last_error_class":          {"internal/modules/capture", "errorClass"},
	"channel_provider.credential_model":            {"pkg/extension", "CredentialModel"},
	"commission_entry.attribution_at_accrual":      {"internal/contracts", "CommissionAttribution"},
	"comms_outbound.bounce_kind":                   {"internal/shared/ports/connector", "BounceKind"},
	"communication_decision.basis":                 {"internal/shared/ports/commsauthz", "Basis"},
	"communication_decision.mode":                  {"internal/shared/ports/commsauthz", "Mode"},
	"communication_decision.phase":                 {"internal/shared/ports/commsauthz", "Phase"},
	"communication_decision.resolved_category":     {"internal/shared/ports/commsauthz", "Category"},
	"communication_decision.verdict":               {"internal/shared/ports/commsauthz", "Verdict"},
	"communication_instruction.reason_code":        {"internal/contracts", "DirectCommunicationSendRequestReasonCode"},
	"communication_override.category":              {"internal/shared/ports/commsauthz", "Category"},
	"communication_suppression.decided_by_level":   {"internal/shared/ports/commsauthz", "AuthorityLevel"},
	"company.visibility":                           {"internal/contracts", "RecordAccessVisibility"},
	"company_domain_disposition.admission":         {"internal/contracts", "SetBlockedDomainRequestAdmission"},
	"company_offered_value.field_key":              {"internal/contracts", "CompanyOfferedValueField"},
	"company_relationship_type.relationship_type":  {"internal/contracts", "CompanyRelationshipTypes"},
	"company_technical_state.lane":                 {"internal/modules/contacts", "TechnicalLane"},
	"company_technical_state.last_outcome":         {"internal/contracts", "TechnicalEnrichLaneOutcome"},
	"contact.narrowing_reason":                     {"internal/modules/contacts", "NarrowingReason"},
	"contact.visibility":                           {"internal/contracts", "RecordAccessVisibility"},
	"contact_confirm_submission.resolution":        {"internal/contracts", "ConfirmSubmissionResolution"},
	"contact_provider_claim.claim_key":             {"internal/shared/ports/provider", "ClaimKey"},
	"data_task_run.status":                         {"internal/contracts", "DataTaskProgressStatus"},
	"deal_line_item.billing_model":                 {"internal/contracts", "ProductBillingModel"},
	"deal_line_item.component_kind":                {"internal/contracts", "ProductComponentKind"},
	"deal_line_item.tax_category":                  {"internal/contracts", "ProductTaxCategory"},
	"deal_product.component_kind":                  {"internal/contracts", "ProductComponentKind"},
	"deal_stage_history.semantic_at_change":        {"internal/modules/deals", "StageSemantic"},
	"dedupe_candidate.disposition":                 {"internal/contracts", "DedupeCandidateStatus"},
	"embed_store_binding.status":                   {"internal/contracts", "EmbedReindexStatusStatus"},
	"field_mask.condition":                         {"internal/shared/kernel/principal", "MaskCondition"},
	"forecast_contribution.viability_disposition":  {"internal/contracts", "DealViabilityDisposition"},
	"forecast_snapshot.scope_kind":                 {"internal/contracts", "ForecastCallScopeKind"},
	"forecast_snapshot.trigger":                    {"internal/contracts", "ForecastSnapshotRefTrigger"},
	"home_read_cache.read":                         {"internal/compose/homecache", "Read"},
	"import_record_map.origin":                     {"internal/modules/migration", "Origin"},
	"import_run.draft_step":                        {"internal/contracts", "ImportDraftStep"},
	"intro_request.origin":                         {"internal/modules/introductions", "Origin"},
	"intro_request.recorded_channel":               {"internal/contracts", "IntroRecordedChannel"},
	"lead_origin.intent":                           {"internal/contracts", "LeadSourceIntent"},
	"lead_privacy_notice.state":                    {"internal/contracts", "InternalEventLeadPrivacyNoticeCreatedState"},
	"list_evaluation.outcome":                      {"internal/contracts", "ListCheckOutcome"},
	"list_live_member.entity_type":                 {"internal/shared/ports/datasource", "RecordType"},
	"list_member_event.entity_type":                {"internal/shared/ports/datasource", "RecordType"},
	"list_member_event.reason":                     {"internal/contracts", "ListHistoryEntryReason"},
	"meeting_calendar_episode.provider":            {"internal/contracts", "MeetingInvitationProvider"},
	"notification_preference.delivery":             {"internal/contracts", "NotificationDelivery"},
	"oauth_client.created_via":                     {"internal/contracts", "AgentClientRegisteredBy"},
	"offer_line_item.billing_model":                {"internal/contracts", "ProductBillingModel"},
	"offer_line_item.tax_category":                 {"internal/contracts", "ProductTaxCategory"},
	"onboarding_wizard_state.path":                 {"internal/contracts", "OnboardingStatePath"},
	"onboarding_wizard_state.source_mode":          {"internal/contracts", "OnboardingStateSourceMode"},
	"onboarding_wizard_state.step":                 {"internal/contracts", "OnboardingStateStep"},
	"privacy_notice_case.rule":                     {"internal/modules/consent", "NoticeRule"},
	"privacy_notice_case.state":                    {"internal/modules/consent", "NoticeState"},
	"privacy_request_proof.request_kind":           {"internal/contracts", "DataSubjectRequestKind"},
	"raw_capture.parts_slim_outcome":               {"internal/modules/capture", "partSlimOutcome"},
	"report_definition.audience":                   {"internal/contracts", "ReportingReportAudience"},
	"report_edition_question.reason":               {"internal/contracts", "ReportingEditionQuestionReason"},
	"report_execution.status":                      {"internal/contracts", "ReportingExecutionStatus"},
	"sales_goal_plan_revision.kind":                {"internal/contracts", "SalesGoalEventKind"},
	"sales_goal_plan_revision.origin":              {"internal/contracts", "SalesGoalRevisionOrigin"},
	"sales_initiative_enrolment.record_type":       {"internal/contracts", "SalesInitiativeMemberRecordType"},
	"sales_initiative_member.record_type":          {"internal/contracts", "SalesInitiativeMemberRecordType"},
	"sales_target.period_kind":                     {"internal/contracts", "ReportingTargetInputPeriodKind"},
	"sales_target.scope_kind":                      {"internal/contracts", "ForecastCallScopeKind"},
	"sdr_handoff.status":                           {"internal/contracts", "HandoffAssigneeStatus"},
	"sdr_handoff_event.to_status":                  {"internal/contracts", "HandoffAssigneeStatus"},
	"sdr_handoff_reason.applies_to":                {"internal/contracts", "SdrHandoffReasonCountOutcome"},
	"signal.visibility":                            {"internal/contracts", "RecordAccessVisibility"},
	"site_read.phase":                              {"internal/contracts", "SiteReadReportPhase"},
	"site_read.status":                             {"internal/contracts", "SiteReadReportStatus"},
	"site_read.stopped_reason":                     {"internal/contracts", "SiteReadReportStoppedReason"},
	"stage_progression_policy.mode":                {"internal/contracts", "TransitionPolicyMode"},
	"system_log.actor_type":                        {"internal/shared/kernel/principal", "PrincipalType"},
	"taggable.assigned_by_kind":                    {"internal/contracts", "RecordTagAssignerKind"},
	"team_weekly_review_coverage.absent":           {"internal/contracts", "TeamWeeklyCoverageAbsent"},
	"team_weekly_review_coverage.basis":            {"internal/modules/forecasting", "SufficiencyBasis"},
	"team_weekly_review_coverage.period_kind":      {"internal/contracts", "TeamWeeklyCoveragePeriodKind"},
	"team_weekly_review_outlook.period_kind":       {"internal/modules/forecasting", "PeriodKind"},
	"team_weekly_review_rep.focus_kind":            {"internal/contracts", "TeamWeeklyRepFocusKind"},
	"team_weekly_review_rep_pattern.kind":          {"internal/contracts", "TeamWeeklyPatternKind"},
	"transcript_read.status":                       {"internal/contracts", "TranscriptReadReportStatus"},
	"user_record_view.entity_type":                 {"internal/contracts", "RecordViewAckEntityType"},
	"weekly_plan_commitment.linked_record_type":    {"internal/shared/ports/datasource", "RecordType"},
	"weekly_review_driver.period_kind":             {"internal/modules/forecasting", "PeriodKind"},
	"weekly_review_learning_citation.subject_type": {"internal/contracts", "WeeklyLearningCitationSubjectType"},
	"weekly_review_movement.period_kind":           {"internal/modules/forecasting", "PeriodKind"},
	"weekly_review_outlook.period_kind":            {"internal/modules/forecasting", "PeriodKind"},
}

// requestEnumBindings maps a create request's inline enum to the column it
// writes. The map above holds one type per column, and a route that decodes its
// own enum would otherwise widen apart from the row it stores.
var requestEnumBindings = []struct{ typeName, col string }{
	{"CreateCaptureCounterpartyHoldRequestKind", "capture_counterparty_hold.kind"},
	{"SalesGoalDraftInputOrigin", "sales_goal_plan_revision.origin"},
}

// checkInList captures CHECK (col IN ('a','b',…)) allowing an optional
// "col IS NULL OR" prefix; applied to a table block's accumulated text.
var checkInList = regexp.MustCompile(`(?is)CHECK\s*\(\s*(?:([a-z_]+)\s+IS\s+NULL\s+OR\s+)?([a-z_]+)\s+IN\s*\(([^)]*)\)`)

// alterTableStmt keys an ALTER statement's CHECK lists to their table.
var alterTableStmt = regexp.MustCompile(`(?is)ALTER\s+TABLE\s+([a-z_]+)`)

// singleQuoted pulls the 'value' literals out of a captured IN-list.
var singleQuoted = regexp.MustCompile(`'([^']*)'`)

// migrationPrefix is the numeric-prefix name shape last-wins ordering needs.
// Widths differ — custom/ stamps 14 digits, core/ ten since it started naming
// a migration for the unix second it was written, and four before that — and
// last-wins does not need them to agree, only to sort
// lexically-equals-chronologically within one namespace, which is all
// tableCheckSets' per-root walk relies on. core/'s closed four-digit sequence
// is zero-padded and so still sorts below its later stamps.
var migrationPrefix = regexp.MustCompile(`^\d{4,}_`)

// recordChecks derives every CHECK (col IN (…)) set in text and records
// it under table.col — the one spelling shared by the CREATE-block and
// ALTER-statement passes.
func recordChecks(sets map[string][]string, table, text string) {
	for _, m := range checkInList.FindAllStringSubmatch(text, -1) {
		var vals []string
		for _, q := range singleQuoted.FindAllStringSubmatch(m[3], -1) {
			vals = append(vals, q[1])
		}
		sort.Strings(vals)
		sets[table+"."+m[2]] = vals
	}
}

// tableCheckSets derives table.column → allowed set from the migration
// sources, using the same line-based CREATE TABLE scan as updateguard
// (column definitions nest parens beyond what a block regex can pair),
// plus a per-statement ALTER TABLE pass: vocabularies grow additively
// (drop CHECK + re-add wider), so the LAST migration to state a column's
// IN-list wins — walk order is lexical, which is migration order.
func tableCheckSets(t *testing.T) map[string][]string {
	t.Helper()
	sets := map[string][]string{}
	for _, root := range []string{"migrations/core", "migrations/custom"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
				return err
			}
			// Last-wins depends on lexical order being migration order:
			// every name must carry the numeric version prefix.
			if !migrationPrefix.MatchString(d.Name()) {
				return fmt.Errorf("%s: migration name lacks the numeric version prefix the lexical-order derivation relies on", path)
			}
			raw, err := gatekit.ReadFile(path) // #nosec G304 G122 -- path is a *.up.sql file from walking the trusted migrations tree
			if err != nil {
				return err
			}
			// Read through the renames the migrations themselves declare: a CHECK
			// against a table since renamed would otherwise be filed under a
			// name no caller uses, and the vocabulary derived for the current
			// one comes back empty.
			sql := withCurrentNames(string(raw))
			current, block := "", strings.Builder{}
			flush := func() {
				if current == "" {
					return
				}
				recordChecks(sets, current, block.String())
				current = ""
				block.Reset()
			}
			for line := range strings.SplitSeq(sql, "\n") {
				if m := createTableLine.FindStringSubmatch(line); m != nil {
					flush()
					current = m[1]
					continue
				}
				if strings.HasPrefix(line, ");") {
					flush()
					continue
				}
				if current != "" {
					block.WriteString(line)
					block.WriteString("\n")
				}
			}
			flush()
			for stmt := range strings.SplitSeq(sql, ";") {
				if alter := alterTableStmt.FindStringSubmatch(stmt); alter != nil {
					recordChecks(sets, alter[1], stmt)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return sets
}

// goConstSet derives the string values of every constant declared with
// the given type in the package directory.
func goConstSet(t *testing.T, pkgDir, typeName string) []string {
	t.Helper()
	var vals []string
	entries, err := gatekit.ReadDir(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(pkgDir, e.Name()), 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				id, ok := vs.Type.(*ast.Ident)
				if !ok || id.Name != typeName {
					continue
				}
				for _, v := range vs.Values {
					lit, ok := v.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					s, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatal(err)
					}
					vals = append(vals, s)
				}
			}
		}
	}
	sort.Strings(vals)
	return vals
}

// A saved view's resource is the CHECK's closed list plus the `ext:` spelling.
// The contract lists the closed part as an x-extensible-enum beside a pattern.
// The list must equal the CHECK's, and the pattern must admit it and an
// extension kind.
func TestSavedViewTargetIsTheResourceCheck(t *testing.T) {
	t.Parallel()
	want, ok := tableCheckSets(t)["saved_view.resource"]
	if !ok {
		t.Fatal("no CHECK (resource IN (…)) found for saved_view in the migrations — broken derivation")
	}
	spelling := spellingOf(contractSchema(t, "SavedViewTarget"))
	sort.Strings(spelling.Enum)
	if fmt.Sprint(spelling.Enum) != fmt.Sprint(want) {
		t.Errorf("SavedViewTarget lists %v but the schema CHECK stores %v — change both together", spelling.Enum, want)
	}
	pattern := regexp.MustCompile(spelling.Pattern)
	for _, resource := range append(spelling.Enum, "ext:finance-demo:invoice") {
		if !pattern.MatchString(resource) {
			t.Errorf("SavedViewTarget's pattern refuses %q", resource)
		}
	}
}

func TestEveryDomainEnumMatchesItsSchemaCheck(t *testing.T) {
	t.Parallel()
	checks, err := catalogVocabularies()
	if err != nil {
		t.Fatal(err)
	}
	type binding struct{ col, pkgDir, typeName string }
	var bindings []binding
	for col, bound := range enumBindings {
		bindings = append(bindings, binding{col, bound.pkgDir, bound.typeName})
	}
	for _, request := range requestEnumBindings {
		bindings = append(bindings, binding{request.col, "internal/contracts", request.typeName})
	}
	for _, binding := range bindings {
		col := binding.col
		want, ok := checks[col]
		if !ok {
			t.Errorf("enumBindings[%s]: no CHECK vocabulary found in %s — stale binding or broken derivation", col, vocabularyCatalog)
			continue
		}
		got := goConstSet(t, binding.pkgDir, binding.typeName)
		if len(got) == 0 {
			t.Errorf("enumBindings[%s]: no %s constants found in %s", col, binding.typeName, binding.pkgDir)
			continue
		}
		if got = asSet(got); setKey(got) != setKey(want) {
			t.Errorf("%s: Go %s set %v != schema CHECK set %v — change both together", col, binding.typeName, got, want)
		}
	}
}
