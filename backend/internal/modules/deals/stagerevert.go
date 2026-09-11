// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Taking back a stage move the product made by itself.
//
// A SEPARATE VERB from the general undo path, and the reason is structural
// rather than a preference. The replay engine restores a record by writing its
// fields back, which works for a corrected close date or a renamed
// company — one column, one restore. A stage move wrote deal_stage_history
// and the progression ledger BESIDE the column, so writing stage_id back would
// leave the history saying the deal is somewhere it is not. The engine refuses
// `advance_stage` by name for exactly this, so the move carries its own undo.
//
// What a reversal claims is narrow and worth stating: the deal should not have
// moved. It does NOT claim the evidence was wrong. A rep may agree with every
// observation the card cited and still want the deal where it was, and a
// reversal that quietly refuted the claims would withdraw evidence nobody
// disputed. Evidence is corrected through its own path.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UndoWindowClosedError refuses a move that can no longer be taken back.
//
// Its own type rather than a bare conflict, because the three ways this verb
// declines are different situations for the caller: a window that has closed
// (move the deal by hand instead), a move a contact made (there is nothing
// automatic to undo), and one already reversed (somebody got there first).
//
// Mapped to 409 by writeUndoConflict in handlers.go, the way this package maps
// every pre-checked conflict. Nothing about the request is malformed — the
// record is in a state that refuses it — so it is not a 422.
type UndoWindowClosedError struct{ Reason string }

func (e *UndoWindowClosedError) Error() string { return e.Reason }

// RevertStageProgression moves a deal back to the stage an automatic move took
// it from.
//
// ONE TRANSACTION for all three writes: the deal moves back, a new history row
// records the move back, and the ledger row is marked reversed. Split apart,
// a failure between them leaves a deal at one stage with a ledger saying it is
// at another — and the ledger is what the launch gate reads to decide whether
// this transition may keep running.
//
// The stage the deal came from is read from the LEDGER, not supplied by the
// caller. A caller who could name the target could send a deal anywhere by
// calling this an undo, and undo means precisely "where it was".
func (s *Store) RevertStageProgression(
	ctx context.Context, dealID ids.DealID, approvalID ids.UUID,
) (crmcontracts.Deal, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return crmcontracts.Deal{}, err
	}
	// Resolved before the transaction opens: the catalog reads through its own
	// connection, and a second connection inside a transaction can deadlock
	// undetectably against a lock that transaction holds.
	active, err := s.activeColumns(ctx)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	var out crmcontracts.Deal
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		// THE DEAL FIRST, then the ledger row. The other door onto a reversal
		// — a contact moving the deal back by hand — takes the deal's lock in
		// advanceOnTx and then the ledger row's, so taking them the other way
		// round here is a cycle: one undo and one manual move back, running at
		// once, each holding what the other waits for. Postgres resolves that
		// by aborting one of them, which reaches a rep as an unexplained
		// failure to undo.
		if err := lockDealForReversal(ctx, tx, dealID); err != nil {
			return err
		}
		// VISIBILITY BEFORE ANY CONFLICT. The three refusals below say
		// different things — the window has closed, a contact made this move,
		// it is already taken back — and each is a fact about a record. Asked
		// after them, a caller with the object grant but no row scope could
		// tell an eligible hidden move from an expired one by the 409 they
		// got back, and learn that a deal exists by the shape of its refusal.
		//
		// advanceOnTx checks writability again on its own; this is not that.
		// It is the existence-hiding the module keeps everywhere: a row-scope
		// miss is a 404.
		if err := auth.EnsureWritable(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		move, err := lockReversibleMove(ctx, tx, dealID, approvalID, s.clock())
		if err != nil {
			return err
		}
		// The paperless-win argument the deal ALREADY holds, carried back with
		// it. An undo can land a deal on a won stage — the move being taken
		// back was a move away from one — and a win with no agreement behind
		// it is refused unless somebody says why. That somebody already did,
		// when the deal was first won; making them retype it to undo a move
		// the PRODUCT made would refuse the undo for a reason the record
		// answers.
		wonReason, wonDetail, err := heldWinReason(ctx, tx, dealID)
		if err != nil {
			return err
		}
		out, err = s.advanceOnTx(ctx, tx, dealID, AdvanceDealInput{
			ToStageID:                move.FromStageID,
			WonWithoutContractReason: wonReason,
			WonWithoutContractDetail: wonDetail,
			// No ApprovalID. This move is not made THROUGH a card — it is a
			// contact overruling one, and recording the card here would tell
			// readProtection that the rep agreed with the product.
			//
			// IsExplicitUndo suppresses the automatic reversal detection: it
			// would find this very move and mark it, and markMoveReversed
			// below would mark it again — one reversal counted twice, and the
			// safety rate reading double.
			IsExplicitUndo: true,
			// What this move undoes, so readProtection knows the deal has
			// already had this argument and the product does not propose the
			// same move again.
			ReversalOf: move.HistoryID,
		}, active)
		if err != nil {
			return err
		}
		return markMoveReversed(ctx, tx, move, s.clock())
	})
	return out, err
}

