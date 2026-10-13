// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// A pending proposal about a deal follows the deal when its owner changes.
//
// The overnight follow-up and a transcript's next step are narrowed to the seat
// they were staged for (seatnarrowing.go). Left on the previous owner after a
// reassignment, such a card is one the new owner cannot see or decide, and the
// nightly sweep never replaces it because the deal already has one pending.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// approvalKeyOnBehalfOf is the audit field a seat move writes both images of.
const approvalKeyOnBehalfOf = "on_behalf_of"

// ownerMovedAway is the reason a withdrawn draft's audit row carries.
const ownerMovedAway = "the deal changed owner; a message drafted for the previous owner is not handed on"

// FollowDealOwnerInTx moves a deal's pending proposals from its previous owner
// to its new one, on the deal writer's transaction so the two commit together.
// Either owner may be nil: a deal nobody owns.
//
// Per kind:
//   - deal_follow_up: every pending card on the deal moves. Its text is the
//     product describing the deal, so it reads the same for any owner.
//   - commitment_task: a promise nobody could be named for moves when its
//     task would land on the deal — the promise is the deal's, and so is the
//     owner who acts on it. A promise a named colleague made stays theirs,
//     whoever owns the deal.
//   - held_draft: a reply drafted for the previous owner is withdrawn. It was
//     composed under their authority, to be sent from their mailbox, so it is
//     not handed on; the next sweep drafts one for the new owner.
//
// The caller has already taken write authority on the deal, which is what
// admits this write. It is audit-only, for the reason the writeshape gate
// records against it.
func FollowDealOwnerInTx(ctx context.Context, tx pgx.Tx, dealID ids.UUID, from, to *ids.UUID) error {
	moving, err := lockProposalsFollowingDeal(ctx, tx, dealID, from, to)
	if err != nil {
		return err
	}
	for _, p := range moving {
		if p.kind == kindHeldDraft {
			if _, err := withdrawInTx(ctx, tx, p.id, ownerMovedAway); err != nil {
				return err
			}
			continue
		}
		if err := retargetInTx(ctx, tx, p, to); err != nil {
			return err
		}
	}
	return nil
}

// followingProposal is one pending row a deal's owner change reaches.
type followingProposal struct {
	id         ids.ApprovalID
	kind       string
	onBehalfOf *ids.UUID
}

// lockProposalsFollowingDeal locks, in the canonical order, every pending row
// FollowDealOwnerInTx is about to move or withdraw.
func lockProposalsFollowingDeal(
	ctx context.Context, tx pgx.Tx, dealID ids.UUID, from, to *ids.UUID,
) ([]followingProposal, error) {
	onDeal := proposalLinksDeal(dealID)
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	deal, prev, next := arg(dealID), arg(from), arg(to)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT id, kind, on_behalf_of FROM approval
		 WHERE status = 'pending' AND expires_at > now()
		   AND (
		     (kind = $%[4]d AND target_entity_type = 'deal' AND target_entity_id = $%[1]d
		        AND on_behalf_of IS DISTINCT FROM $%[3]d)
		  OR (kind = $%[5]d AND NOT proposed_change ? 'seat_id'
		        AND proposed_change->'links' @> $%[6]d
		        AND on_behalf_of IS DISTINCT FROM $%[3]d)
		  OR (kind = $%[7]d AND target_entity_type = 'deal' AND target_entity_id = $%[1]d
		        AND on_behalf_of IS NOT NULL AND on_behalf_of IS NOT DISTINCT FROM $%[2]d)
		   )
		 `+lockOrder+`
		 FOR UPDATE`,
		deal, prev, next, arg(kindDealFollowUp), arg(kindCommitmentTask), arg(onDeal), arg(kindHeldDraft)),
		args...)
	if err != nil {
		return nil, fmt.Errorf("lock the proposals following the deal's owner: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (followingProposal, error) {
		var p followingProposal
		err := row.Scan(&p.id, &p.kind, &p.onBehalfOf)
		return p, err
	})
}

// proposalLinksDeal is the containment document matching a commitment proposal
// whose task lands on this deal. The keys are the Go field names of
// activities.ActivityLinkInput, which the payload's links carry untagged.
//
// Held by: TestEveryTranscriptCardOnTheDealFollowsIt
// (backend/internal/compose/proposalsfollowdeal_integration_test.go), which
// stages through the real transcript reader.
func proposalLinksDeal(dealID ids.UUID) json.RawMessage {
	// Spelled rather than marshalled: a uuid's text needs no escaping, so the
	// document has one possible shape and no encoding step that could fail.
	return json.RawMessage(`[{"EntityType":"deal","EntityID":"` + dealID.String() + `"}]`)
}

// retargetInTx names the new owner as the seat one proposal is for. The row is
// already locked; the compare on the seat it was read with keeps the write
// honest if that ever stops being true.
func retargetInTx(ctx context.Context, tx pgx.Tx, p followingProposal, to *ids.UUID) error {
	tag, err := tx.Exec(ctx, `UPDATE approval SET on_behalf_of = $2
		 WHERE id = $1 AND status = 'pending' AND on_behalf_of IS NOT DISTINCT FROM $3`,
		p.id, to, p.onBehalfOf)
	if err != nil {
		return fmt.Errorf("re-target approval to the deal's new owner: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: the proposal changed while its deal was reassigned", apperrors.ErrConflict)
	}
	if _, err := storekit.AuditWithEvidence(ctx, tx, "update", entityApproval, p.id.UUID,
		map[string]any{approvalKeyOnBehalfOf: p.onBehalfOf},
		map[string]any{approvalKeyOnBehalfOf: to},
		map[string]any{approvalKeyKind: p.kind, approvalKeyReason: "the deal changed owner"}); err != nil {
		return fmt.Errorf("audit re-targeted approval: %w", err)
	}
	return nil
}
