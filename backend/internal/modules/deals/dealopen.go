// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Margince opening a deal from Deal Scout's evidence for the company's owner,
// and the owner taking that back. The deal is born through the writer a
// human's acceptance uses. Undo archives it and dismisses the suggestion, so
// the same evidence does not open it again.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DealOpenCorrection is the correction an opened deal records, and DealOpenKind
// the owner's switch for it.
const (
	DealOpenCorrection = "deal_open"
	DealOpenKind       = "deal_open"
)

// SuggestionKindOpenDeal is the suggestion kind Margince may open by itself.
const SuggestionKindOpenDeal = "open_deal"

// OpenSuggestedDealInput is one opening Margince decided on.
type OpenSuggestedDealInput struct {
	SuggestionID ids.UUID
	// Owner is the company's owner when the opening was decided. A company
	// that changed hands since is left to the suggestion.
	Owner ids.UUID
	// Basis is the one line the owner's receipt shows: the words and the source.
	Basis string
	// SwitchOn asks again, under the company lock, whether the owner lets
	// Margince open deals. The switch is not this module's.
	SwitchOn func(ctx context.Context, tx pgx.Tx) (bool, error)
	// EvidenceHolds locks every cited item and asks whether the whole workspace
	// may still read it. The visibility rule is Deal Scout's, not this module's.
	EvidenceHolds func(ctx context.Context, tx pgx.Tx, evidence []SuggestionEvidence) (bool, error)
	// Amount answers the amount a cited document states in its own words, read
	// at write time, or nil to open the deal with none.
	Amount func(ctx context.Context, tx pgx.Tx, evidence []SuggestionEvidence) (*int64, *string, error)
	// Record writes the entry on the deal's timeline, in the same transaction.
	Record func(ctx context.Context, tx pgx.Tx, deal, correction ids.UUID) error
	// AsOwner binds the owner's agent as of the opening's transaction, so a
	// role change or suspension that committed before it stops the opening.
	AsOwner OwnerAuthorityTx
}

// OpenSuggestedDeal opens the suggested deal for the company's owner, and
// answers the deal it opened. A suggestion that was decided, superseded, or
// whose company changed owner or got a deal since opens nothing: zero, no error.
func (s *Store) OpenSuggestedDeal(ctx context.Context, in OpenSuggestedDealInput) (ids.UUID, error) {
	if err := RequireSuggestionDecision(ctx, false); err != nil {
		return ids.Nil, err
	}
	if s.suggestionEffects == nil {
		return ids.Nil, errors.New("deals: opening a suggested deal needs its effects wired")
	}
	var opened ids.UUID
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		ctx, may, err := actAsOwnerNow(ctx, tx, in.AsOwner, func(ctx context.Context) error {
			return RequireSuggestionDecision(ctx, false)
		})
		if err != nil || !may {
			return err
		}
		current, ready, err := s.openableSuggestion(ctx, tx, in)
		if err != nil || !ready {
			return err
		}
		owner := in.Owner
		pick := AcceptSuggestionInput{OwnerID: &owner, NoAmount: true}
		amount, currency, err := in.Amount(ctx, tx, citedEvidence(current))
		if err != nil {
			return err
		}
		if amount != nil && currency != nil {
			pick.NoAmount, pick.AmountMinor, pick.Currency = false, amount, currency
		}
		birth, err := s.acceptedDealInput(ctx, tx, current, pick)
		if err != nil {
			return err
		}
		birth.auditEvidence = map[string]any{
			"basis": in.Basis, "basis_owner": in.Owner.String(), CorrectionKindEvidenceKey: DealOpenCorrection,
		}
		deal, err := s.CreateDealTx(ctx, tx, birth, CustomColumns{})
		if err != nil {
			return err
		}
		dealID := ids.UUID(deal.Id)
		if _, _, err := s.applyAcceptEffects(ctx, tx, current, dealID); err != nil {
			return err
		}
		if err := recordDecision(ctx, tx, current.ID, SuggestionAccepted, &dealID); err != nil {
			return err
		}
		correction, err := recordDealOpen(ctx, tx, ids.From[ids.DealKind](dealID))
		if err != nil {
			return err
		}
		opened = dealID
		return in.Record(ctx, tx, dealID, correction)
	})
	return opened, err
}

