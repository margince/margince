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
	// Hidden answers what the queue's own hiding rules are keeping off this
	// reader's page, one rule at a time.
	//
	// On the same seam as the lane it measures rather than on one of its own:
	// the figures are differences between runs of the query behind Unanswered,
	// so a second seam would let an installation bind a queue and a guardrail
	// that disagree about who is waiting.
	Hidden(ctx context.Context, asOf time.Time) (HiddenWork, error)
}

// HiddenWork is how much waiting work each rule is holding back, and whether
// anything is.
//
// The module's own struct restated here rather than imported, like every other
// type across this seam: compose owns the wire shape and modules own their
// storage, and a projection reaching into a module's struct is the edge the
// architecture forbids.
type HiddenWork struct {
	Shown       int
	SetAside    int
	NotSales    int
	PastHorizon int
	Unlinked    int
	// Truncated says a read stopped at its own scan bound, which makes every
	// figure a floor. The module states why it is fatal to Clear rather than
	// merely noted beside it.
	Truncated bool
}

// Clear reports that nothing is being held back — the guardrail's target.
func (h HiddenWork) Clear() bool {
	return !h.Truncated &&
		h.SetAside == 0 && h.NotSales == 0 && h.PastHorizon == 0 && h.Unlinked == 0
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

// HiddenBacklog answers what the queue is not showing this reader.
//
// A projection over the seam and nothing more: the arithmetic is the module's,
// because the figures are differences between runs of ITS query and computing
// them here would need this package to hold a second copy of the eligibility
// rules.
//
// An unbound seam answers a clear backlog rather than an error. An installation
// that does not read the mail stream has no waiting queue to hide work from, so
// "nothing is held back" is the true answer rather than a degraded one.
func (s *Service) HiddenBacklog(ctx context.Context) (crmcontracts.HiddenBacklog, error) {
	asOf := s.now()
	if s.waiting == nil {
		return crmcontracts.HiddenBacklog{AsOf: asOf, Clear: true}, nil
	}
	work, err := s.waiting.Hidden(ctx, asOf)
	if err != nil {
		return crmcontracts.HiddenBacklog{}, err
	}
	return crmcontracts.HiddenBacklog{
		AsOf:        asOf,
		Shown:       work.Shown,
		SetAside:    work.SetAside,
		NotSales:    work.NotSales,
		PastHorizon: work.PastHorizon,
		Unlinked:    work.Unlinked,
		Truncated:   work.Truncated,
		// Derived from the same struct the figures came from, so the flag and
		// the numbers cannot disagree — a client reading `clear` over four
		// non-zero counts is the one lie this endpoint must not tell.
		Clear: work.Clear(),
	}, nil
}