// heldWinReason answers the paperless-win argument the deal already carries.
//
// Read from the deal rather than asked of the caller: this is an UNDO, and the
// argument for the state it returns to was made when the deal reached it. A
// deal that never claimed a paperless win answers nil for both, which is what
// an ordinary move needs.
func heldWinReason(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID,
) (*string, *string, error) {
	var reason, detail *string
	if err := tx.QueryRow(ctx, `
		SELECT won_without_contract_reason, won_without_contract_detail
		  FROM deal WHERE id = $1`, dealID).Scan(&reason, &detail); err != nil {
		return nil, nil, fmt.Errorf("read the deal's win-evidence claim: %w", err)
	}
	return reason, detail, nil
}

// reversibleMove is the ledger row a revert is about.
type reversibleMove struct {
	ID          ids.UUID
	DealID      ids.DealID
	FromStageID ids.StageID
	ToStageID   ids.StageID
	PipelineID  ids.PipelineID
	// HistoryID is the deal_stage_history row the original move wrote, so the
	// move back can name what it undid. Nil when the history row is gone,
	// which does not stop the undo — the ledger is the record that matters and
	// a missing history row is not a reason to leave a deal where it should
	// not be.
	HistoryID *ids.UUID
}

// lockDealForReversal takes the deal's row lock before anything else.
//
// The lock ORDER is the point, not the lock: every path that touches both a
// deal and its progression ledger has to take them in one order, and the
// ordinary advance takes the deal first. See RevertStageProgression.
//
// ErrNotFound for a deal that is gone or archived, which is the same answer a
// caller who cannot see it gets from the visibility gate below.
func lockDealForReversal(ctx context.Context, tx pgx.Tx, dealID ids.DealID) error {
	var id ids.DealID
	err := tx.QueryRow(ctx,
		`SELECT id FROM deal WHERE id = $1 AND archived_at IS NULL FOR UPDATE`,
		dealID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("hold the deal being moved back: %w", err)
	}
	return nil
}

