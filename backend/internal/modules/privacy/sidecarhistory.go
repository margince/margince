// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// A company's profile fields and facts live in sidecar tables and are audited
// under the sidecar row's own id, so the company's own audit rows never name
// them. Both history reads join those rows back to the company here and show
// each as a change of the company field it states.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auditattribution"
)

const (
	sidecarProfileFieldTable = "company_profile_field"
	sidecarFactTable         = "company_fact"
	sidecarValueKey          = "value"
)

// sidecarSubjectCTE renders the `side` CTE: every audit row of the company's
// profile fields and facts, with the company field it was about. The empty
// string means the kind has no sidecars.
//
// The first two branches follow the live sidecar row to its company. The third
// finds every row of a removed fact, whose own row is gone: its removal row
// still names the company and field in its before-image.
func sidecarSubjectCTE(entityType string, anchorPos int) string {
	if entityType != entityTypeCompany {
		return ""
	}
	return fmt.Sprintf(`side AS (
			SELECT a.id AS audit_id, f.field AS side_field
			  FROM %[1]s f
			  JOIN audit_log a ON a.entity_type = '%[1]s' AND a.entity_id = f.id
			 WHERE f.company_id = $%[3]d
			UNION ALL
			SELECT a.id, f.field
			  FROM %[2]s f
			  JOIN audit_log a ON a.entity_type = '%[2]s' AND a.entity_id = f.id
			 WHERE f.company_id = $%[3]d
			UNION ALL
			SELECT a.id, removed.before ->> 'field'
			  FROM audit_log removed
			  JOIN audit_log a ON a.entity_type = '%[2]s' AND a.entity_id = removed.entity_id
			 WHERE removed.entity_type = '%[2]s' AND removed.action = 'delete'
			   AND removed.before ->> 'company_id' = $%[3]d::text
		)`, sidecarProfileFieldTable, sidecarFactTable, anchorPos)
}

// sidecarArmSQL is the record-history union arm over the `side` CTE, in the
// window's column order.
func sidecarArmSQL(where string, fetchPos int) string {
	return fmt.Sprintf(`
			SELECT`+recordAuditColumns+`,
				`+noEdgeSubjectColumns+`, s.side_field
			FROM side s
			JOIN audit_log a ON a.id = s.audit_id`+
		auditattribution.ActorNameJoins+auditattribution.AgentClientNameJoin+`%s
			ORDER BY a.occurred_at DESC, a.id DESC
			LIMIT $%d`, where, fetchPos)
}

// sidecarImages turns a sidecar row's images into the company's own: the field
// name keyed to its value before and after. A write that moved no value (a
// confirmation) yields empty images, so it reads as no change. The verb is
// always an update of the company, whatever the sidecar row did.
func sidecarImages(field, action string, before, after map[string]any) (map[string]any, map[string]any, string) {
	if action == actionDelete {
		return map[string]any{field: before[sidecarValueKey]}, nil, actionUpdate
	}
	newValue, moved := after[sidecarValueKey]
	if !moved {
		return nil, nil, actionUpdate
	}
	var old map[string]any
	if oldValue, had := before[sidecarValueKey]; had {
		old = map[string]any{field: oldValue}
	}
	return old, map[string]any{field: newValue}, actionUpdate
}

const fieldHistoryBatchColumns = `
		a.id, a.action, a.actor_type, a.actor_id, a.passport_id, a.evidence,
		a.occurred_at, a.before, a.after,
		` + auditattribution.ActorNameColumn + ` AS actor_display_name,
		obo.display_name AS on_behalf_of_display_name, oc.client_name`

// fieldHistoryBatchSQL renders one window of the field-history spine, newest
// first. A company's window also takes the rows of its profile fields and
// facts. Each arm carries the LIMIT itself so each walks its own index to it.
func fieldHistoryBatchSQL(entityType string, own, shared []string, limitPos int) string {
	arm := func(cols, from string, conds []string) string {
		return fmt.Sprintf(`SELECT`+fieldHistoryBatchColumns+`, %s
			FROM %s`+auditattribution.ActorNameJoins+auditattribution.AgentClientNameJoin+`
			WHERE %s
			ORDER BY a.occurred_at DESC, a.id DESC
			LIMIT $%d`, cols, from, strings.Join(conds, " AND "), limitPos)
	}
	ownArm := arm("NULL::text AS side_field", "audit_log a", append(slices.Clone(own), shared...))
	side := sidecarSubjectCTE(entityType, 2)
	if side == "" {
		return ownArm
	}
	sideArm := arm("s.side_field", "side s JOIN audit_log a ON a.id = s.audit_id", shared)
	return fmt.Sprintf(`WITH %s
		SELECT * FROM ((%s) UNION ALL (%s)) AS spine
		ORDER BY occurred_at DESC, id DESC
		LIMIT $%d`, side, ownArm, sideArm, limitPos)
}

// fieldHistoryExistsSQL asks whether any row of the same window lies beyond the
// cursor, across the same arms the batch reads.
func fieldHistoryExistsSQL(entityType string, own, shared []string) string {
	ownProbe := fmt.Sprintf(`SELECT 1 FROM audit_log a WHERE %s`,
		strings.Join(append(slices.Clone(own), shared...), " AND "))
	side := sidecarSubjectCTE(entityType, 2)
	if side == "" {
		return "SELECT EXISTS(" + ownProbe + ")"
	}
	return fmt.Sprintf(`WITH %s
		SELECT EXISTS(%s) OR EXISTS(SELECT 1 FROM side s JOIN audit_log a ON a.id = s.audit_id WHERE %s)`,
		side, ownProbe, strings.Join(shared, " AND "))
}