// openableSuggestion locks the suggestion and its company and asks again what
// the decision rested on. The suggestion is still open and of the kind
// Margince opens. Its evidence and buyer still stand. The company is still
// free and owned by the same member, and the owner's switch is still on.
func (s *Store) openableSuggestion(ctx context.Context, tx pgx.Tx, in OpenSuggestedDealInput) (Suggestion, bool, error) {
	current, err := openSuggestionForDecision(ctx, tx, in.SuggestionID)
	if errors.As(err, new(*SuggestionDecidedError)) || errors.Is(err, apperrors.ErrNotFound) {
		return Suggestion{}, false, nil
	}
	if err != nil || current.Kind != SuggestionKindOpenDeal {
		return Suggestion{}, false, err
	}
	for _, superseded := range []func(context.Context, pgx.Tx, Suggestion) (bool, error){
		supersedeIfEvidenceChanged, s.supersedeIfBuyerChanged, supersedeIfCompanyTaken,
	} {
		if gone, err := superseded(ctx, tx, current); err != nil || gone {
			return Suggestion{}, false, err
		}
	}
	if held, err := in.EvidenceHolds(ctx, tx, citedEvidence(current)); err != nil || !held {
		return Suggestion{}, false, err
	}
	var owner *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM company WHERE id = $1`, current.CompanyID).Scan(&owner); err != nil {
		return Suggestion{}, false, fmt.Errorf("deals: reading the suggestion company's owner: %w", err)
	}
	if owner == nil || *owner != in.Owner {
		return Suggestion{}, false, nil
	}
	on, err := in.SwitchOn(ctx, tx)
	return current, on, err
}

// citedEvidence is the suggestion's evidence without the reader's titles.
func citedEvidence(s Suggestion) []SuggestionEvidence {
	out := make([]SuggestionEvidence, 0, len(s.Evidence))
	for _, item := range s.Evidence {
		out = append(out, item.SuggestionEvidence)
	}
	return out
}

// recordDealOpen writes the correction row for an opened deal, naming the
// deal's create audit row.
func recordDealOpen(ctx context.Context, tx pgx.Tx, deal ids.DealID) (ids.UUID, error) {
	var created ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id FROM audit_log WHERE entity_type = 'deal' AND entity_id = $1 AND action = 'create'
		 ORDER BY occurred_at DESC LIMIT 1`, deal).Scan(&created); err != nil {
		return ids.Nil, fmt.Errorf("deals: reading the opened deal's audit row: %w", err)
	}
	recorded, err := recordCorrection(ctx, tx, DealCorrection{
		DealID: deal, AuditLogID: created, Correction: DealOpenCorrection, Fields: []string{dealNameColumn},
	}, nil)
	return recorded.ID, err
}

// DealOpenUndoNeedsConfirmError refuses to archive an opened deal, or to close
// a reopened one again, that the team has worked on since, until the owner
// confirms.
type DealOpenUndoNeedsConfirmError struct {
	Notes, Tasks, Edits int
	// Work names the notes, tasks and edits counted. A confirmation
	// sends it back, and holds only while the team's work is still that.
	Work string
}

func (e *DealOpenUndoNeedsConfirmError) Error() string {
	return fmt.Sprintf("this deal now has %d notes, %d tasks and %d edits from the team; confirm to take Margince's change back anyway",
		e.Notes, e.Tasks, e.Edits)
}

// Unwrap answers a conflict: the deal is in a state that refuses the Undo.
func (e *DealOpenUndoNeedsConfirmError) Unwrap() error { return apperrors.ErrConflict }

