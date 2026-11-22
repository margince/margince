// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// StartAtCreation are the fragments that read the start an activity was
// created with, which a later reschedule or precision change moves.
type StartAtCreation struct {
	// Join reads the activity's audit trail once, as the lateral `created`.
	Join string
	// Start and Precision are the start and its precision at creation.
	Start, Precision string
}

// StartAtCreationSQL reads the activity under alias as it was created. The
// first audit row that changed the start holds the creation start in its
// before image, and the first that changed the precision holds that. Every
// change is audited, so a start no row changed is the one created.
func StartAtCreationSQL(alias string) StartAtCreation {
	first := func(field string) string {
		return `(array_agg(moved.before->>'` + field + `' ORDER BY moved.occurred_at,moved.id) FILTER (WHERE moved.before ? '` + field +
			`' AND moved.after ? '` + field + `' AND (moved.before->>'` + field + `') IS DISTINCT FROM (moved.after->>'` + field + `')))[1]`
	}
	return StartAtCreation{
		Join: `LEFT JOIN LATERAL (SELECT ` + first(auditFieldOccurredAt) + ` AS start,` + first(fieldOccurredPrecision) + ` AS precision
   FROM audit_log moved WHERE moved.entity_type='activity' AND moved.entity_id=` + alias + `.id AND moved.action<>'create') created ON true`,
		Start:     `COALESCE(CAST(created.start AS timestamptz),` + alias + `.occurred_at)`,
		Precision: `COALESCE(created.precision,` + alias + `.occurred_precision)`,
	}
}
