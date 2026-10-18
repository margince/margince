// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// WHOSE customer is waiting. The lane resolves an owner from the record the
// thread is filed under, and these hold the three answers that owner produces:
// the reader's own, a colleague's, and nobody's.
//
// Asserted through ownedByReader over a CLASSIFIED row, which is the function
// the page actually calls. A helper taking the WaitingCustomer would prove the
// walk and not the filter — and that gap is exactly how a named owner's queue
// came to drop every wait while a test of the walk stayed green.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var (
	theReader    = ids.MustParse("01a05500-0000-7000-8000-000000000001")
	theColleague = ids.MustParse("01a05500-0000-7000-8000-0000000000c0")
)

func reader() principal.Principal {
	return principal.Principal{Type: principal.PrincipalHuman, UserID: theReader}
}

// A wait on a record the reader owns is theirs, whatever state that record is
// in. This is the defect the resolved owner replaced: ownership used to be
// judged against the deals in the day's AT-RISK lane, so a reader whose deal was
// perfectly healthy never found it there and their own waiting customer read as
// somebody else's.
func TestAWaitOnTheReadersOwnRecordIsTheirs(t *testing.T) {
	t.Parallel()

	mine := classifyWaiting(WaitingCustomer{OwnerID: theReader}, readInstant)
	if !ownedByReader(mine, reader()) {
		t.Fatal("a customer waiting on the reader's own record was called somebody else's")
	}
}

// A wait on a colleague's record is the colleague's. Without this the queue is
// back to showing every reader every unanswered message in the installation,
// which is the report the whole scope rework comes from.
func TestAWaitOnAColleaguesRecordIsNotTheReaders(t *testing.T) {
	t.Parallel()

	theirs := classifyWaiting(WaitingCustomer{OwnerID: theColleague}, readInstant)
	if ownedByReader(theirs, reader()) {
		t.Fatal("a customer waiting on a colleague's record arrived in the reader's own queue")
	}
}

// Unowned work is reachable through its explicit queue, never every reader's personal queue.
func TestAWaitNobodyOwnsIsNotPersonalWork(t *testing.T) {
	t.Parallel()
	unowned := classifyWaiting(WaitingCustomer{}, readInstant)
	for _, person := range []principal.Principal{reader(), {Type: principal.PrincipalHuman, UserID: theColleague}} {
		if ownedByReader(unowned, person) {
			t.Fatal("unowned work was claimed as personal responsibility")
		}
	}
}

func TestAReadableUnassignedWaitIsDistinctFromAWithheldOwner(t *testing.T) {
	visible := classifyWaiting(WaitingCustomer{OwnerKnown: true}, readInstant)
	withheld := classifyWaiting(WaitingCustomer{}, readInstant)
	if visible.ownerRef.kind != ownerNobody || withheld.ownerRef.kind != ownerUnreadable {
		t.Fatal("unassigned responsibility was confused with inaccessible ownership")
	}
	if ownedByReader(visible, reader()) {
		t.Fatal("unassigned work became personal")
	}
}
