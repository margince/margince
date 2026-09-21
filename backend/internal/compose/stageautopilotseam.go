// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Binding stage automation's judgement to the auto-applier.
//
// The applier knows an approval; the judgement is about a TRANSITION. This
// file is the translation, and it lives in compose because it crosses two
// modules: approvals holds the staged card, deals holds the rule.
//
// It answers only. What applies the move is the ordinary approved effect, run
// under the authority this seam's caller has already bound.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stageProgressionPolicy answers whether one staged move may apply without
// asking.
//
// THE PAYLOAD IS READ HERE, not carried from staging, and the transition comes
// out of it. A card names the move it proposes; the rule that governs that
// move is looked up now, in the transaction that would apply it, because a
// threshold read at staging authorizes a move on numbers that were true hours
// ago.
//
// A card that cannot be read answers NO rather than an error: an unreadable
// payload is a card a contact should look at, and failing the sweep over it
// would park every proposal behind it.
func stageProgressionPolicy(pool *pgxpool.Pool, store *deals.Store) kindPolicy {
	return func(ctx context.Context, approvalID ids.ApprovalID) (bool, error) {
		payload, err := stagedChange(ctx, pool, approvalID)
		if err != nil {
			return false, err
		}
		change, err := deals.ReadProgressionChange(payload)
		if err != nil {
			// Not applied, and not a sweep failure. An unreadable card is one
			// a CONTACT should look at: failing here would end the pass on it,
			// and because the batch is ordered oldest-first that one card
			// would park every other transition's automation behind it until
			// it expired.
			//
			// Logged rather than discarded, because a card the product cannot
			// read is a defect somewhere upstream and a silent skip is how it
			// stays one.
			slog.Default().WarnContext(ctx,
				"stage progression card cannot be read, so it will not apply automatically",
				"approval_id", approvalID, "error", err)
			return false, nil
		}
		verdict, err := store.MayAutoApplyStageMove(
			ctx, change.DealID, change.FromStageID, change.ToStageID)
		if err != nil {
			return false, err
		}
		if verdict.Mode != deals.ModeAuto {
			return false, nil
		}
		// A move the card itself marks as needing judgement never applies,
		// however good the transition's record. ConfirmFirst is set for a
		// closing stage, a skip, or a criterion resting on something merely
		// proposed — facts about THIS move that no amount of history about
		// the transition answers.
		return !change.ConfirmFirst, nil
	}
}

// stagedChange reads the payload a card is standing on.
//
// ONE column, because a modify-then-approve edit REPLACES proposed_change in
// place under a freshly computed diff_hash (approvals/decide.go's
// applyEditedPayload) rather than landing beside it. So this reads what the
// card would apply, edited or not, and there is no second column to prefer.
func stagedChange(
	ctx context.Context, pool *pgxpool.Pool, approvalID ids.ApprovalID,
) (json.RawMessage, error) {
	var proposed json.RawMessage
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT proposed_change FROM approval
			 WHERE id = $1 AND status = 'pending'`,
			approvalID).Scan(&proposed)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Decided by a contact between the sweep listing this card and reading
		// it. ErrNotFound rather than the raw pgx error, because that is what
		// refusesThisRow matches: a wrapped ErrNoRows ends the whole pass, and
		// since the batch is ordered oldest-first, one card somebody happened
		// to answer would park every other transition's automation behind it.
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("compose: read the staged stage move: %w", err)
	}
	return proposed, nil
}

// governedKindPolicies builds the seam for each admin-governed kind.
//
// It CHECKS ITSELF against the module's own set rather than trusting the map
// below to stay complete, so a kind added to AdminGovernedAutoKinds without a
// seam fails at startup instead of silently never applying. An unwired
// governed kind is invisible otherwise: the applier refuses it, correctly, and
// nothing says the feature was never connected.
//
// Held by: TestEveryAdminGovernedKindHasAPolicySeam
// (backend/gates/governedkindseams_test.go)
func governedKindPolicies(pool *pgxpool.Pool, store *deals.Store) map[string]kindPolicy {
	seams := map[string]kindPolicy{
		deals.StageProgressionKind: stageProgressionPolicy(pool, store),
	}
	for kind := range approvals.AdminGovernedAutoKinds {
		if _, wired := seams[kind]; !wired {
			panic(fmt.Sprintf(
				"compose: %q is admin-governed but has no policy seam, so it would "+
					"never apply and nothing would say why", kind))
		}
	}
	return seams
}
