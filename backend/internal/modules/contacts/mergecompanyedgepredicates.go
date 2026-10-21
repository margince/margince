// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "github.com/margince/margince/backend/internal/shared/kernel/employment"

const (
	mergeCompanyEdgeSource    = "a.archived_at IS NULL AND (a.company_id = @source OR a.counterparty_company_id = @source)"
	mergeCompanyForwardSource = "a.company_id = @source AND a.archived_at IS NULL"
	mergeCompanyEdgeSelf      = "((a.company_id = @source AND a.counterparty_company_id = @target) OR (a.company_id = @target AND a.counterparty_company_id = @source))"
)

var mergeCompanyEdgeDuplicate = `EXISTS (SELECT 1 FROM relationship b
 WHERE b.kind=a.kind AND b.archived_at IS NULL AND b.id<>a.id
 AND b.contact_id IS NOT DISTINCT FROM a.contact_id
 AND b.deal_id IS NOT DISTINCT FROM a.deal_id
 AND b.project_id IS NOT DISTINCT FROM a.project_id
 AND b.company_id IS NOT DISTINCT FROM (CASE WHEN a.company_id=@source THEN @target::uuid ELSE a.company_id END)
 AND b.counterparty_company_id IS NOT DISTINCT FROM (CASE WHEN a.counterparty_company_id=@source THEN @target::uuid ELSE a.counterparty_company_id END)
 AND ` + roleKeyedDuplicateSQL + `)`

var mergeCompanyPrimaryWitness = "b.contact_id = a.contact_id AND b.id <> a.id AND " + employment.CurrentPrimarySlotSQL("b")

var mergeCompanyEdgePrimary = "a.is_current_primary AND NOT EXISTS (SELECT 1 FROM relationship b WHERE " + mergeCompanyPrimaryWitness + ")"
