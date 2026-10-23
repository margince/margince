// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// A commitment proposal is for the one colleague its payload names
// (approvalsubject.CommitmentAskedOf). Rows staged before that rule may name
// nobody, or a member who did not make the promise. These close them, and tell
// the notice sweep which copies went to somebody the rule does not name.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unaskedCommitmentReason is the audit reason on a withdrawn commitment.
const unaskedCommitmentReason = "the colleague who made this promise cannot hold the task"

// AskedPair is one notice's question: is this approval still asked of this member?
type AskedPair struct {
	Approval  ids.ApprovalID
	Recipient ids.UUID
}

// StillAskedOf answers, for system maintenance, whether each approval is
// pending and still put to the member its notice went to. That is
// PendingAskedOfInTx's question without the grants the recipient's own session
// would add. A row that names no decider is asked of nobody. The rows stay
// share-locked for the caller's transaction, so a decider change waits for it.
func (s *Service) StillAskedOf(ctx context.Context, tx pgx.Tx, pairs []AskedPair) (map[AskedPair]bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if len(pairs) > 1000 {
		return nil, apperrors.ErrInvalidArgument
	}
	out := make(map[AskedPair]bool, len(pairs))
	wanted := make([]ids.ApprovalID, 0, len(pairs))
	for _, pair := range pairs {
		out[pair] = false
		wanted = append(wanted, pair.Approval)
	}
	if len(pairs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM approval WHERE id = ANY(@ids) `+lockOrder+` FOR SHARE`,
		pgx.StrictNamedArgs{"ids": wanted})
	if err != nil {
		return nil, fmt.Errorf("approvals: reading who pending approvals are asked of: %w", err)
	}
	stored, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) { return scan(r) })
	if err != nil {
		return nil, err
	}
	byID := make(map[ids.ApprovalID]row, len(stored))
	for _, a := range stored {
		byID[a.ID] = a
	}
	now := s.now()
	for _, pair := range pairs {
		a, found := byID[pair.Approval]
		out[pair] = found && a.PendingAt(now) && askedOf(a, pair.Recipient)
	}
	return out, nil
}

// askedOf is whether a row is put to this member: it names them, and the
// narrowing every reader applies does not withhold it from them.
func askedOf(a row, member ids.UUID) bool {
	return requestedFor(a, member) && !withheldFromOtherSeats(principal.Principal{UserID: member}, a)
}

// unaskedCommitmentScope selects the live commitment proposals; the batch
// decides per row whether its payload names a member, because that is a Go rule.
const unaskedCommitmentScope = `FROM approval WHERE kind = @commitment AND status = 'pending' AND expires_at > now()`

// EstimateUnaskedCommitments bounds the withdrawal task's work.
func EstimateUnaskedCommitments(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id `+unaskedCommitmentScope+` LIMIT @cap) candidates`,
		pgx.NamedArgs{"commitment": kindCommitmentTask, "cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

// WithdrawUnaskedCommitmentsTx closes each pending commitment proposal whose
// payload names nobody it may be put to. The withdrawal is audited and sends
// no event, so no webhook hears about a request that should not have reached it.
func WithdrawUnaskedCommitmentsTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+` `+unaskedCommitmentScope+` AND id > @after ORDER BY id LIMIT @batch_limit`,
		pgx.NamedArgs{"commitment": kindCommitmentTask, "after": after, "batch_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("approvals: reading pending commitments: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) { return scan(r) })
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	changed := 0
	for _, c := range candidates {
		scanned = append(scanned, c.ID.UUID)
		if askedAsStored(c) {
			continue
		}
		withdrawn, err := withdrawInTx(ctx, tx, c.ID, unaskedCommitmentReason)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if withdrawn {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// askedAsStored reports whether a stored commitment is put to the one member
// its payload names, and was staged on their behalf.
func askedAsStored(a row) bool {
	asked, named := approvalsubject.CommitmentAskedOf(a.ProposedChange)
	return named && askedOf(a, asked) && (a.OnBehalfOf == nil || a.OnBehalfOf.UUID == asked)
}
