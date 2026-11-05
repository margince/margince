// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "github.com/margince/margince/backend/internal/shared/kernel/employment"

const (
	mergeContactEdgeSource  = "a.contact_id = @source AND a.kind <> 'works_with' AND a.archived_at IS NULL"
	mergeWorksWithSource    = "a.kind = 'works_with' AND a.archived_at IS NULL AND (a.contact_id = @source OR a.counterparty_contact_id = @source)"
	mergeWorksWithDuplicate = `(a.contact_id = @target OR a.counterparty_contact_id = @target OR EXISTS (
 SELECT 1 FROM relationship b WHERE b.kind = 'works_with' AND b.archived_at IS NULL AND b.id <> a.id
 AND LEAST(b.contact_id, b.counterparty_contact_id) = LEAST(@target::uuid, CASE WHEN a.contact_id = @source THEN a.counterparty_contact_id ELSE a.contact_id END)
 AND GREATEST(b.contact_id, b.counterparty_contact_id) = GREATEST(@target::uuid, CASE WHEN a.contact_id = @source THEN a.counterparty_contact_id ELSE a.contact_id END)))`
)

var mergeContactEdgeDuplicate = `EXISTS (SELECT 1 FROM relationship b
 WHERE b.contact_id = @target AND b.kind = a.kind AND b.archived_at IS NULL
 AND b.company_id IS NOT DISTINCT FROM a.company_id
 AND b.deal_id IS NOT DISTINCT FROM a.deal_id
 AND b.project_id IS NOT DISTINCT FROM a.project_id
 AND ` + roleKeyedDuplicateSQL + `)`

var mergeContactPrimaryWitness = "b.contact_id = @target AND " + employment.CurrentPrimarySlotSQL("b")

var mergeContactEdgePrimary = "a.is_current_primary AND NOT EXISTS (SELECT 1 FROM relationship b WHERE " + mergeContactPrimaryWitness + ")"
