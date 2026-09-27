// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The vocabulary the selectors write and the collector reads back.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Each reason files its message under its own head.
func TestEachWithheldReasonFilesUnderItsOwnHead(t *testing.T) {
	var subject PurgeSubject
	held, statute, request := ids.NewV7(), ids.NewV7(), ids.NewV7()
	subject.noteWithheld(withheldByHold, held)
	subject.noteWithheld(withheldByStatute, statute)
	subject.noteWithheld(withheldByRequest, request)

	if len(subject.Held) != 1 || subject.Held[0] != held {
		t.Errorf("held = %v, want the pinned message", subject.Held)
	}
	if len(subject.UnderStatute) != 1 || subject.UnderStatute[0] != statute {
		t.Errorf("under statute = %v, want the shielded message", subject.UnderStatute)
	}
	if len(subject.UnderRequest) != 1 || subject.UnderRequest[0] != request {
		t.Errorf("under request = %v, want the message a request is about", subject.UnderRequest)
	}
}

// A reason nothing recognizes is filed nowhere rather than guessed at. The
// union already counts it, so the total still balances — and inventing a
// category would tell an owner something the query never said.
func TestAnUnknownReasonIsFiledNowhere(t *testing.T) {
	var subject PurgeSubject
	subject.noteWithheld("something-nobody-writes", ids.NewV7())

	if len(subject.Held)+len(subject.UnderStatute)+len(subject.UnderRequest) != 0 {
		t.Fatalf("an unrecognized reason was filed anyway: %+v", subject)
	}
}

// The precedence the owner is owed one answer under: the most specific act
// about THIS record first. A pinned Handelsbrief named by an open request is
// all three, and reporting a list would answer none of them.
func TestTheReasonClauseRanksTheMostSpecificActFirst(t *testing.T) {
	clause := withheldReason("SHIELD", true)
	hold := strings.Index(clause, withheldByHold)
	statute := strings.Index(clause, withheldByStatute)
	request := strings.Index(clause, withheldByRequest)
	if hold < 0 || statute < 0 || request < 0 {
		t.Fatalf("clause names only some of the reasons: %s", clause)
	}
	if hold >= statute || statute >= request {
		t.Fatalf("precedence is hold, then statute, then request; got %s", clause)
	}
}

// The workspace purge asks no question about open requests, so its clause
// carries no arm for one — a selector that reported a reason it never tested
// would put words in the query's mouth.
func TestAClauseWithoutTheRequestArmDoesNotNameIt(t *testing.T) {
	clause := withheldReason("SHIELD", false)
	if strings.Contains(clause, withheldByRequest) {
		t.Fatalf("clause names a reason it never tests: %s", clause)
	}
	if !strings.Contains(clause, withheldByHold) || !strings.Contains(clause, withheldByStatute) {
		t.Fatalf("clause dropped a reason it does test: %s", clause)
	}
}
