// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// commitmentAskedOf is the one colleague a promise is put to: the colleague who
// made it, when they are active or invited. Nobody else is asked. Not the
// reader of the conversation, the mailbox owner or the deal's owner: taking on
// somebody else's promise is an explicit act, never a fallback. A private
// conversation is put only to its own owner, and only when they made the
// promise. The zero id means nobody is asked; the finding stays on its source.
func commitmentAskedOf(ctx context.Context, tx pgx.Tx, c Commitment) (ids.UUID, error) {
	if c.Seat == nil || c.Seat.IsZero() {
		return ids.Nil, nil
	}
	if c.PrivateTo != nil && *c.PrivateTo != *c.Seat {
		return ids.Nil, nil
	}
	askable, err := approvals.CanBeAsked(ctx, tx, *c.Seat)
	if err != nil || !askable {
		return ids.Nil, err
	}
	return *c.Seat, nil
}

// pendingProposalAskedOf is who a waiting proposal stays with when its
// recipients change. One about an existing task goes to that task's current
// assignee, read afresh, so a reassigned task reaches its new holder. Any other stays with its seat, else its promiser, while they may still
// be asked.
func (d *CommitmentDispatcher) pendingProposalAskedOf(ctx context.Context, tx pgx.Tx, c Commitment, proposal *CommitmentTaskProposal) (ids.UUID, error) {
	colleague := derefSeat(proposal.SeatID)
	if proposal.ExistingTaskID != nil {
		target, _, err := d.recipientTaskTarget(ctx, tx, c)
		if err != nil || target.ID != *proposal.ExistingTaskID {
			return ids.Nil, err
		}
		proposal.SeatID = target.Assignee
		colleague = derefSeat(target.Assignee)
	}
	if colleague.IsZero() {
		colleague = derefSeat(proposal.PromiserSeatID)
	}
	if colleague.IsZero() {
		return ids.Nil, nil
	}
	askable, err := approvals.CanBeAsked(ctx, tx, colleague)
	if err != nil || !askable {
		return ids.Nil, err
	}
	return colleague, nil
}
