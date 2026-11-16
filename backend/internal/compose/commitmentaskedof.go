// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dueday"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// commitmentAskedOf is the one colleague a promise is put to: the colleague who
// made it, when they are active or invited. Nobody else is asked: not the
// reader, a personal mailbox's owner or the deal's owner. Taking on somebody
// else's promise is an explicit act, never a fallback. On a
// private conversation the promiser must also be able to open the message they
// made the promise in (auth.SeatMayHoldPrivateWorkTx). Our promise in mail a
// shared mailbox sent goes to the customer's owner (storekit.SharedMailWorkOwnerSQL),
// who owns the relationship and so is no stand-in. The zero id means nobody is
// asked; the finding stays on its source.
//
// It asks the task writer's own rule, so the dispatcher never stages or writes
// for a seat the writer then refuses. It also says whether they signed in yet.
func commitmentAskedOf(ctx context.Context, tx pgx.Tx, c Commitment) (ids.UUID, activities.SeatWorkState, error) {
	seat, err := ids.Nil, error(nil)
	if c.Seat != nil {
		seat = *c.Seat
	} else if c.Theirs == nil && !c.absentColleague {
		seat, err = storekit.SharedMailWorkOwnerTx(ctx, tx, c.SourceActivityID, true)
	}
	if err != nil || seat.IsZero() {
		return ids.Nil, activities.SeatCannotHoldWork, err
	}
	may, err := auth.SeatMayHoldPrivateWorkTx(ctx, tx, c.SourceActivityID, c.PrivateTo, seat)
	if err != nil || !may {
		return ids.Nil, activities.SeatCannotHoldWork, err
	}
	state, err := activities.NewTaskSeatState(ctx, tx, ids.From[ids.UserKind](seat))
	if err != nil || state == activities.SeatCannotHoldWork {
		return ids.Nil, activities.SeatCannotHoldWork, err
	}
	return seat, state, nil
}

// followUpCode titles the request to chase a customer's promise.
const followUpCode = "commitment.follow_up"

// followUpOf is the request to chase a customer's new promise, due the day
// after its day. Its seat is nil when nobody is responsible for it.
func followUpOf(ctx context.Context, tx pgx.Tx, c Commitment, claim ids.UUID) (CommitmentTaskProposal, activities.SeatWorkState, error) {
	owner, state, err := conversationOwnerTx(ctx, tx, c)
	if err != nil || owner.IsZero() {
		return CommitmentTaskProposal{}, state, err
	}
	due, err := dueday.DayAfter(c.DueDate)
	promise := strings.TrimRight(strings.TrimSpace(c.Summary), ".!?")
	text := systemtext.Message{}
	if c.SystemText != nil {
		text = *c.SystemText
	}
	text.Subject = systemtext.Sentence(followUpCode, systemtext.Text(promise))
	return CommitmentTaskProposal{
		SourceActivityID: c.SourceActivityID, Summary: fmt.Sprintf(systemtext.Copy(followUpCode, textlang.English), promise),
		Party: c.Party, SystemText: &text, SeatID: &owner, DueDate: due, DueZone: c.DueZone, Links: c.Links, Quote: c.Quote,
		Locator: c.Locator, Body: c.Body, PrivateTo: c.PrivateTo, Tentative: c.Tentative, FollowUpOf: &claim,
	}, state, err
}

// conversationOwnerTx is who chases a customer's promise: the owner of the
// mail it is in, else the member who logged it. Nobody else is asked.
func conversationOwnerTx(ctx context.Context, tx pgx.Tx, c Commitment) (ids.UUID, activities.SeatWorkState, error) {
	owner, err := storekit.SharedMailWorkOwnerTx(ctx, tx, c.SourceActivityID, false)
	if c.PrivateTo != nil {
		owner, err = *c.PrivateTo, nil
	} else if err == nil && owner.IsZero() {
		owner, err = activities.LoggedByTx(ctx, tx, c.SourceActivityID)
	}
	if err != nil || owner.IsZero() {
		return ids.Nil, activities.SeatCannotHoldWork, err
	}
	return commitmentAskedOf(ctx, tx, Commitment{Seat: &owner, SourceActivityID: c.SourceActivityID, PrivateTo: c.PrivateTo})
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
