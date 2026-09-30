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
	clause := withheldReason("SHIELD", withheldByStatute, true)
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
	clause := withheldReason("SHIELD", withheldByStatute, false)
	if strings.Contains(clause, withheldByRequest) {
		t.Fatalf("clause names a reason it never tests: %s", clause)
	}
	if !strings.Contains(clause, withheldByHold) || !strings.Contains(clause, withheldByStatute) {
		t.Fatalf("clause dropped a reason it does test: %s", clause)
	}
}

// A shield nobody could measure is not reported as a statutory window.
//
// An absent floor shields every row, which is the right answer to "may this be
// destroyed" — a purge that cannot ask what the law requires must not guess
// that the answer is nothing. It is not an answer to "why was it kept", and
// the two shared one value: every shielded row came back labelled `statute`,
// so the receipt told an owner their mail is commercial correspondence the law
// requires keeping when nothing had established that.
//
// Both halves are asserted. The shield has to stay — dropping it to fix the
// label would destroy the correspondence the label was wrong about.
func TestAnUndeterminedFloorShieldsWithoutClaimingAStatute(t *testing.T) {
	var absent StatutoryFloor
	if absent.shieldedAs() != withheldByUndeterminedFloor {
		t.Fatal("a floor with no clause reported itself determinate")
	}
	shield, _ := absent.column(0, nil)
	if shield != "true" {
		t.Fatalf("an undetermined floor shields %q, want every row — the safe answer is to keep", shield)
	}

	clause := withheldReason(shield, absent.shieldedAs(), true)
	if strings.Contains(clause, "'"+withheldByStatute+"'") {
		t.Errorf("an undetermined floor still labels rows %q: %s — the receipt names a basis "+
			"nobody established", withheldByStatute, clause)
	}
	if !strings.Contains(clause, withheldByUndeterminedFloor) {
		t.Errorf("no arm reports the floor as undetermined: %s", clause)
	}

	// And a real floor is unaffected: it says statute, because it measured one.
	measured := StatutoryFloor{Clause: func(int, int) string { return "SHIELD" }}
	if measured.shieldedAs() != withheldByStatute {
		t.Fatal("a floor with a clause reported itself undetermined")
	}
	if got := withheldReason("SHIELD", measured.shieldedAs(), true); !strings.Contains(got, "'"+withheldByStatute+"'") {
		t.Errorf("a measured floor stopped reporting a statute: %s", got)
	}
}

// The collector files an undetermined shield under its own list rather than
// into UnderStatute, so a count the receipt renders cannot silently absorb it.
func TestAnUndeterminedShieldIsFiledApartFromTheStatute(t *testing.T) {
	var subject PurgeSubject
	id := ids.NewV7()
	subject.noteWithheld(withheldByUndeterminedFloor, id)
	if len(subject.UnderStatute) != 0 {
		t.Errorf("an undetermined shield landed in UnderStatute: %+v", subject)
	}
	if len(subject.UnderUndeterminedFloor) != 1 {
		t.Errorf("an undetermined shield was filed nowhere: %+v", subject)
	}
}
