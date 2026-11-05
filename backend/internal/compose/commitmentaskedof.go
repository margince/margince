// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// commitmentAskedOf is the one colleague a promise is put to: the colleague who
// made it, when they are active or invited. Nobody else is asked. Not the
// reader of the conversation, the mailbox owner or the deal's owner: taking on
// somebody else's promise is an explicit act, never a fallback. On a private
// conversation the promiser must also be able to open the message they made the
// promise in (auth.SeatMayHoldPrivateWorkTx). The zero id means nobody is asked;
// the finding stays on its source.
//
// It asks the task writer's own rule, so the dispatcher never stages or writes
// for a seat the writer then refuses. It also says whether they signed in yet.
func commitmentAskedOf(ctx context.Context, tx pgx.Tx, c Commitment) (ids.UUID, activities.SeatWorkState, error) {
	if c.Seat == nil || c.Seat.IsZero() {
		return ids.Nil, activities.SeatCannotHoldWork, nil
	}
	may, err := auth.SeatMayHoldPrivateWorkTx(ctx, tx, c.SourceActivityID, c.PrivateTo, *c.Seat)
	if err != nil || !may {
		return ids.Nil, activities.SeatCannotHoldWork, err
	}
	state, err := activities.NewTaskSeatState(ctx, tx, ids.From[ids.UserKind](*c.Seat))
	if err != nil || state == activities.SeatCannotHoldWork {
		return ids.Nil, activities.SeatCannotHoldWork, err
	}
	return *c.Seat, state, nil
}

// pendingProposalAskedOf is who a waiting proposal stays with when its
// recipients change. One about an existing task goes to that task's current
// assignee, read afresh, so a reassigned task reaches its new holder. Any
// other stays with the colleague who made the promise
// (approvalsubject.CommitmentAskedOf), while they may still be asked.
func (d *CommitmentDispatcher) pendingProposalAskedOf(ctx context.Context, tx pgx.Tx, c Commitment, proposal *CommitmentTaskProposal) (ids.UUID, activities.SeatWorkState, error) {
	if proposal.ExistingTaskID != nil {
		target, _, err := d.recipientTaskTarget(ctx, tx, c)
		if err != nil || target.ID != *proposal.ExistingTaskID {
			return ids.Nil, activities.SeatCannotHoldWork, err
		}
		proposal.SeatID = target.Assignee
	}
	raw, err := json.Marshal(proposal)
	if err != nil {
		return ids.Nil, activities.SeatCannotHoldWork, fmt.Errorf("compose: marshal a pending commitment proposal: %w", err)
	}
	colleague, named := approvalsubject.CommitmentAskedOf(raw)
	if !named {
		return ids.Nil, activities.SeatCannotHoldWork, nil
	}
	state, err := activities.NewTaskSeatState(ctx, tx, ids.From[ids.UserKind](colleague))
	if err != nil || state == activities.SeatCannotHoldWork {
		return ids.Nil, activities.SeatCannotHoldWork, err
	}
	return colleague, state, nil
}
