// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package lifecyclereview

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	sourceReadLimit = 100
	scopeUnknown    = "unknown"
)

// Actionable requires complete account-wide evidence without continuing work.
func Actionable(finding Finding) bool {
	return BlockingReason(finding) == ""
}

// BlockingReason explains why an account-ending proposal cannot be approved.
func BlockingReason(finding Finding) string {
	switch {
	case finding.Incomplete || finding.SourceID.IsZero():
		return "incomplete_evidence"
	case finding.Scope != "company":
		return "unknown_scope"
	case finding.ActiveContract:
		return "active_contract"
	case finding.ActiveDelivery:
		return "active_delivery"
	default:
		return ""
	}
}

func latest(ctx context.Context, tx pgx.Tx, company ids.CompanyID, lock bool) (Finding, error) {
	if lock {
		if err := signals.LockCompanySignalsTx(ctx, tx, company); err != nil {
			return Finding{}, err
		}
	}
	records, page, err := signals.ListSignalsTx(ctx, tx, signals.ListSignalsInput{
		CompanyID: &company.UUID, Kind: new("contract_ended"), Status: new("open"), Limit: new(sourceReadLimit),
	}, nil)
	if err != nil {
		return Finding{}, err
	}
	if lock {
		if err := lockSources(ctx, tx, records); err != nil {
			return Finding{}, err
		}
	}
	complete, err := signals.CompanySignalsCompleteTx(ctx, tx, company, "contract_ended")
	if err != nil {
		return Finding{}, err
	}
	return latestFinding(ctx, tx, company, records, page.HasMore || !complete)
}

func latestFinding(ctx context.Context, tx pgx.Tx, company ids.CompanyID, records []crmcontracts.Signal, incomplete bool) (Finding, error) {
	latest := Finding{Scope: scopeUnknown}
	ambiguous := false
	for _, record := range records {
		finding, err := readSignalFinding(ctx, tx, company, record)
		if err != nil {
			return Finding{}, err
		}
		if finding.SourceID.IsZero() {
			incomplete = true
		}
		if finding.OccurredAt.After(latest.OccurredAt) {
			ambiguous = false
		} else if finding.OccurredAt.Equal(latest.OccurredAt) && !latest.SignalID.IsZero() && finding.Scope != latest.Scope {
			ambiguous = true
		}
		if latest.SignalID.IsZero() || finding.OccurredAt.After(latest.OccurredAt) ||
			(finding.OccurredAt.Equal(latest.OccurredAt) && finding.SignalID.String() > latest.SignalID.String()) {
			latest = finding
		}
	}
	latest.Incomplete = incomplete || ambiguous
	return latest, nil
}

func lockSources(ctx context.Context, tx pgx.Tx, records []crmcontracts.Signal) error {
	var sources []string
	for _, record := range records {
		for _, evidence := range record.Evidence {
			if evidence.SourceType != nil && *evidence.SourceType == "activity" && evidence.SourceId != nil {
				sources = append(sources, *evidence.SourceId)
			}
		}
	}
	slices.Sort(sources)
	for _, source := range slices.Compact(sources) {
		id, err := ids.Parse(source)
		if err != nil {
			return apperrors.ErrVersionSkew
		}
		if err := activities.LockActivityLive(ctx, tx, ids.From[ids.ActivityKind](id)); err != nil {
			return err
		}
	}
	return nil
}
