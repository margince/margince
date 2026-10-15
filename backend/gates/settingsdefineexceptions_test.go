// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The reasons a key may still be declared with settings.Define. A module that
// converts deletes its lines, so the list ends holding only real exceptions.
const (
	convertsIn12b = "converts in Task 12b"
	convertsIn12c = "converts in Task 12c"
	convertsIn12d = "converts in Task 12d"
	convertsIn13  = "converts in Task 13, with the other secrets"
	// readBySQL is why a boot value cannot be honoured. SQL reads the row,
	// and a boot value it never sees would split Go from SQL.
	readBySQL = "SQL views, functions and raw statements read this row directly, so a boot value would split Go from SQL"
	// pendingState is product bookkeeping an operator has no reason to set.
	pendingState = "product-written pending state for a scheduled window change, not configuration"
)

// gatekit:fixture the settings.Define keys a module may still declare, and why: expected exceptions, not a waived cost
var settingsDefineExceptions = map[string]string{
	"installation.base_currency":           readBySQL,
	"installation.timezone":                readBySQL,
	"installation.agent_window_next_hours": pendingState,
	"installation.agent_window_next_from":  pendingState,

	"installation.smtp_password_ref": convertsIn13,
	"installation.license_token_ref": convertsIn13,
	"ai.provider_keys":               convertsIn13,
	"capture.google_app":             convertsIn13,
	"capture.microsoft_app":          convertsIn13,

	"capture.auto_enrich":                convertsIn12b,
	"capture.auto_enrich_daily_cap":      convertsIn12b,
	"capture.mail_sharing":               convertsIn12b,
	"capture.mail_sync_interval_seconds": convertsIn12b,
	"capture.shared_posture_allowed":     convertsIn12b,
	"capture.signature_enrich":           convertsIn12b,
	"capture.site_read_max_mib":          convertsIn12b,
	"capture.site_read_max_pages":        convertsIn12b,
	"capture.site_read_wall_seconds":     convertsIn12b,
	"integrations.automatic_lookup":      convertsIn12b,

	"ai.budget":              convertsIn12c,
	"ai.price_sync":          convertsIn12c,
	"ai.price_sync_last_run": convertsIn12c,
	"ai.routing":             convertsIn12c,
	"ai.task_overrides":      convertsIn12c,

	"activities.follow_up_after_days":        convertsIn12d,
	"consent.authorization_modes":            convertsIn12d,
	"consent.controller_particulars":         convertsIn12d,
	"contacts.first_response_enabled":        convertsIn12d,
	"contacts.first_response_target_minutes": convertsIn12d,
	"contacts.signature_template":            convertsIn12d,
	"contacts.unassigned_escalation_user_id": convertsIn12d,
	"deals.legal_review_enabled":             convertsIn12d,
	"deals.maintenance_writes_enabled":       convertsIn12d,
	"deals.stage_autopilot_enabled":          convertsIn12d,
	"privacy.retain_only":                    convertsIn12d,
	"privacy.retention_floor_carry":          convertsIn12d,
}
