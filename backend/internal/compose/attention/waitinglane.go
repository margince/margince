// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The customers nobody has answered.
//
// Its own file because it is the one lane read BESIDE the assembled day rather
// than as one of its fourteen: /attention publishes those fourteen and this is
// not among them, so it carries its own bound, its own truncation answer and its
// own ownership walk.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Waiting is who has written to this workspace and had no reply.
//
// Its own reader rather than a filter over AtRisk, because a fresh inbound
// makes a deal LESS quiet: deriving "waiting" from "quiet" loses the newest
// cases, which are the ones a rep most needs. It also reaches a person with no
// deal at all, whom the deal-shaped lanes never see.
type Waiting interface {
	// The bool reports that the read was CUT — the lane scanned to its own bound
	// and messages may sit past it.
	//
	// It travels beside the rows for the reason the at-risk lane's does: this
	// lane FILTERS after it scans. The seam drops machine senders and folds
	// duplicate threads out of what SQL returned, so a hundred and eighty rows
	// can be what is left of a full two hundred — and a hundred and eighty is
	// also what a smaller, complete installation returns. A caller counting rows
	// against the bound would read the truncated scan as complete.
	Unanswered(ctx context.Context, asOf time.Time) (rows []WaitingCustomer, cut bool, err error)
}

// WaitingCustomer is one message nobody has answered.
type WaitingCustomer struct {
	// ActivityID is the message itself — what a reply would be drafted to.
	ActivityID ids.UUID
	Subject    string
	// Since is when they wrote. The wait is measured from it, and it is what
	// the card says out loud.
	Since time.Time
	// The record the thread is filed under, most specific first. Any may be
	// zero: a message from a stranger names nobody.
	PersonID       ids.UUID
	OrganizationID ids.UUID
	DealID         ids.UUID
	// HasOpenDeal reports whether money this reader can see is still on this
	// thread. It is what keeps a long wait in execution instead of sending it
	// to review.
	HasOpenDeal bool
	// OwnerID is who owes this reply, resolved by the module from the record
	// the thread is filed under. Zero when nothing on it names an owner, which
	// is an unowned customer rather than a missing answer.
	OwnerID ids.UUID
}
