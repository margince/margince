// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The stage-advance lifecycle: won/lost is DERIVED from the target
// stage's semantic, terminal fields (closed_at, lost_reason, frozen FX)
// come and go with the transition, and every move lands in
// deal_stage_history plus the first-class deal.stage_changed event.

package deals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type AdvanceDealInput struct {
	ToStageID  ids.StageID
	LostReason *string
	IfVersion  *int64
	// WonWithoutContractReason says why this win has no agreement behind it
	// (ADR-0109 §6). Absent means the deal claims one, and the store looks for
	// it — a win that claims nothing and offers no reason is what gets refused.
	WonWithoutContractReason *string
	WonWithoutContractDetail *string
}

// StagePipelineMismatchError maps to 422: the target stage exists but
// belongs to another pipeline.
type StagePipelineMismatchError struct{ StageID ids.StageID }

func (e *StagePipelineMismatchError) Error() string {
	return "stage " + e.StageID.String() + " does not belong to the deal's pipeline"
}

// FieldFault refuses a target stage that belongs to another pipeline.
func (e *StagePipelineMismatchError) FieldFault() (field, code, message string) {
	return "to_stage_id", "stage_not_in_pipeline", e.Error()
}

// LostReasonRequiredError maps to 422 on advancing to a lost stage
// without a reason (deal_lost_reason CHECK, features/01 §3.1).
type LostReasonRequiredError struct{}

func (e *LostReasonRequiredError) Error() string { return "lost_reason is required to close as lost" }

// FieldFault refuses closing as lost with no reason recorded.
func (e *LostReasonRequiredError) FieldFault() (field, code, message string) {
	return "lost_reason", "lost_reason_required", e.Error()
}

