// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// WHOSE customer is waiting. The lane resolves an owner from the record the
// thread is filed under, and these hold the three answers that owner produces:
// the reader's own, a colleague's, and nobody's.

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

	mine := WaitingCustomer{OwnerID: theReader}
	if !waitingIsMine(mine, reader()) {
		t.Fatal("a customer waiting on the reader's own record was called somebody else's")
	}
}

// A wait on a colleague's record is the colleague's. Without this the queue is
// back to showing every reader every unanswered message in the installation,
// which is the report the whole scope rework comes from.
func TestAWaitOnAColleaguesRecordIsNotTheReaders(t *testing.T) {
	t.Parallel()

	theirs := WaitingCustomer{OwnerID: theColleague}
	if waitingIsMine(theirs, reader()) {
		t.Fatal("a customer waiting on a colleague's record arrived in the reader's own queue")
	}
}

// A wait no record attributes to anybody stays in EVERY reader's mine.
//
// It is the one case where two readers seeing one row is the right answer: an
// unowned customer writing in has nobody looking at them, and a rule that
// dropped the row from every personal queue would leave them waiting forever
// while the page reported a clear day.
func TestAWaitNobodyOwnsStaysInEveryReadersQueue(t *testing.T) {
	t.Parallel()

	unowned := WaitingCustomer{}
	if !waitingIsMine(unowned, reader()) {
		t.Fatal("an unowned customer was dropped from the reader's queue, so nobody sees them")
	}
	other := principal.Principal{Type: principal.PrincipalHuman, UserID: theColleague}
	if !waitingIsMine(unowned, other) {
		t.Fatal("an unowned customer reached one reader but not another")
	}
}
