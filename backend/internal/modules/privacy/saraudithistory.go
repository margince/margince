// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func sarAuditHistorySection(pkg *SARPackage, identities, leads []ids.UUID) sarSection {
	args := []any{identities, leads}
	ownRecord := storekit.SQLf(`(entity_type='contact' AND entity_id=ANY($%[1]d::uuid[]))
 OR (entity_type='lead' AND entity_id=ANY($%[2]d::uuid[]))`, len(args)-1, len(args))
	bound := func(image string) string {
		return storekit.SQLf(`entity_type='weekly_plan_commitment' AND
 ((%[1]s->>'linked_record_type'='contact' AND %[1]s->>'linked_record_id'=ANY(ARRAY(SELECT id::text FROM unnest($%[2]d::uuid[]) id)))
 OR (%[1]s->>'linked_record_type'='lead' AND %[1]s->>'linked_record_id'=ANY(ARRAY(SELECT id::text FROM unnest($%[3]d::uuid[]) id))))`, image, len(args)-1, len(args))
	}
	project := func(image string) string {
		// A current link cannot prove whom an earlier label was about.
		return `CASE WHEN own_record THEN ` + image + ` WHEN ` + bound(image) + ` THEN
 jsonb_build_object('linked_record_type',` + image + `->'linked_record_type',
 'linked_record_id',` + image + `->'linked_record_id','label',` + image + `->'label',
 'help_requested',` + image + `->'help_requested','manager_response',` + image + `->'manager_response',
 'state',` + image + `->'state','due_on',` + image + `->'due_on') END`
	}
	query := `WITH history AS (SELECT *, (` + ownRecord + `) AS own_record FROM audit_log)
 SELECT action,entity_type,entity_id,` + project("before") + ` AS before,` + project("after") + ` AS after,
 CASE WHEN own_record THEN evidence END AS evidence,occurred_at FROM history
 WHERE own_record OR (` + bound("before") + `) OR (` + bound("after") + `)`
	return sarSection{&pkg.AuditHistory, query, args}
}
