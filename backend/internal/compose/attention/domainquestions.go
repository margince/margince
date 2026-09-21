// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The domain the capture triage could not judge, asked of the colleague whose
// mail raised it. Apart from the health lanes beside it because those report
// that something BROKE and this reports that something was never decided: only
// the colleague whose correspondence produced it can answer, and two seats may
// judge the same domain differently and both be right.

import (
	"context"
	"time"
)

// DomainQuestions is the reader's OWN undecided domains: the triage found
// nothing that named a company and left the question open rather than inventing
// a record. The seam takes no owner argument because the read binds to the
// acting human, which is what lets a row claim its reader as owner without a
// second field saying so.
type DomainQuestions interface {
	OpenDomainQuestions(ctx context.Context) ([]DomainQuestion, error)
}

// DomainQuestion is one domain waiting on a human answer.
type DomainQuestion struct {
	// The registrable form: both the row's identity and the question asked.
	Domain string
	// Why the machine stopped, as prose rather than the stored token, so this
	// lane and the capture-rules screen say one thing.
	Reason string
	// When the question was last opened or re-opened; orders and dates the row.
	AskedAt time.Time
}
