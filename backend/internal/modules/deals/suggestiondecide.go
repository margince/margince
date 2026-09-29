// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A rep deciding a suggestion: accept opens the deal it proposed, with the
// rep's corrections; dismiss records, for the whole workspace, that the
// evidence was not a deal. Both lock the suggestion and re-read it under the
// caller's visibility before they write.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealScoutSource is the provenance of a deal opened from a suggestion.
const dealScoutSource = "agent:dealscout"

// SuggestionEffects are the writes an acceptance makes outside this module:
// filing the evidence messages under the new deal and acknowledging the
// signals that raised it. Compose supplies them, because deals may import
// neither activities nor signals.
type SuggestionEffects interface {
	// LinkActivityToDeal files one message under the deal, on the caller's
	// authority. A refusal is ErrPermissionDenied or ErrNotFound.
	LinkActivityToDeal(ctx context.Context, tx pgx.Tx, activityID, dealID ids.UUID) error
	// AcknowledgeSignal settles one open signal, and reports whether it moved.
	AcknowledgeSignal(ctx context.Context, tx pgx.Tx, signalID ids.UUID) (bool, error)
}

// WithSuggestionEffects wires the acceptance's cross-module writes.
func (h Handlers) WithSuggestionEffects(effects SuggestionEffects) Handlers {
	h.store = h.store.WithSuggestionEffects(effects)
	return h
}

// WithSuggestionEffects answers a copy of the store with the acceptance's
// cross-module writes wired, leaving the receiver as it was.
func (s *Store) WithSuggestionEffects(effects SuggestionEffects) *Store {
	wired := *s
	wired.suggestionEffects = effects
	return &wired
}

// SuggestionDecidedError is a decision on a suggestion somebody already
// decided. It answers 409.
type SuggestionDecidedError struct{ State string }

func (e *SuggestionDecidedError) Error() string {
	return "deals: the suggestion is already " + e.State
}

// MessageFault names the state the suggestion is in, so the caller can reload.
func (e *SuggestionDecidedError) MessageFault() (code, message string) {
	return "suggestion_decided", "This suggestion was already " + e.State + ". Reload to see where it stands."
}

func (e *SuggestionDecidedError) Unwrap() error { return apperrors.ErrConflict }

// SuggestionSupersededError is an acceptance that found the company already
// has an open deal. The suggestion is retired rather than opening a second
// deal. It answers 409.
type SuggestionSupersededError struct{}

func (e *SuggestionSupersededError) Error() string {
	return "deals: the company already has an open deal"
}

// MessageFault says why nothing was opened.
func (e *SuggestionSupersededError) MessageFault() (code, message string) {
	return "suggestion_superseded", "This company already has an open deal, so the suggestion was retired. Open the deal instead."
}

func (e *SuggestionSupersededError) Unwrap() error { return apperrors.ErrConflict }

// NoAmountConflictError is a request that both drops the amount and names one.
type NoAmountConflictError struct{}

func (e *NoAmountConflictError) Error() string {
	return "deals: no_amount cannot be sent with an amount or a currency"
}

// FieldFault names the flag the caller must drop, or the amount.
func (e *NoAmountConflictError) FieldFault() (field, code, message string) {
	return "no_amount", "conflicting_amount", "Send no_amount alone, or an amount with its currency."
}

// AcceptSuggestionInput is the rep's corrections. A nil field keeps the
// suggestion's own value; the amount and currency travel as a pair, and
// NoAmount drops the suggested amount.
type AcceptSuggestionInput struct {
	Name        *string
	NoAmount    bool
	AmountMinor *int64
	Currency    *string
	StageID     *ids.UUID
	OwnerID     *ids.UUID
	CloseDate   *time.Time
}

// SuggestionAcceptance is what an acceptance did: the deal it opened, the
// evidence messages it could not file under the deal because the rep may not
// move them, and how many signals it acknowledged.
type SuggestionAcceptance struct {
	Suggestion   Suggestion
	DealID       ids.UUID
	Unlinked     []ids.UUID
	Acknowledged int
}