// AdvanceDeal moves a deal one stage, deriving won/lost from the target
// stage's semantic (never from client-supplied status), appending the
// stage history snapshot and emitting the first-class deal.stage_changed
// event — never a generic deal.updated (events.md §1).
func (s *Store) AdvanceDeal(ctx context.Context, id ids.DealID, in AdvanceDealInput) (crmcontracts.Deal, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return crmcontracts.Deal{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return crmcontracts.Deal{}, err
	}

	var out crmcontracts.Deal
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, dealTable, id.UUID); err != nil {
			return err
		}
		// A decision read (stage/amount snapshot for the transition patch
		// and history row) — advance touches no custom columns, so the
		// pre-image needs none; the wire-returning read below carries them.
		current, err := readDeal(ctx, tx, id, storekit.LiveOnly, nil)
		if err != nil {
			return fmt.Errorf("read deal before advance: %w", err)
		}
		// pipeline_id/stage_id became nullable for overlay-mirror deals
		// (OVA-MAP-6), but a NATIVE deal — the only kind this native advance
		// path ever runs against — always carries both (NOT NULL columns).
		// Refuse a deal missing them rather than nil-deref below: an overlay
		// deal cannot reach here (advance_deal is unsupported in overlay mode),
		// so a nil is corruption, not a valid transition.
		if current.StageId == nil || current.PipelineId == nil {
			return fmt.Errorf("advance deal %s: deal has no native pipeline/stage", id)
		}

		semantic, winProbability, err := resolveAdvanceTarget(ctx, tx, in.ToStageID, current)
		if err != nil {
			return err
		}
		// Checked inside the transaction that writes the transition, and
		// checked BEFORE the patch is built, so a refusal costs nothing and a
		// concurrent archive cannot remove the evidence between the two.
		if StageSemantic(semantic) == SemanticWon {
			if err := ensureWinEvidence(ctx, tx, id, in); err != nil {
				return err
			}
		}

		p, status, err := s.stageTransitionPatch(ctx, tx, current, in, semantic)
		if err != nil {
			return err
		}
		if err := applyDealPatchGuarded(ctx, tx, id, p, in.IfVersion); err != nil {
			return fmt.Errorf("apply stage advance: %w", err)
		}

		// win_probability_at_change freezes the target stage's probability
		// the moment the deal lands there — stage config is mutable, and the
		// trajectory view must say what the odds WERE (the amount_at_change
		// rationale). Won/lost stages carry their semantic 100/0 in the same
		// column, so terminal moves snapshot too.
		if _, err := tx.Exec(ctx,
			`INSERT INTO deal_stage_history (deal_id, from_stage_id, to_stage_id, changed_by, amount_minor_at_change, currency_at_change, win_probability_at_change)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, ids.UUID(*current.StageId), in.ToStageID, by,
			current.AmountMinor, current.Currency, winProbability); err != nil {
			return fmt.Errorf("record stage history: %w", err)
		}

		auditID, err := storekit.Audit(ctx, tx, "advance_stage", "deal", id.UUID, p.Before(), p.After())
		if err != nil {
			return fmt.Errorf("audit stage advance: %w", err)
		}
		// The §5.3 payload carries the amount snapshot so as-of-date
		// pipeline reports and the overnight stalled/forecast sweep react
		// without a read-back; to_status records the 🟡 won/lost class.
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID,
			dealStageChangedPayload(current, in.ToStageID, status, winProbability, frozenFxFromPatch(p, current))); err != nil {
			return fmt.Errorf("emit deal.stage_changed: %w", err)
		}
		// Winning the deal turns the correspondence filed against it into
		// Handelsbriefe (A165/ADR-0114). Stamped here, in the transaction that
		// won it: a stamp that landed later would leave a window in which an
		// erasure sees unclassified correspondence and destroys it. Reopening
		// the deal never unstamps — the classification is monotonic because
		// over-retention is arguable and destruction is not.
		if DealStatus(status) == DealWon {
			if err := s.stampCorrespondence(ctx, tx, id, BasisDealWon); err != nil {
				return fmt.Errorf("stamp won deal's correspondence: %w", err)
			}
			// And a deal that BECOMES won starts the delivery it was sold for,
			// in the same transaction, so a won deal and a project still
			// reading as "pursuing" can never be observed together. See
			// project_delivery.go for which projects this moves and which it
			// deliberately leaves alone.
			//
			// Gated on the transition, not on the resulting status, and that
			// is a security boundary rather than an optimization. The stamp
			// above may re-run freely because it is monotonic; this is not. A
			// caller who re-asserts the won stage on an already-won deal must
			// not thereby drive the project — which they may have no authority
			// to see, let alone write — back to `delivering` after somebody
			// deliberately moved it elsewhere.
			//
			// The patch is what says whether the status actually moved:
			// stageTransitionPatch sets the column only when it differs from
			// what the deal already had, so its presence IS the transition.
			if _, becameWon := p.After()["status"]; becameWon {
				if err := s.startDeliveryForWonDeal(ctx, tx, id, by); err != nil {
					return fmt.Errorf("start delivery on the won deal's project: %w", err)
				}
			}
		}
		if out, err = readDealForCaller(ctx, tx, id, storekit.LiveOnly, active); err != nil {
			return fmt.Errorf("read advanced deal: %w", err)
		}
		return nil
	})
	return out, err
}

// dealStageChangedPayload builds the deal.stage_changed wire payload from
// the pre-move deal snapshot and the resolved transition — the ONE place
// that maps AdvanceDeal's local values onto the published schema, so a
// future field rename shows up here (and at its call site) rather than at
// two independently-drifting map literals.
func dealStageChangedPayload(current crmcontracts.Deal, toStageID ids.StageID, toStatus string, winProbability int, frozenFx *string) crmcontracts.PublicEventDealStageChanged {
	payload := crmcontracts.PublicEventDealStageChanged{
		FromStageId:         current.StageId,
		ToStageId:           openapi_types.UUID(toStageID.UUID),
		FromStatus:          string(current.Status),
		ToStatus:            toStatus,
		AmountMinorAtChange: current.AmountMinor,
		CurrencyAtChange:    current.Currency,
		WinProbability:      winProbability,
		PartnerOrgId:        current.PartnerOrgId,
		FxRateToBase:        frozenFx,
	}
	if current.PartnerAttribution != nil {
		attribution := string(*current.PartnerAttribution)
		payload.PartnerAttribution = &attribution
	}
	return payload
}

// frozenFxFromPatch reads the rate this transition froze out of the patch that
// froze it. Taken from the patch rather than from the pre-move snapshot because
// the freeze happens IN this transaction: the snapshot predates it, and a
// consumer reading the deal back afterwards can lose the rate entirely — a
// reopen clears the column.
func frozenFxFromPatch(p *storekit.Patch, current crmcontracts.Deal) *string {
	rate, changed := p.After()[fxRateColumn]
	if !changed {
		return current.FxRateToBase
	}
	frozen, ok := rate.(string)
	if !ok {
		// Cleared on the way out of won, which carries no rate to publish.
		return nil
	}
	return &frozen
}

// resolveAdvanceTarget reads the target stage's semantic and win
// probability and enforces that it belongs to the deal's own pipeline —
// a stage from another pipeline is a 422, a missing/archived stage a 404.
func resolveAdvanceTarget(ctx context.Context, tx pgx.Tx, toStage ids.StageID, current crmcontracts.Deal) (semantic string, winProbability int, err error) {
	var stagePipeline ids.PipelineID
	err = tx.QueryRow(ctx,
		`SELECT semantic, pipeline_id, win_probability FROM stage WHERE id = $1 AND archived_at IS NULL`+
			lockLiveStageTarget,
		toStage).Scan(&semantic, &stagePipeline, &winProbability)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, apperrors.ErrNotFound
	}
	if err != nil {
		return "", 0, fmt.Errorf("resolve target stage: %w", err)
	}
	if stagePipeline.UUID != ids.UUID(*current.PipelineId) {
		return "", 0, &StagePipelineMismatchError{StageID: toStage}
	}
	return semantic, winProbability, nil
}

// stageTransitionPatch derives the row changes one stage move implies
// and the resulting status: terminal fields (closed_at, lost_reason,
// frozen FX) are set when the target semantic closes the deal and
// cleared when a won/lost deal reopens.
func (s *Store) stageTransitionPatch(ctx context.Context, tx pgx.Tx,
	current crmcontracts.Deal, in AdvanceDealInput, semantic string,
) (*storekit.Patch, string, error) {
	status := "open"
	var closedAt *time.Time
	switch semantic {
	case "won", "lost":
		status = semantic
		now := time.Now().UTC()
		closedAt = &now
		if StageSemantic(semantic) == SemanticLost && (in.LostReason == nil || *in.LostReason == "") {
			return nil, "", &LostReasonRequiredError{}
		}
	}

	p := storekit.NewPatch()
	p.Set("stage_id", current.StageId, in.ToStageID)
	if status != string(current.Status) {
		p.Set("status", current.Status, status)
	}
	if closedAt != nil {
		p.Set("closed_at", current.ClosedAt, *closedAt)
	}
	// Written when this advance lands on lost, and cleared on every other
	// landing. A reason describing a previous close is worse than none: it
	// answers the report with a fact about a different outcome — so a deal
	// re-decided from lost to won must not carry the loss explanation with it.
	// The clear is conditional on there being something to clear: Set records
	// an assignment unconditionally, so clearing a column that is already NULL
	// would put lost_reason into the UPDATE and the audit diff of every
	// ordinary open-to-open advance.
	if DealStatus(status) == DealLost && in.LostReason != nil {
		p.Set("lost_reason", current.LostReason, *in.LostReason)
	} else if DealStatus(status) != DealLost && current.LostReason != nil {
		p.Set("lost_reason", current.LostReason, nil)
	}
	// The won-without-contract reason is written only on a win, and cleared on
	// every other landing, for the same reason.
	if DealStatus(status) == DealWon {
		p.Set("won_without_contract_reason", current.WonWithoutContractReason, in.WonWithoutContractReason)
		p.Set("won_without_contract_detail", current.WonWithoutContractDetail, in.WonWithoutContractDetail)
	} else {
		p.Set("won_without_contract_reason", current.WonWithoutContractReason, nil)
		p.Set("won_without_contract_detail", current.WonWithoutContractDetail, nil)
	}
	// Closing with an amount freezes the FX rate so base-currency roll-ups stay
	// reproducible (deal_closed_fx). As of the moment recorded as the close, and
	// read from `closedAt` rather than from the clock a second time: two reads
	// either side of UTC midnight would date the frozen rate a day after the
	// close it is supposed to price, and the roll-up would then reconcile
	// against a rate the deal never closed at.
	if DealStatus(status) != DealOpen && current.AmountMinor != nil && current.Currency != nil {
		rateBefore, rateDateBefore := frozenBefore(current)
		if err := s.freezeBaseRate(ctx, tx, p, current.Id, string(*current.Currency),
			*current.AmountMinor, *closedAt, rateBefore, rateDateBefore); err != nil {
			return nil, "", fmt.Errorf("freeze fx at close: %w", err)
		}
	}
	// Reopening a won/lost deal must clear the remaining terminal fields —
	// the DB CHECKs are one-directional, so a stale closed_at or frozen rate
	// on an open deal would silently corrupt forecast and won-lost reporting.
	// lost_reason is not here: it clears on every non-lost landing above,
	// which covers the reopen too.
	if DealStatus(status) == DealOpen && DealStatus(current.Status) != DealOpen {
		p.Set("closed_at", current.ClosedAt, nil)
		// The pre-images are what the row HELD, not nil. A closed deal carrying an
		// amount carries a rate — deal_closed_fx requires it — and the reopen's
		// audit row is precisely what a reversal reads to put the old rate back.
		rateBefore, rateDateBefore := frozenBefore(current)
		p.Set(fxRateColumn, rateBefore, nil)
		p.SetDate(fxRateDateColumn, rateDateBefore, nil)
		// The converted amount goes with the rate it was converted at, so the
		// reopen clears both. clearFrozenConversion owns that, beside the writer
		// that sets it: two places deciding what a frozen conversion consists of
		// is how one of them comes to leave half of it behind.
		if err := clearFrozenConversion(ctx, tx, p, current); err != nil {
			return nil, "", err
		}
	}
	return p, status, nil
}
