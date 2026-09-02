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
	"errors"
	"log/slog"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// waitingCustomers reads who is waiting, or names why it could not.
//
// A refusal is reported as a withheld source; any other failure as a failed
// one. Neither takes the rest of the day down with it — a page that answered
// nothing because one source stumbled is less useful than a page that says
// which part it could not read.
func (s *Service) waitingCustomers(
	ctx context.Context, asOf time.Time,
) (waitingRead, *crmcontracts.WorklistSourceUnavailable) {
	if s.waiting == nil {
		return waitingRead{}, nil
	}
	rows, cut, err := s.waiting.Unanswered(ctx, asOf)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return waitingRead{}, &crmcontracts.WorklistSourceUnavailable{
			Source: sourceWaiting, Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
		}
	case err != nil:
		// Named on the page AND recorded here. A source reported as failed with
		// nothing in the log leaves an operator with a warning they cannot act
		// on — which is how this one went a whole verification round without
		// anybody being able to say what broke.
		slog.ErrorContext(ctx, "the who-is-waiting read failed", "error", err)
		return waitingRead{}, &crmcontracts.WorklistSourceUnavailable{
			Source: sourceWaiting, Reason: crmcontracts.WorklistSourceUnavailableReasonFailed,
		}
	default:
		return waitingRead{rows: rows, read: true, cut: cut}, nil
	}
}

// waitingRead is what the who-is-waiting source answered, and whether it ran.
//
// `read` tells an empty answer from an absent lane, the way the lead read's own
// wrapper does: reach publishes a row for every source it is told about, so
// recording a source that never ran would report it as successfully read and
// empty. `cut` is the lane's own scan depth, which the row count cannot
// recover — the seam drops machine senders and folds duplicate threads AFTER
// the SQL cap, so a short answer is what a truncated scan looks like.
type waitingRead struct {
	rows []WaitingCustomer
	read bool
	cut  bool
}