// AcceptSuggestion opens the proposed deal and records the decision, in one
// transaction.
func (s *Store) AcceptSuggestion(ctx context.Context, id ids.UUID, in AcceptSuggestionInput) (SuggestionAcceptance, error) {
	if err := auth.Require(ctx, "deal", principal.ActionCreate); err != nil {
		return SuggestionAcceptance{}, err
	}
	if s.suggestionEffects == nil {
		return SuggestionAcceptance{}, errors.New("deals: accepting a suggestion needs its effects wired")
	}
	var out SuggestionAcceptance
	superseded := false
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		current, err := openSuggestionForDecision(ctx, tx, id)
		if err != nil {
			return err
		}
		if superseded, err = supersedeIfCompanyTaken(ctx, tx, current); err != nil || superseded {
			return err
		}
		birth, err := s.acceptedDealInput(ctx, tx, current, in)
		if err != nil {
			return err
		}
		deal, err := s.CreateDealTx(ctx, tx, birth)
		if err != nil {
			return err
		}
		out.DealID = ids.UUID(deal.Id)
		if out.Unlinked, out.Acknowledged, err = s.applyAcceptEffects(ctx, tx, current, out.DealID); err != nil {
			return err
		}
		out.Suggestion = current
		out.Suggestion.State = SuggestionAccepted
		return recordDecision(ctx, tx, current.ID, SuggestionAccepted, &out.DealID)
	})
	if err == nil && superseded {
		return SuggestionAcceptance{}, &SuggestionSupersededError{}
	}
	return out, err
}

// supersedeIfCompanyTaken retires the suggestion, and reports so, when its
// company has an open deal by now — opened by hand since the last pass.
//
// The company row is locked FOR UPDATE first. Every deal insert naming the
// company takes a key-share lock on that row for its foreign key, which this
// lock conflicts with: a deal created by hand concurrently either committed
// before the check below sees it, or waits for this acceptance to finish. So
// no acceptance opens a second deal beside one it could not see, and ordinary
// deal creation takes no lock it did not already take.
func supersedeIfCompanyTaken(ctx context.Context, tx pgx.Tx, current Suggestion) (bool, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return false, err
	}
	var free bool
	if err := tx.QueryRow(ctx, `
		SELECT `+SuggestionCompanyFreeClause("c.id")+`
		  FROM company c WHERE c.id = $1 FOR UPDATE OF c`, current.CompanyID).Scan(&free); err != nil {
		return false, fmt.Errorf("deals: locking the suggestion's company: %w", err)
	}
	if free {
		return false, nil
	}
	return true, supersedeTx(ctx, tx, current.ID)
}

// DismissSuggestion records that the evidence is not a deal. The decision is
// the workspace's: nobody is offered this evidence again, and the company is
// offered a new suggestion only on evidence newer than now.
func (s *Store) DismissSuggestion(ctx context.Context, id ids.UUID) (Suggestion, error) {
	if err := auth.Require(ctx, "deal", principal.ActionCreate); err != nil {
		return Suggestion{}, err
	}
	var out Suggestion
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		current, err := openSuggestionForDecision(ctx, tx, id)
		if err != nil {
			return err
		}
		out = current
		out.State = SuggestionDismissed
		return recordDecision(ctx, tx, current.ID, SuggestionDismissed, nil)
	})
	return out, err
}

// openSuggestionForDecision locks the suggestion under the caller's
// visibility and refuses one that is no longer open.
func openSuggestionForDecision(ctx context.Context, tx pgx.Tx, id ids.UUID) (Suggestion, error) {
	current, err := readVisibleSuggestionTx(ctx, tx, id, true)
	if err != nil {
		return Suggestion{}, err
	}
	if current.State != SuggestionOpen {
		return Suggestion{}, &SuggestionDecidedError{State: current.State}
	}
	return current, nil
}

// acceptedDealInput folds the rep's corrections over the suggestion.
func (s *Store) acceptedDealInput(ctx context.Context, tx pgx.Tx, current Suggestion, in AcceptSuggestionInput) (CreateDealInput, error) {
	name := current.CompanyName
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		name = strings.TrimSpace(*in.Name)
	}
	amount, currency := current.AmountMinor, current.Currency
	switch {
	case in.NoAmount && (in.AmountMinor != nil || in.Currency != nil):
		return CreateDealInput{}, &NoAmountConflictError{}
	case in.NoAmount:
		amount, currency = nil, nil
	case in.AmountMinor != nil || in.Currency != nil:
		amount, currency = in.AmountMinor, in.Currency
	}
	pipeline, stage := &current.PipelineID, &current.StageID
	if in.StageID != nil {
		pipeline, stage = nil, in.StageID
	}
	pipelineID, stageID, err := BirthStageTx(ctx, tx, pipeline, stage)
	if err != nil {
		return CreateDealInput{}, err
	}
	company := ids.From[ids.CompanyKind](current.CompanyID)
	birth := CreateDealInput{
		Name: name, AmountMinor: amount, Currency: currency,
		PipelineID: pipelineID, StageID: stageID, CompanyID: &company,
		Source: dealScoutSource, ExpectedClose: current.CloseDate,
		CloseDateProvisional: current.CloseDate != nil,
	}
	if in.CloseDate != nil {
		birth.ExpectedClose, birth.CloseDateProvisional = in.CloseDate, false
	}
	if in.OwnerID != nil {
		if err := auth.EnsureAssignee(ctx, tx, *in.OwnerID); err != nil {
			return CreateDealInput{}, err
		}
		owner := ids.From[ids.UserKind](*in.OwnerID)
		birth.OwnerID, birth.OwnerExact = &owner, true
	}
	return birth, nil
}

