// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The rate-refresh proposal kinds and their apply-on-approval effects. A
// producer (Task 3/4) stages one proposal per changed value; on human
// approval the effect redeems and writes through the Phase-1 store method as
// a system principal on behalf of the deciding admin. The proposals target
// the workspace (config, no row scope) and carry the logical identity in the
// payload; the effect applies the rate effective TODAY (never a date pinned
// at staging time — a cross-midnight approval must not miss the past-date
// guard), so the payload carries no date.
const (
	fxRateProposalKind = "fx_rate_proposal"
	fxRateTargetType   = "fx_rate"
)

type fxRateProposal struct {
	FromCurrency string `json:"from_currency"`
	Rate         string `json:"rate"`
	// ExpectedPriorRate is the rate in force (as of the staging day) the diff
	// was computed against; empty = no rate was in force. The apply effect
	// re-reads and refuses on mismatch (ErrVersionSkew) so an old proposal can
	// never overwrite a newer manual or approved rate.
	ExpectedPriorRate string `json:"expected_prior_rate,omitempty"`
}

// sameRate reports numeric equality of two decimal strings — numeric(20,10)
// text and a source's text may spell one value at different scales.
func sameRate(a, b string) bool {
	ra, okA := new(big.Rat).SetString(a)
	rb, okB := new(big.Rat).SetString(b)
	return okA && okB && ra.Cmp(rb) == 0
}

// stageRateProposal marshals a proposal, computes its identity-bearing diff
// hash (sha256 over the payload, per the scrape.go shape), and stages it under
// JoinPending with the sheet row's logical identity — the atomic
// advisory-locked path that collapses an identical live proposal to a no-op
// AND withdraws a stale pending diff for the same identity (two refreshes
// fetching different values must not leave competing proposals whose late
// approval restores the older value).
//
//craft:ignore naked-any payload is any JSON-marshalable proposal struct (fx or model); the concrete type rides through json.Marshal
func stageRateProposal(ctx context.Context, svc *approvals.Service, kind, targetType string, ws ids.UUID, payload any, identity json.RawMessage, summary string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("compose: marshal %s: %w", kind, err)
	}
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	// StageUnlessDeclined rather than Stage, and the difference is a rejection
	// rather than a duplicate. JoinPending collapses a proposal that is still
	// waiting; the moment a human turns one down there is no pending row left to
	// join, so a plain Stage puts the refused figure straight back. And it comes
	// back on every refresh after that: the diff is computed against the RATE
	// SHEET, which a rejection does not change, so the same click re-derives the
	// same proposal from the same source indefinitely.
	//
	// The identity was already declared for the join, and it is what makes the
	// memory precise: a refused $5 stays refused while a genuine move to $6 is
	// still offered.
	_, _, err = svc.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind: kind, ProposedChange: raw, DiffHash: hash,
		TargetType: targetType, TargetID: ws, Summary: summary,
		JoinPending: true, Identity: identity,
	})
	return err
}

// rateRefreshActor binds the system principal a rate-refresh effect applies
// under (bypasses auth.Require), on behalf of the deciding admin.
func rateRefreshActor(ctx context.Context) (context.Context, error) {
	decider, ok := principal.Actor(ctx)
	if !ok {
		return nil, errors.New("compose: rate refresh effect without a deciding principal")
	}
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "agent:rate-refresh",
		UserID: decider.UserID, OnBehalfOf: decider.UserID,
	}), nil
}

func fxRateAcceptEffect(svc *approvals.Service, store *deals.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var p fxRateProposal
		if err := json.Unmarshal(proposedChange, &p); err != nil {
			return fmt.Errorf("compose: fx rate proposal payload: %w", err)
		}
		execCtx, err := rateRefreshActor(ctx)
		if err != nil {
			return err
		}
		// Redeem the authority object and apply the sheet write in ONE
		// transaction: a failed write leaves the approval unconsumed and the
		// job retryable, never permanently consumed with the sheet unchanged.
		// The precondition read returns the day it sampled and the write is
		// pinned to that SAME day, so a cross-midnight apply fails the
		// past-date guard rather than overwriting the new day's scheduled row.
		return svc.RedeemAndApply(ctx, approvalID, fxRateProposalKind, diffHash, func(tx pgx.Tx) error {
			// The diff was computed against the rate then in force; if the sheet
			// moved since (manual write, competing approval), applying would
			// silently restore a stale value — refuse and roll back instead. The
			// decision itself stays on record; the remedy is a fresh refresh.
			// The read holds the currency's write-identity lock and the write is
			// pinned to the SAME sampled day, so neither a concurrent standalone
			// write nor a UTC-midnight crossing can slip between check and apply
			// (the past-date guard refuses the crossed-midnight case honestly).
			prior, asOf, found, err := store.EffectiveFxRateInTx(execCtx, tx, p.FromCurrency)
			if err != nil {
				return err
			}
			if err := fxPriorMatches(p, prior, found); err != nil {
				return err
			}
			_, err = store.SetFxRateInTx(execCtx, tx, deals.SetFxRateInput{
				FromCurrency: p.FromCurrency, Rate: p.Rate, EffectiveDate: asOf,
			})
			return err
		})
	}
}

// fxRefreshAdvice is the remedy both refusals name, as the web's decision.fxRateMoved does.
const fxRefreshAdvice = "Refresh the rates from their sources for a current proposal."

// fxPriorMatches enforces the proposal's precondition: the rate in force now
// must be exactly the one the diff was computed against (numerically — the
// sheet stores scale-10 text). An empty ExpectedPriorRate asserts "none was
// in force", which is also how a payload staged before the precondition
// existed reads — such a proposal fails closed onto a re-diff.
func fxPriorMatches(p fxRateProposal, prior string, found bool) error {
	switch {
	case !found && p.ExpectedPriorRate == "":
		return nil
	case found && p.ExpectedPriorRate != "" && sameRate(prior, p.ExpectedPriorRate):
		return nil
	case !found:
		return &apperrors.VersionSkewError{Message: fmt.Sprintf(
			"The %s exchange rate this proposal was made against is no longer in force, so it was not applied. "+
				fxRefreshAdvice, p.FromCurrency)}
	default:
		return &apperrors.VersionSkewError{Message: fmt.Sprintf(
			"The %s exchange rate changed to %s after this proposal was made, so it was not applied. "+
				fxRefreshAdvice, p.FromCurrency, prior)}
	}
}
