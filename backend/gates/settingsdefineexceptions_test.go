// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The reasons a key may still be declared with settings.Define. A module that
// converts deletes its lines, so the list ends holding only real exceptions.
const (
	// readBySQL is why a boot value cannot be honoured. SQL reads the row,
	// and a boot value it never sees would split Go from SQL.
	readBySQL = "SQL views, functions and raw statements read this row directly, so a boot value would split Go from SQL"
	// pendingState is product bookkeeping an operator has no reason to set.
	pendingState = "product-written pending state for a scheduled window change, not configuration"
	// productState is a record the product writes of what it did.
	productState = "product-written state, not configuration"
	// bootFieldsApart is a stored value whose boot source is several separate
	// settings, which one stored setting cannot name. Compose locks it while
	// they set it.
	bootFieldsApart = "its boot source is several per-field settings one stored setting cannot carry; compose locks it while they set it"
	// keyedByUnit is a map whose keys the composed units decide, which no
	// boot setting can name.
	keyedByUnit = "one entry per composed unit, keyed by the unit's name; a boot setting cannot name keys the build decides"
)

// gatekit:fixture the settings.Define keys a module may still declare, and why: expected exceptions, not a waived cost
var settingsDefineExceptions = map[string]string{
	"installation.base_currency":           readBySQL,
	"installation.timezone":                readBySQL,
	"installation.agent_window_next_hours": pendingState,
	"installation.agent_window_next_from":  pendingState,

	"capture.google_app":    bootFieldsApart,
	"capture.microsoft_app": bootFieldsApart,
	"capture.unit_apps":     keyedByUnit,

	"identity.extension_overwrites_edited_fields": keyedByUnit,
	"identity.extension_setting_values":           keyedByUnit,

	"ai.price_sync_last_run":         productState,
	"privacy.retention_floor_carry":  productState,
	"installation.smtp_password_ref": productState,
	"installation.license_token_ref": productState,
}