// applyAcceptEffects files the evidence messages under the deal and
// acknowledges the contributing signals. Each message is filed inside a
// savepoint: one the rep may not move is left where it is and reported, and
// the rest still land.
func (s *Store) applyAcceptEffects(ctx context.Context, tx pgx.Tx, current Suggestion, dealID ids.UUID) ([]ids.UUID, int, error) {
	unlinked := []ids.UUID{}
	acknowledged := 0
	for _, e := range current.Evidence {
		if e.SignalID != nil {
			moved, err := s.acknowledgeIfAllowed(ctx, tx, *e.SignalID)
			if err != nil {
				return nil, 0, err
			}
			if moved {
				acknowledged++
			}
			continue
		}
		activity, err := evidenceActivity(e)
		if err != nil {
			return nil, 0, err
		}
		linked, err := s.linkInSavepoint(ctx, tx, activity, dealID)
		if err != nil {
			return nil, 0, err
		}
		if !linked {
			unlinked = append(unlinked, activity)
		}
	}
	return unlinked, acknowledged, nil
}

// acknowledgeIfAllowed settles a signal only when the rep could have settled
// it themselves.
func (s *Store) acknowledgeIfAllowed(ctx context.Context, tx pgx.Tx, signalID ids.UUID) (bool, error) {
	if !auth.Allows(ctx, "signal", principal.ActionUpdate) {
		return false, nil
	}
	if err := auth.EnsureSignalVisible(ctx, tx, signalID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return s.suggestionEffects.AcknowledgeSignal(ctx, tx, signalID)
}

// evidenceActivity is the message a meeting or document item stands for.
func evidenceActivity(e SuggestionEvidenceItem) (ids.UUID, error) {
	switch {
	case e.ActivityID != nil:
		return *e.ActivityID, nil
	case e.carrier != nil:
		return *e.carrier, nil
	}
	return ids.Nil, fmt.Errorf("deals: a %s evidence item names no message: %w", e.Kind, ErrSuggestionDraftInvalid)
}

// linkInSavepoint files one message under the deal, and rolls back only that
// message when the rep may not move it.
func (s *Store) linkInSavepoint(ctx context.Context, tx pgx.Tx, activityID, dealID ids.UUID) (bool, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("deals: opening a savepoint: %w", err)
	}
	err = s.suggestionEffects.LinkActivityToDeal(ctx, savepoint, activityID, dealID)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
			return false, fmt.Errorf("deals: rolling back a refused link: %w", rollbackErr)
		}
		return false, nil
	}
	if err != nil {
		return false, errors.Join(err, savepoint.Rollback(ctx))
	}
	return true, savepoint.Commit(ctx)
}

// recordDecision moves an open suggestion to its decided state, with the
// audit row and the event in the same transaction.
func recordDecision(ctx context.Context, tx pgx.Tx, id ids.UUID, state string, dealID *ids.UUID) error {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE deal_suggestion SET state = $2, accepted_deal_id = $3, decided_by = $4, decided_at = now()
		 WHERE id = $1 AND state = 'open'`,
		id, state, dealID, storekit.UUIDOrNil(actor.UserID))
	if err != nil {
		return fmt.Errorf("deals: recording the decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &SuggestionDecidedError{State: "decided"}
	}
	after := map[string]any{columnState: state}
	if dealID != nil {
		after["accepted_deal_id"] = *dealID
	}
	auditID, err := storekit.Audit(ctx, tx, "update", suggestionEntity, id,
		map[string]any{columnState: SuggestionOpen}, after)
	if err != nil {
		return fmt.Errorf("deals: auditing the decision: %w", err)
	}
	if state == SuggestionAccepted {
		err = storekit.EmitPipelinePayload(ctx, tx, auditID, crmcontracts.InternalEventDealSuggestionAccepted{})
	} else {
		err = storekit.EmitPipelinePayload(ctx, tx, auditID, crmcontracts.InternalEventDealSuggestionDismissed{})
	}
	if err != nil {
		return fmt.Errorf("deals: emitting the decision: %w", err)
	}
	return nil
}
