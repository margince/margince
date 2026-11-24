// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package auditentity names operational audit subjects shared by writers and receipts.
package auditentity

// Maintenance subjects describe internal state or configuration, not CRM work.
const (
	KnowledgeDocument     = "knowledge_document"
	ModelRate             = "ai_model_rate"
	ForecastCaptureStatus = "forecast_capture_status"
	Tag                   = "tag"
	List                  = "list"
	// The administered catalogs of stage exit criteria and record roles.
	StageExitCriterion = "stage_exit_criterion"
	RecordRole         = "record_role"
)

// MaintenanceTypes excludes only declared upkeep; unknown subjects remain visible
// to diagnostics rather than silently claiming the receipt is complete.
func MaintenanceTypes() []string {
	return []string{KnowledgeDocument, ModelRate, ForecastCaptureStatus, Tag, List, StageExitCriterion, RecordRole}
}

// The after-image keys a list or tag audit row files a record link under.
const (
	ListMemberAdded   = "added"
	ListMemberRemoved = "removed"
	TagApplied        = "applied"
	TagRemoved        = "removed"
)

// UpkeepTypes are business subjects whose machine changes Home does not show.
// The maintainers decided so for catalogs, plans, drafts, approvals outside
// their own lane, matching work and consent cases. Each is named as its writer
// audits it.
func UpkeepTypes() []string {
	return []string{
		"product", "approval", "mail_draft", "sales_initiative", "weekly_plan", "weekly_plan_commitment",
		"sdr_handoff", "linkedin_connection", "partner_stage",
		"communication_suppression", "communication_instruction", "communication_review",
		"data_subject_request", "privacy_notice_case", "privacy_request_proof",
		"lead_privacy_notice", "existing_customer_notice",
	}
}