// revertDealOpen archives the deal Margince opened and dismisses the
// suggestion it came from, in the caller's transaction. Work a human added
// since needs a confirmation naming that work.
func (s *Store) revertDealOpen(ctx context.Context, tx pgx.Tx, c DealCorrection, ifVersion *int64, confirmedFor string) error {
	if err := storekit.EnsureIfMatch(ctx, tx, dealTable, c.DealID.UUID, ifVersion); err != nil {
		return err
	}
	work, err := teamWorkSince(ctx, tx, c)
	if err != nil {
		return err
	}
	if work.Notes+work.Tasks+work.Edits > 0 && work.Work != confirmedFor {
		return &work
	}
	if err := s.ArchiveDealTx(ctx, tx, c.DealID, nil); err != nil {
		return err
	}
	if err := dismissOpenedSuggestion(ctx, tx, c.DealID); err != nil {
		return err
	}
	var archived ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id FROM audit_log WHERE entity_type = 'deal' AND entity_id = $1 AND action = 'archive'
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`, c.DealID).Scan(&archived); err != nil {
		return fmt.Errorf("deals: reading the archive's audit row: %w", err)
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	stamped, err := markReversed(ctx, tx, c.ID, archived, by)
	if err != nil {
		return err
	}
	if !stamped {
		return &CorrectionReversalError{Reason: alreadyTakenBack}
	}
	return nil
}

// teamWorkSince counts what humans added to the deal after Margince opened or
// reopened it.
// That is notes and tasks filed on it since, however old, and edits to the deal.
// The link rows are held, so nothing is filed between the count and the archive.
func teamWorkSince(ctx context.Context, tx pgx.Tx, c DealCorrection) (DealOpenUndoNeedsConfirmError, error) {
	var out DealOpenUndoNeedsConfirmError
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.kind = 'task'
		  FROM activity a JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		 WHERE a.archived_at IS NULL AND starts_with(a.captured_by, $3)
		   AND (a.created_at > $2 OR l.created_at > $2)
		 ORDER BY a.id
		   FOR SHARE OF l`, c.DealID, c.AppliedAt, principal.HumanIDPrefix)
	if err != nil {
		return out, fmt.Errorf("deals: counting the team's work on the opened deal: %w", err)
	}
	type filed struct {
		id   ids.UUID
		task bool
	}
	work, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (filed, error) {
		var f filed
		return f, row.Scan(&f.id, &f.task)
	})
	if err != nil {
		return out, fmt.Errorf("deals: counting the team's work on the opened deal: %w", err)
	}
	sum := sha256.New()
	seen := map[ids.UUID]bool{}
	for _, f := range work {
		if seen[f.id] {
			continue
		}
		seen[f.id] = true
		if f.task {
			out.Tasks++
		} else {
			out.Notes++
		}
		sum.Write([]byte("a:" + f.id.String() + "|"))
	}
	edits, err := tx.Query(ctx, `
		SELECT id FROM audit_log
		 WHERE entity_type = 'deal' AND entity_id = $1 AND occurred_at > $2 AND actor_type = $3
		 ORDER BY id`, c.DealID, c.AppliedAt, string(principal.PrincipalHuman))
	if err != nil {
		return out, fmt.Errorf("deals: counting the team's edits to the opened deal: %w", err)
	}
	audits, err := pgx.CollectRows(edits, pgx.RowTo[ids.UUID])
	if err != nil {
		return out, fmt.Errorf("deals: counting the team's edits to the opened deal: %w", err)
	}
	for _, id := range audits {
		sum.Write([]byte("e:" + id.String() + "|"))
	}
	out.Edits = len(audits)
	out.Work = hex.EncodeToString(sum.Sum(nil))
	return out, nil
}

// dismissOpenedSuggestion records the workspace's dismissal of the suggestion
// the deal came from. Its decided_at is the new floor: the same evidence does
// not open a deal again, and newer evidence does.
func dismissOpenedSuggestion(ctx context.Context, tx pgx.Tx, deal ids.DealID) error {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	var id ids.UUID
	err = tx.QueryRow(ctx, `
		UPDATE deal_suggestion SET state = 'dismissed', accepted_deal_id = NULL, decided_by = $2, decided_at = now()
		 WHERE accepted_deal_id = $1 AND state = 'accepted' AND kind = $3
		RETURNING id`, deal, storekit.UUIDOrNil(actor.UserID), SuggestionKindOpenDeal).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("deals: dismissing the opened deal's suggestion: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", suggestionEntity, id,
		map[string]any{columnState: SuggestionAccepted, "accepted_deal_id": deal.UUID},
		map[string]any{columnState: SuggestionDismissed, "dismiss_reason": dismissReasonNotADeal})
	if err != nil {
		return fmt.Errorf("deals: auditing the dismissal: %w", err)
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, auditID, crmcontracts.InternalEventDealSuggestionDismissed{}); err != nil {
		return fmt.Errorf("deals: emitting the dismissal: %w", err)
	}
	return nil
}
