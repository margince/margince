// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The domain the capture triage could not judge, asked of the colleague whose
// mail raised it.
//
// It sits apart from the health lanes beside it for one reason: those report
// that something BROKE, and this reports that something was never decided. A
// stopped mailbox is restored by an administrator; an open domain question is
// answered by the colleague whose correspondence produced it, and nobody else
// can answer it for them — two seats on one installation may judge the same
// domain differently and both be right.

import (
	"context"
	"time"
)

// DomainQuestions is the reader's OWN undecided domains: the triage read the
// site, found nothing that named a company, and left the question open rather
// than inventing a record.
//
// Per-user by construction, the way the capture lane beside it is. The seam
// takes no owner argument because the read binds to the acting human itself —
// which is what lets the row claim the reader as its owner without a second
// field saying so.
type DomainQuestions interface {
	OpenDomainQuestions(ctx context.Context) ([]DomainQuestion, error)
}

// DomainQuestion is one domain waiting on a human answer.
type DomainQuestion struct {
	// Domain is the registrable form, which is both the row's identity and what
	// the reader is being asked about.
	Domain string
	// Reason is why the machine stopped, in the words the operator's own list
	// uses — the two withholding reasons spelled as prose rather than as the
	// stored token, so this lane and the capture-rules screen say one thing.
	Reason string
	// AskedAt is when the question was last touched: opened, or re-opened by
	// somebody who thought the crawl deserved another try. It orders the lane
	// and dates the row.
	AskedAt time.Time
}
