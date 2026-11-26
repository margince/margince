// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

const (
	cohortLinkedKey   = "cohort_linked"
	cohortPromotedKey = "cohort_promoted"
)

// CohortAuditSQL identifies the additive address-to-contact repair by its immutable audit payload.
func CohortAuditSQL(alias string) string {
	return `COALESCE((` + alias + `.entity_type='contact' AND ` + alias + `.action='update' AND ` + alias + `.after ? '` + cohortLinkedKey + `' AND ` + alias + `.after ? '` + cohortPromotedKey + `'),false)`
}
