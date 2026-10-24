// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// Exchange is a recent activity as a draft surface reads it.
type Exchange interface {
	ActivityKind() string
	ActivitySubject() string
}

// ThreadMail is the newest email in recent, newest first, when it has a
// subject. A task or a logged call is not a message in the thread, and its
// title is internal. One recorded after their mail still leaves that mail as
// the thread a draft answers.
func ThreadMail[A Exchange](recent []A) (A, bool) {
	for _, act := range recent {
		if act.ActivityKind() == string(crmcontracts.ActivityKindEmail) {
			return act, act.ActivitySubject() != ""
		}
	}
	var none A
	return none, false
}

// ContinueThread is the subject a draft is served under. A draft that
// continues an email thread keeps that thread's subject behind one reply
// prefix, whether they wrote last or only our side has. The model's own
// subject stays when there is no thread, because there is nothing to keep.
//
// Set in code, after the checks, because a model asked for a follow-up
// subject invents one ("Quick follow-up") and breaks the recipient's thread.
func ContinueThread(subject string, in Input) string {
	thread, ok := in.ThreadSubject()
	if !ok {
		return subject
	}
	// Fresh stands for "a thread exists": a band of none would drop the reply
	// prefix for an email whose direction is unknown, and the prefix is the
	// same in every other band.
	return draftfloor.Subject(in.WrittenInto().Lang(), convstate.BandFresh, thread, true)
}
