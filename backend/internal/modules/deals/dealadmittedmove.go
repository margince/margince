// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The premise an unattended deal move was admitted on, re-derived where it is
// acted on.
//
// Its own file because it is its own concept: the advance path is about deriving
// a transition, and this is about whether the AUTHORITY to make it unattended
// still describes the move being made. The two answer to different racing
// actors — the deal's version pin to the agent, this to a human admin editing
// stage configuration — and reading them apart is what keeps either legible.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// autoExecutedMoveIsOpenToOpen is the tier rule this store re-checks, and it is
// a DECLARED MIRROR of agents.advanceDealTier — the resolver that admits an
// unattended deal move exactly when both endpoints are open.
//
// Spelled twice because a module never imports a sibling: the resolver lives in
// the agents module and this is the deals module, so the two cannot share the
// function. What holds them equal is a gate rather than a convention —
// backend/gates/dealmovemirror_test.go fails when either spelling moves — for
// the reason AGENTS.md gives about an invariant spelled on both sides of a
// wire: fixing one side alone can be a regression rather than half a fix.
func autoExecutedMoveIsOpenToOpen(source, target StageSemantic) bool {
	return source == SemanticOpen && target == SemanticOpen
}

// refuseAMoveTheGateDidNotAdmit re-derives, inside the writing transaction, the
// premise the tier gate admitted this call on.
//
// The gate auto-executes a deal move only when it can prove BOTH endpoints
// open, and it proves that by reading two STAGE rows. The version pin it
// carries forward binds the DEAL, and a stage row is mutable independently of
// any deal — so an admin changing the target stage's semantic to `won` in the
// window between the gate's read and this write leaves the pin perfectly
// satisfied and closes a deal on a verdict about an open-to-open move. The
// mirror case reopens a closed one.
//
// The racing actor is a human admin rather than the agent, so this is not the
// window the deal's own pin was added for; it is the one that pin cannot see.
// Re-deriving here is what makes the premise true at the moment it is acted on
// instead of at the moment it was read.
//
// It costs nothing on any other path. A call the gate did not admit unattended
// carries no pin: an approved move was judged by a human who saw the sentence,
// and a human's own move is not governed by the tier model at all.
func refuseAMoveTheGateDidNotAdmit(
	ctx context.Context, tx pgx.Tx, current crmcontracts.Deal, toStage ids.StageID,
) error {
	if _, autoExecuted := auth.AutoExecutePin(ctx); !autoExecuted {
		return nil
	}
	source, target, err := lockedMoveSemantics(ctx, tx,
		ids.From[ids.StageKind](ids.UUID(*current.StageId)), toStage)
	if err != nil {
		return err
	}
	if autoExecutedMoveIsOpenToOpen(source, target) {
		return nil
	}
	return fmt.Errorf(
		"this move was admitted unattended as open-to-open and is now %s-to-%s — "+
			"a stage's semantic changed after the gate read it: %w",
		source, target, apperrors.ErrVersionSkew)
}

// lockedMoveSemantics reads both endpoints' semantics under FOR SHARE, which is
// what makes the answer still true when the deal is written.
//
// FOR SHARE and not the FOR KEY SHARE the ordinary target lookup takes. That one
// is deliberately the weakest lock that conflicts with a stage REMOVAL, so a
// rename or a probability edit runs alongside a deal moving (stageremoval.go
// says so) — and a semantic change is exactly such an edit, taking FOR NO KEY
// UPDATE, which FOR KEY SHARE does not conflict with. Under it, the value this
// check reads can be replaced before the deal write commits, and the re-check
// would have narrowed the window rather than closed it.
//
// The stronger lock is taken ONLY here, on the auto-executed path. Applying it
// to every advance would make an ordinary stage rename wait behind every deal
// move, which is the concurrency the weaker lock was chosen to keep.
//
// One statement over both ids, ordered, rather than two reads: two concurrent
// moves in opposite directions would otherwise take the same pair of rows in
// opposite orders. Both sides here run this same statement, so both take them
// the same way round.
func lockedMoveSemantics(
	ctx context.Context, tx pgx.Tx, from, to ids.StageID,
) (source, target StageSemantic, err error) {
	rows, err := tx.Query(ctx,
		`SELECT id, semantic FROM stage WHERE id = ANY($1) AND archived_at IS NULL
		  ORDER BY id FOR SHARE`,
		[]ids.UUID{from.UUID, to.UUID})
	if err != nil {
		return "", "", fmt.Errorf("lock the move's stages: %w", err)
	}
	defer rows.Close()
	semantics := map[ids.UUID]StageSemantic{}
	for rows.Next() {
		var id ids.UUID
		var semantic string
		if err := rows.Scan(&id, &semantic); err != nil {
			return "", "", fmt.Errorf("read a move endpoint's semantic: %w", err)
		}
		semantics[id] = StageSemantic(semantic)
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("read the move's stages: %w", err)
	}
	// A missing endpoint is a stage archived under this move. It is refused
	// rather than read as "not open", because the two are different answers and
	// only one of them is true: nothing established what that stage was.
	source, hasSource := semantics[from.UUID]
	target, hasTarget := semantics[to.UUID]
	if !hasSource || !hasTarget {
		return "", "", fmt.Errorf(
			"a stage this move names is no longer live, so the premise it was admitted on "+
				"cannot be re-checked: %w", apperrors.ErrVersionSkew)
	}
	return source, target, nil
}
