// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	objectPartner    = "partner"
	objectCommission = "commission"
)

// commissionCredit is what the report adds to a ledger entry: the deal it was
// earned on, that deal's current owner, and the partner's breakdown group.
type commissionCredit struct {
	entry                            ids.UUID
	deal                             string
	owner                            *ids.UUID
	ownerLabel, groupKey, groupLabel string
}

// readReportingPartnerCommission reads the ledger through commissions.AccrualsTx,
// which owns the commission_entry table and its visibility. This reader only
// narrows the entries to the picked scope by their deal's owner and prices them.
func readReportingPartnerCommission(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	accruals, withheld, err := commissions.AccrualsTx(ctx, tx, reportingEarliest(frame), frame.EvaluatedAt, reportingFactLimit)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	credits, err := commissionCredits(ctx, tx, frame, accruals)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	facts := make([]reporting.Fact, 0, len(accruals))
	for _, accrual := range accruals {
		credit, ok := credits[accrual.ID]
		if !ok {
			continue
		}
		fact, err := commissionFact(frame, accrual, credit)
		if err != nil {
			return nil, crmcontracts.ReportingCoverage{}, err
		}
		facts = append(facts, fact)
	}
	return reportingCohorts(frame, facts), crmcontracts.ReportingCoverage{Status: "ok", Withheld: withheld}, nil
}

// commissionCredits keeps the entries whose deal's owner is in the picked scope,
// keyed by entry id. The ledger already admitted only deals this reader may see.
func commissionCredits(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, accruals []commissions.Accrual) (map[ids.UUID]commissionCredit, error) {
	out := map[ids.UUID]commissionCredit{}
	if len(accruals) == 0 {
		return out, nil
	}
	entries, dealIDs, partners := make([]ids.UUID, len(accruals)), make([]ids.UUID, len(accruals)), make([]*ids.UUID, len(accruals))
	for i, accrual := range accruals {
		entries[i], dealIDs[i], partners[i] = accrual.ID, accrual.DealID, accrual.PartnerCompanyID
	}
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), colOwnerID, b.arg, unownedIsExcluded)
	if err != nil {
		return nil, err
	}
	visible, err := partnerNameVisible(ctx, &b)
	if err != nil {
		return nil, err
	}
	hidden := b.add(reporting.ExplanationCopy(frame).Say(reportingBreakdownLabels.PartnerHidden))
	query := `SELECT e.id,t.name,t.owner_id,COALESCE(u.display_name,''),
 'partner:'||CASE WHEN e.partner_id IS NOT NULL AND ` + visible + ` THEN e.partner_id::text ELSE 'hidden' END,
 CASE WHEN e.partner_id IS NOT NULL AND ` + visible + ` THEN pc.display_name ELSE ` + hidden + ` END
 FROM unnest(` + b.add(entries) + `::uuid[],` + b.add(dealIDs) + `::uuid[],` + b.add(partners) + `::uuid[]) AS e(id,deal_id,partner_id)
 JOIN deal t ON t.id=e.deal_id LEFT JOIN company pc ON pc.id=e.partner_id LEFT JOIN app_user u ON u.id=t.owner_id
 WHERE ` + orUnnarrowed(population)
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, err
	}
	credits, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commissionCredit, error) {
		var c commissionCredit
		err := row.Scan(&c.entry, &c.deal, &c.owner, &c.ownerLabel, &c.groupKey, &c.groupLabel)
		return c, err
	})
	if err != nil {
		return nil, err
	}
	for _, credit := range credits {
		out[credit.entry] = credit
	}
	return out, nil
}

// commissionFact prices one entry in the reporting currency. Another currency
// converts at the rate the entry kept when it accrued. An entry with no usable
// rate counts without a value, which marks the figure partial.
func commissionFact(frame crmcontracts.ReportingContext, accrual commissions.Accrual, credit commissionCredit) (reporting.Fact, error) {
	fact := reporting.Fact{Metric: reportingPartnerCommission, SourceType: string(recordTypeDeal), SourceID: accrual.DealID, OwnerLabel: credit.ownerLabel, Provenance: "commission_accrual", GroupKey: credit.groupKey, GroupLabel: credit.groupLabel}
	at := accrual.AccruedAt
	fact.Row = crmcontracts.ReportingEvidenceRow{Key: accrual.ID.String(), Label: credit.deal, OccurredAt: &at, SourceId: ptrUUID(accrual.DealID), SourceType: &fact.SourceType}
	if credit.owner != nil {
		fact.OwnerID = *credit.owner
		fact.Row.OwnerId = ptrUUID(*credit.owner)
	}
	amount := accrual.AmountMinor
	if accrual.Currency != frame.Currency {
		converted, err := deals.ConvertToBase(accrual.AmountMinor, accrual.FxRateToBase, accrual.Currency, frame.Currency)
		if errors.Is(err, deals.ErrRateNotFinite) || errors.Is(err, deals.ErrAmountOutOfRange) {
			return fact, nil
		}
		if err != nil {
			return fact, err
		}
		amount = converted
	}
	if amount > reportingExactInteger {
		return fact, nil
	}
	value := float64(amount)
	fact.Money, fact.Row.Value = &amount, &value
	return fact, nil
}
