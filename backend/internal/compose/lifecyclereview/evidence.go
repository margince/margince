// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package lifecyclereview

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const RuleVersion = "1"

type Finding struct {
	ActiveDelivery bool      `json:"active_delivery,omitempty"`
	ActiveContract bool      `json:"active_contract,omitempty"`
	Incomplete     bool      `json:"incomplete,omitempty"`
	SignalID       ids.UUID  `json:"signal_id"`
	SignalVersion  int64     `json:"signal_version"`
	SourceID       ids.UUID  `json:"source_id"`
	SourceVersion  int64     `json:"source_version"`
	OccurredAt     time.Time `json:"-"`
	Quote          string    `json:"-"`
	Summary        string    `json:"-"`
	QuoteDigest    string    `json:"quote_digest"`
	Scope          string    `json:"scope"`
	Workstream     string    `json:"-"`
}

// LiveStages keeps conflict cards and proposal scans on the same account boundary.
func LiveStages() []string { return []string{"prospect", "opportunity", "customer"} }

func LiveStage(stage string) bool { return slices.Contains(LiveStages(), stage) }

func Revalidate(ctx context.Context, tx pgx.Tx, company ids.CompanyID, signalID ids.UUID, shown *Finding, now time.Time) error {
	current, err := Read(ctx, tx, company, signalID)
	if err != nil {
		return err
	}
	if shown == nil || current.SourceID.IsZero() {
		return apperrors.ErrVersionSkew
	}
	current, err = ReconcileForDecision(ctx, tx, company, now)
	if err != nil {
		return err
	}
	if !Matches(current, shown) {
		return apperrors.ErrVersionSkew
	}
	return nil
}

func Matches(current Finding, shown *Finding) bool {
	return shown != nil && Actionable(current) && !shown.Incomplete && shown.Scope == current.Scope &&
		!current.SourceID.IsZero() && current.SignalID == shown.SignalID && current.SignalVersion == shown.SignalVersion &&
		current.SourceID == shown.SourceID && current.SourceVersion == shown.SourceVersion &&
		current.QuoteDigest != "" && current.QuoteDigest == shown.QuoteDigest
}

// Read binds a signal's scope to words its reader can still open.
func Read(ctx context.Context, tx pgx.Tx, company ids.CompanyID, signalID ids.UUID) (Finding, error) {
	signal, err := signals.ReadSignalTx(ctx, tx, ids.From[ids.SignalKind](signalID), storekit.LiveOnly)
	if err != nil {
		return Finding{SignalID: signalID, Scope: "unknown"}, err
	}
	return readSignalFinding(ctx, tx, company, signal)
}

func readSignalFinding(ctx context.Context, tx pgx.Tx, company ids.CompanyID, signal crmcontracts.Signal) (Finding, error) {
	finding := Finding{SignalID: ids.UUID(signal.Id), Scope: "unknown", Summary: signal.Summary}
	if signal.Kind != "contract_ended" || signal.Status != "open" ||
		signal.ResolvedCompanyId == nil || ids.UUID(*signal.ResolvedCompanyId) != company.UUID {
		return finding, nil
	}
	if signal.Version != nil {
		finding.SignalVersion = *signal.Version
	}
	for _, evidence := range signal.Evidence {
		if evidence.SourceType == nil || *evidence.SourceType != "activity" || evidence.SourceId == nil || evidence.SourceQuote == nil {
			continue
		}
		source, err := ids.Parse(*evidence.SourceId)
		if err != nil {
			continue
		}
		activity, err := activities.ReadActivityContentTx(ctx, tx, ids.From[ids.ActivityKind](source), storekit.LiveOnly)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			continue
		}
		if err != nil {
			return finding, err
		}
		if activity.Body == nil {
			continue
		}
		proof := values.QuoteProof{Start: evidence.SourceQuote.Start, End: evidence.SourceQuote.End, SHA256: evidence.SourceQuote.Sha256}
		quote, verified := values.VerifyQuote(textlang.CurrentMessage(*activity.Body), proof)
		if !verified {
			continue
		}
		finding.SourceID, finding.OccurredAt, finding.Quote = source, activity.OccurredAt, quote
		finding.QuoteDigest = proof.SHA256
		if activity.Version != nil {
			finding.SourceVersion = *activity.Version
		}
		if evidence.LifecycleScope != nil {
			finding.Scope = string(*evidence.LifecycleScope)
		}
		if evidence.Workstream != nil {
			finding.Workstream = *evidence.Workstream
		}
		return finding, nil
	}
	return finding, nil
}
