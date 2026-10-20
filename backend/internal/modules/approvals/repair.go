// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RepairProposal keeps the reviewed snapshot separate from its authorized payload.
type RepairProposal struct {
	ID         ids.ApprovalID
	Kind       string
	TargetType *string
	TargetID   *ids.UUID
	Change     json.RawMessage
	Snapshot   string
	Pristine   bool
}

// RepairPageTx uses the inbox's decision authority before returning any payload.
func RepairPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, asOf time.Time) ([]RepairProposal, error) {
	if err := actingForAHuman(ctx); err != nil {
		return nil, err
	}
	actor, _ := principal.Actor(ctx)
	limit = min(max(limit, 1), PendingScanCap)
	out := make([]RepairProposal, 0, limit)
	for len(out) < limit {
		batch, err := collect(ctx, tx, `SELECT `+columns+` FROM approval WHERE status='pending' AND id>@after ORDER BY id LIMIT @limit`, []any{pgx.NamedArgs{"after": after, "limit": limit - len(out)}})
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, candidate := range batch {
			after = candidate.ID.UUID
			visible, err := decidable(ctx, tx, actor, candidate)
			if err != nil {
				return nil, err
			}
			if !visible || !candidate.PendingAt(asOf) {
				continue
			}
			item, err := repairProposal(ctx, tx, candidate)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
	}
	return out, nil
}

// RepairProposalTx rechecks the inbox boundary, optionally after taking its decision lock.
func RepairProposalTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, asOf time.Time, lock bool) (RepairProposal, error) {
	if err := actingForAHuman(ctx); err != nil {
		return RepairProposal{}, err
	}
	if lock {
		if _, err := tx.Exec(ctx, `SELECT id FROM approval WHERE id=@id `+lockOrder+` FOR UPDATE`, pgx.NamedArgs{"id": id}); err != nil {
			return RepairProposal{}, err
		}
	}
	candidate, err := get(ctx, tx, id)
	if err != nil {
		return RepairProposal{}, err
	}
	actor, _ := principal.Actor(ctx)
	visible, err := decidable(ctx, tx, actor, candidate)
	if err != nil {
		return RepairProposal{}, err
	}
	if !visible || !candidate.PendingAt(asOf) {
		return RepairProposal{}, apperrors.ErrNotFound
	}
	return repairProposal(ctx, tx, candidate)
}

func repairProposal(ctx context.Context, tx pgx.Tx, candidate row) (RepairProposal, error) {
	encoded, err := json.Marshal(candidate)
	if err != nil {
		return RepairProposal{}, err
	}
	sum := sha256.Sum256(encoded)
	item := RepairProposal{ID: candidate.ID, Kind: candidate.Kind, TargetType: candidate.TargetType, TargetID: candidate.TargetID, Change: candidate.ProposedChange, Snapshot: hex.EncodeToString(sum[:])}
	item.Pristine, err = storekit.MachineCreatedUntouched(ctx, tx, "approval", candidate.ID.UUID)
	return item, err
}

// DuplicateRepairProposalTx keeps the oldest identical offer, taking staging locks only for decisions.
func DuplicateRepairProposalTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, asOf time.Time, forDecision bool) (bool, error) {
	if _, err := RepairProposalTx(ctx, tx, id, asOf, false); err != nil {
		return false, err
	}
	candidate, err := get(ctx, tx, id)
	if err != nil || candidate.TargetID == nil {
		return false, err
	}
	offers, err := readOffersAtTx(ctx, tx, candidate.Kind, *candidate.TargetID, asOf, forDecision)
	if err != nil {
		return false, err
	}
	for _, offer := range offers {
		if offer.Pending() && bytes.Equal(offer.Change, candidate.ProposedChange) {
			return offer.ID != candidate.ID, nil
		}
	}
	return false, nil
}

// WithdrawReviewedTx preserves an edited or decided proposal after the review was made.
func WithdrawReviewedTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, snapshot, reason string, asOf time.Time) (bool, error) {
	proposal, err := RepairProposalTx(ctx, tx, id, asOf, true)
	if err != nil {
		return false, err
	}
	if !proposal.Pristine || proposal.Snapshot != snapshot {
		return false, apperrors.ErrConflict
	}
	return withdrawInTx(ctx, tx, id, reason)
}
