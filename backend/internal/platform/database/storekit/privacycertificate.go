// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// PhysicalErasureCertificateSQL excludes holds that never certified destruction.
// The alias and actions expression are SQL literals from the caller.
func PhysicalErasureCertificateSQL(alias, actions string) string {
	return SQLf(`%[1]s.action=ANY(%[2]s) AND %[1]s.before IS NULL AND %[1]s.after IS NULL
 AND (%[1]s.evidence ? 'cause' OR %[1]s.evidence ? 'retention_action' OR %[1]s.evidence ? 'emails_suppressed')`, alias, actions)
}