// historyRowOf finds the history row an automatic move wrote.
//
// Matched by the APPROVAL, which advanceOnTx stamps onto the row for a move
// made through a card. That is exact where matching on the stage pair would
// not be: a deal can travel the same transition more than once.
//
// Answers ids.Nil and no error when there is no such row. A missing history
// row does not stop an undo — the ledger is the record that decides whether
// the move can be taken back, and a deal is not left where it should not be
// because one trail row is gone — so the caller tests the id rather than
// branching on an error.
func historyRowOf(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, approvalID ids.UUID,
) (ids.UUID, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM deal_stage_history
		 WHERE deal_id = $1 AND approval_id = $2
		 ORDER BY changed_at DESC LIMIT 1`, dealID, approvalID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("find the history row this undo takes back: %w", err)
	}
	return id, nil
}

// lockReversibleMove reads the move and holds it, refusing everything that is
// not an automatic move still inside its window.
//
// LOCKED, because the decision is made from what it read: two reverts of one
// move would both see it un-reversed, both move the deal back, and the second
// would write a history row for a move that did not happen.
//
// The window comes from the transition's OWN rule, read here rather than
// carried as a constant — an installation that gave a transition a longer undo
// window meant it for that transition. A move with no rule any more (the admin
// deleted it) falls back to the column default, because the window a move was
// made under is a promise to the contact it was made for.
func lockReversibleMove(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, approvalID ids.UUID, now time.Time,
) (reversibleMove, error) {
	var m reversibleMove
	var outcome string
	var decidedAt time.Time
	var reversedAt *time.Time
	var undoHours int
	err := tx.QueryRow(ctx, `
		SELECT o.id, o.deal_id, o.from_stage_id, o.to_stage_id, o.pipeline_id,
		       o.outcome, o.decided_at, o.reversed_at,
		       -- The window FROZEN on the move, then the rule's current one,
		       -- then the column default. A move made before the freeze column
		       -- existed reads the rule live, exactly as it always did.
		       coalesce(o.undo_window_hours, p.undo_window_hours, 72)
		  FROM stage_progression_outcome o
		  LEFT JOIN stage_progression_policy p
		    ON p.pipeline_id = o.pipeline_id
		   AND p.from_stage_id = o.from_stage_id
		   AND p.to_stage_id = o.to_stage_id
		 WHERE o.approval_id = $1 AND o.deal_id = $2
		 FOR UPDATE OF o`,
		approvalID, dealID).
		Scan(&m.ID, &m.DealID, &m.FromStageID, &m.ToStageID, &m.PipelineID,
			&outcome, &decidedAt, &reversedAt, &undoHours)
	if errors.Is(err, pgx.ErrNoRows) {
		// No such move, or one belonging to a different deal. ErrNotFound for
		// both, so a caller cannot learn that a progression exists by putting
		// its id against a deal they can see.
		return m, apperrors.ErrNotFound
	}
	if err != nil {
		return m, fmt.Errorf("read the move being taken back: %w", err)
	}
	if reversedAt != nil {
		return m, &UndoWindowClosedError{Reason: "this move has already been taken back"}
	}
	if outcome != ProgressionAutoApplied {
		// A human's approval is not undone here. Somebody read the card and
		// agreed; moving the deal back is an ordinary stage move they can make
		// themselves, and it is counted as a reversal on the same ledger.
		return m, &UndoWindowClosedError{
			Reason: "this move was made by a contact, so there is nothing automatic to take back",
		}
	}
	if now.After(decidedAt.Add(time.Duration(undoHours) * time.Hour)) {
		return m, &UndoWindowClosedError{
			Reason: "this move can no longer be taken back automatically; move the deal by hand instead",
		}
	}
	// THE DEAL MUST STILL BE WHERE THE MOVE PUT IT. Without this an old card
	// is a way to send a deal backwards from wherever it has since reached:
	// automatic A→B, a rep moves it B→C, and reverting the still-open A→B
	// card would take it C→A — a move nobody made and no undo of anything.
	//
	// Undo means "put it back", which is only meaningful while it is still
	// where it was put.
	var stage ids.StageID
	if err := tx.QueryRow(ctx,
		`SELECT stage_id FROM deal WHERE id = $1`, dealID).Scan(&stage); err != nil {
		return m, fmt.Errorf("read where the deal is now: %w", err)
	}
	if stage != m.ToStageID {
		return m, &UndoWindowClosedError{
			Reason: "this deal has moved on since, so there is no longer that move to take back",
		}
	}
	historyID, err := historyRowOf(ctx, tx, dealID, approvalID)
	if err != nil {
		return m, err
	}
	if historyID != ids.Nil {
		m.HistoryID = &historyID
	}
	return m, nil
}

// markMoveReversed closes the ledger row and says who did it.
//
// The outcome becomes `reversed` AND reversed_at is set, both. The report's
// safety numerator reads either — a row standing at reversed with a null
// timestamp is legal under the table's constraint — so writing one without the
// other would leave the two spellings of one fact disagreeing.
func markMoveReversed(
	ctx context.Context, tx pgx.Tx, move reversibleMove, now time.Time,
) error {
	actor := reversalActor(ctx)
	var reversed bool
	if err := tx.QueryRow(ctx, `
		UPDATE stage_progression_outcome
		   SET outcome = $2, reversed_at = now(), reversed_by = $3
		 WHERE id = $1 AND reversed_at IS NULL
		RETURNING true`,
		move.ID, ProgressionReversed, actor).Scan(&reversed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Reversed by a concurrent caller between the lock and here. It
			// cannot happen while the lock above is held, and answering it as
			// a conflict rather than a crash is what the row's own guard is
			// for if the lock is ever loosened.
			return &UndoWindowClosedError{Reason: "this move has already been taken back"}
		}
		return fmt.Errorf("record the move as taken back: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", progressionEntity, move.ID,
		map[string]any{progressionOutcomeKey: ProgressionAutoApplied},
		map[string]any{
			progressionOutcomeKey: ProgressionReversed,
			progressionDealKey:    move.DealID.String(),
			// The stage the deal went BACK to, so a reader of the trail sees
			// where the undo put it rather than only that one happened.
			progressionToKey: move.FromStageID.String(),
		})
	if err != nil {
		return fmt.Errorf("audit the move being taken back: %w", err)
	}
	if err := emitProgressionChanged(ctx, tx, auditID, move.DealID); err != nil {
		return err
	}
	// A reversal is the one outcome the safety ceiling exists to count, so the
	// rule is re-measured in the same transaction that records it. Left to the
	// decision path alone, an undo would never trip a ceiling: nothing decides
	// a card here, so the sweep that runs on decisions never sees it, and a
	// run of undos would age out of the window with the transition still
	// applying.
	return suspendIfRecordWentBadTx(ctx, tx, TransitionRef{
		PipelineID:  move.PipelineID,
		FromStageID: move.FromStageID,
		ToStageID:   move.ToStageID,
	}, now)
}

// reversalActor is the contact the undo is recorded against.
//
// Nil for a principal with no human behind it, matching the column's ON DELETE
// SET NULL: an instant whose contact has since been erased is what an anonymized
// workspace looks like, and the audit row still holds who.
func reversalActor(ctx context.Context) *ids.UUID {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID.IsZero() {
		return nil
	}
	id := p.UserID
	return &id
}

// countAManualMoveBackAsAReversal marks an automatic move that a contact has
// just undone by hand.
//
// THE SAME FACT AS THE UNDO BUTTON, reached the other way. A rep who disagrees
// with what the autopilot did can press Undo or simply drag the deal back, and
// only one of those routes going onto the ledger would leave the safety number
// dodgeable by the more obvious one — the transition would keep applying while
// contacts quietly corrected it all day.
//
// It does NOT refute the evidence, for the same reason the explicit undo does
// not: moving a deal back says it should not have moved, not that the criteria
// were misread.
//
// Narrow on purpose. It matches only a move whose direction exactly undoes an
// automatic one (this move's target is that move's origin, and vice versa),
// still inside that move's undo window, not already reversed. A deal that
// wanders forward and back over a week is not undoing anything.
func countAManualMoveBackAsAReversal(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID,
	fromStage ids.StageID, in AdvanceDealInput, now time.Time,
) error {
	if in.ApprovalID != nil {
		// This move came THROUGH a card, so it is somebody agreeing with the
		// product rather than overruling it — including the card that would
		// move the deal back again.
		return nil
	}
	// A CONTACT's move, named positively rather than as "not through a card".
	// An agent can advance a deal too, and its move carries no approval id
	// either — read as a human's, it would record a reversal nobody made,
	// attribute it to the agent's user, and let a machine reverse what another
	// machine did with no contact in the loop. The undo verb is human-only for
	// exactly that reason, and this door must not be the way around it.
	//
	// readProtection names its human the same way, and its comment says why:
	// an agent moving a deal is precisely what a human has not done.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return nil
	}
	var move reversibleMove
	var approvalID ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT o.id, o.deal_id, o.from_stage_id, o.to_stage_id, o.pipeline_id,
		       o.approval_id
		  FROM stage_progression_outcome o
		  LEFT JOIN stage_progression_policy p
		    ON p.pipeline_id = o.pipeline_id
		   AND p.from_stage_id = o.from_stage_id
		   AND p.to_stage_id = o.to_stage_id
		 WHERE o.deal_id = $1
		   AND o.outcome = $2
		   AND o.reversed_at IS NULL
		   -- The move being undone went from where this one is going, to where
		   -- this one is coming from. Both halves, or a deal moved forward
		   -- again after an undo would count as undoing something itself.
		   AND o.from_stage_id = $3
		   AND o.to_stage_id = $4
		   AND o.decided_at > $5::timestamptz - make_interval(
		           hours => coalesce(o.undo_window_hours, p.undo_window_hours, 72))
		 ORDER BY o.decided_at DESC
		 LIMIT 1
		 FOR UPDATE OF o`,
		dealID, ProgressionAutoApplied, in.ToStageID, fromStage, now).
		Scan(&move.ID, &move.DealID, &move.FromStageID, &move.ToStageID,
			&move.PipelineID, &approvalID)
	if errors.Is(err, pgx.ErrNoRows) {
		// The ordinary case: an ordinary stage move undoing nothing.
		return nil
	}
	if err != nil {
		return fmt.Errorf("look for an automatic move this one takes back: %w", err)
	}
	// The history row for THIS move is already written by the time detection
	// runs, so the link is stamped onto it rather than passed in. Same fact
	// either way: readProtection asks whether any history row on this deal
	// names a move it undid.
	undone, err := historyRowOf(ctx, tx, dealID, approvalID)
	if err != nil {
		return err
	}
	if undone != ids.Nil {
		if _, err := tx.Exec(ctx, `
			UPDATE deal_stage_history
			   SET reversal_of = $2
			 WHERE id = (SELECT id FROM deal_stage_history
			              WHERE deal_id = $1
			              ORDER BY changed_at DESC LIMIT 1)`,
			dealID, undone); err != nil {
			return fmt.Errorf("link this move to the one it undid: %w", err)
		}
	}
	return markMoveReversed(ctx, tx, move, now)
}
