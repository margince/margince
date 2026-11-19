// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	reportLossEvents = "loss-events"
	fieldDealID      = "deal_id"
	aliasLosses      = "losses"

	// colLossAt is when a deal was lost: the instant of the stage move to lost,
	// not deal.closed_at, which holds only the latest closing.
	colLossAt = "h.changed_at"

	// joinLossMoves has one row per loss of the deal, so the grain is the loss
	// and a deal lost twice is two rows. A move that was later reversed is a
	// correction and no loss.
	joinLossMoves = "JOIN deal_stage_history h ON h.deal_id = t.id AND h.semantic_at_change = 'lost'" +
		" AND h.reversal_of IS NULL" +
		" AND NOT EXISTS (SELECT 1 FROM deal_stage_history reversal WHERE reversal.reversal_of = h.id)"
)

// lossEventsSpec counts losses where win-loss counts deals. It reads the deal
// through the same table alias as every deal report, so the deal's row scope,
// field masks and owner gate narrow it without a second spelling.
func lossEventsSpec() reportSpec {
	dimensions := map[string]string{
		fieldOwnerID:    colOwnerID,
		fieldPipelineID: colPipelineID,
		fieldCompanyID:  colCompanyID,
		fieldLossReason: "h.lost_reason_code_at_change",
	}
	maps.Copy(dimensions, periodDimensions(colLossAt))
	filters := maps.Clone(dimensions)
	dimensions[fieldDealID] = colRowID
	// Each column read off the history row names its own deal, so the deal's
	// row scope is the history row's scope.
	scopeVia := map[string]string{fieldLossReason: colRowID}
	for name := range periodDimensions(colLossAt) {
		scopeVia[name] = colRowID
	}
	return reportSpec{
		entity:    datasource.EntityDeal,
		table:     tableDeal,
		joins:     []string{joinLossMoves},
		baseWhere: whereArchivedNull,
		basePlain: "every loss of a live (unarchived) deal, one row per move to lost, bucketed by when that move happened " +
			"in the installation's reporting timezone (a loss that was reversed is not a loss)",
		notes: "a deal lost, reopened and lost again counts twice, each time under the loss_reason it was lost with; " +
			"win-loss counts that deal once. loss_reason is empty for a loss recorded without a code. " +
			"owner_id, pipeline_id and company_id are the deal's current ones. Group by deal_id and loss_reason to list each loss",
		population:      measureEveryReadableRow,
		dimensions:      dimensions,
		measures:        map[string]string{},
		filters:         filters,
		rowKeys:         []string{fieldDealID},
		referenceScopes: map[string]string{colCompanyID: tableCompany, colRowID: tableDeal},
		scopeVia:        scopeVia,
		defaultBy:       []string{fieldLossReason},
		defaultAggs:     []reportAggregate{{Fn: aggFnCount, As: aliasLosses}},
	}
}
