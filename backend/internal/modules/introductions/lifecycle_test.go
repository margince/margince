// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Permission to mention somebody is not a handshake. A product that let a
// name-drop become `introduced` would tell a rep a door had been opened that
// nobody opened — and the rep would stop chasing it.
func TestANameDropNeverBecomesAnIntroduction(t *testing.T) {
	for _, actor := range []Actor{ActorRequester, ActorIntroducer, ActorClock, ActorCapture} {
		if err := May(StatusNameDropApproved, StatusIntroduced, actor); err == nil {
			t.Errorf("%s was allowed to turn a lent name into an introduction", actor)
		}
	}
}

// The colleague's answer is the colleague's to give. A rep who could accept on
// their behalf would be asserting a relationship in somebody else's name.
func TestOnlyTheColleagueAnswersTheAsk(t *testing.T) {
	answers := []Status{
		StatusAccepted, StatusNameDropApproved, StatusSuggestOther, StatusDeclined,
	}
	for _, answer := range answers {
		if err := May(StatusRequested, answer, ActorIntroducer); err != nil {
			t.Errorf("the colleague could not answer %q: %v", answer, err)
		}
		err := May(StatusRequested, answer, ActorRequester)
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("the requester answering %q gave %v; want a permission refusal", answer, err)
		}
	}
}

// A reply is the product's best outcome and the one claim a person must never
// be able to type. It comes from captured activity or not at all.
func TestOnlyCapturedActivityRecordsAReply(t *testing.T) {
	for _, from := range []Status{StatusIntroduced, StatusNameDropped} {
		if err := May(from, StatusReplied, ActorCapture); err != nil {
			t.Errorf("capture could not record a reply from %q: %v", from, err)
		}
		for _, human := range []Actor{ActorRequester, ActorIntroducer} {
			if err := May(from, StatusReplied, human); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("%s could assert a reply from %q (%v)", human, from, err)
			}
		}
	}
}

// Using a lent name is something the REP does. The colleague who lent it has no
// way to know whether it was used.
func TestOnlyTheRequesterReportsUsingALentName(t *testing.T) {
	if err := May(StatusNameDropApproved, StatusNameDropped, ActorRequester); err != nil {
		t.Errorf("the requester could not report using the name: %v", err)
	}
	err := May(StatusNameDropApproved, StatusNameDropped, ActorIntroducer)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the colleague reported a name-drop they cannot observe (%v)", err)
	}
}

// An accepted ask that nobody completes is exactly the request a queue loses
// quietly. Expiry has to reach every state where somebody still owes an action.
func TestExpiryReachesEveryStateThatStillOwesAnAction(t *testing.T) {
	for _, from := range []Status{StatusRequested, StatusAccepted, StatusNameDropApproved} {
		if !Open(from) {
			t.Errorf("%q owes an action and does not read as open", from)
		}
		if err := May(from, StatusExpired, ActorClock); err != nil {
			t.Errorf("the sweep could not expire %q: %v", from, err)
		}
	}
	// And nothing settled expires: re-closing a closed ask would rewrite its
	// outcome, and the audit would carry two endings.
	for _, from := range []Status{StatusDeclined, StatusIntroduced, StatusReplied, StatusCancelled} {
		if Open(from) {
			t.Errorf("%q is settled and reads as still open", from)
		}
		if err := May(from, StatusExpired, ActorClock); err == nil {
			t.Errorf("the sweep expired %q, which was already settled", from)
		}
	}
}

// The clock is not a person. A sweep that could decline on a colleague's behalf
// would put a refusal in their mouth that they never gave.
func TestTheSweepCanOnlyExpire(t *testing.T) {
	answers := []Status{
		StatusAccepted, StatusNameDropApproved, StatusSuggestOther,
		StatusDeclined, StatusIntroduced, StatusReplied,
	}
	for _, to := range answers {
		if err := May(StatusRequested, to, ActorClock); err == nil {
			t.Errorf("the sweep moved an ask to %q", to)
		}
	}
}

// Withdrawing is the requester's while the ask is still live, and nobody's once
// it is settled.
func TestOnlyTheRequesterWithdrawsAndOnlyWhileItIsLive(t *testing.T) {
	for _, from := range []Status{StatusRequested, StatusAccepted, StatusNameDropApproved} {
		if err := May(from, StatusCancelled, ActorRequester); err != nil {
			t.Errorf("the requester could not withdraw from %q: %v", from, err)
		}
		if err := May(from, StatusCancelled, ActorIntroducer); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("the colleague cancelled the rep's ask from %q (%v)", from, err)
		}
	}
	if err := May(StatusDeclined, StatusCancelled, ActorRequester); err == nil {
		t.Error("a declined ask was withdrawn, rewriting the colleague's answer")
	}
}

// A suggestion is a pointer, not an ask. Turning it into one here would create
// a request against a colleague who never agreed to carry it.
func TestASuggestionIsTerminal(t *testing.T) {
	onward := []Status{StatusAccepted, StatusIntroduced, StatusNameDropped, StatusReplied}
	for _, to := range onward {
		for _, actor := range []Actor{ActorRequester, ActorIntroducer, ActorCapture} {
			if err := May(StatusSuggestOther, to, actor); err == nil {
				t.Errorf("%s moved a suggestion to %q instead of opening a new ask", actor, to)
			}
		}
	}
}

// A move nobody may make and a move the wrong person attempts are different
// facts, and the caller renders them as 409 and 403. Collapsing them would tell
// a rep the record was in the wrong state when the truth is it is not theirs.
func TestAnImpossibleMoveIsNotAForbiddenOne(t *testing.T) {
	if err := May(StatusRequested, StatusAccepted, ActorRequester); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the wrong actor on a legal move gave %v; want permission denied", err)
	}
	if err := May(StatusDeclined, StatusAccepted, ActorIntroducer); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("an illegal move gave %v; want a conflict", err)
	}
}

// The wire says which answer; an unknown one is refused rather than defaulted.
// Defaulting would let a typo become a decision.
func TestOnlyTheFourAnswersAreDecisions(t *testing.T) {
	for _, ok := range []string{"accepted", "name_drop_approved", "suggest_other", "declined"} {
		if _, valid := Decision(ok); !valid {
			t.Errorf("%q is one of the four answers and was refused", ok)
		}
	}
	for _, bad := range []string{"introduced", "replied", "expired", "cancelled", "", "ACCEPTED"} {
		if _, valid := Decision(bad); valid {
			t.Errorf("%q was accepted as a decision", bad)
		}
	}
}

// The set Open() describes is spelled twice: here, and as the WHERE clause of
// the partial unique index that is the duplicate guard. A status added to one
// and forgotten in the other is how an ask becomes undetectably duplicable, or
// unexpirable — so the migration's own text is the assertion.
func TestOpenMatchesTheDuplicateGuardIndex(t *testing.T) {
	sql, err := os.ReadFile(
		"../../../migrations/core/1788220000_an_introduction_is_asked_and_answered.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	guard := string(sql)
	start := strings.Index(guard, "CREATE UNIQUE INDEX intro_request_open_route")
	if start < 0 {
		t.Fatal("the duplicate-guard index is gone from the migration")
	}
	clause := guard[start:]
	if end := strings.Index(clause, ";"); end > 0 {
		clause = clause[:end]
	}

	every := []Status{
		StatusRequested, StatusAccepted, StatusNameDropApproved, StatusSuggestOther,
		StatusDeclined, StatusIntroduced, StatusNameDropped, StatusReplied,
		StatusExpired, StatusCancelled,
	}
	for _, s := range every {
		named := strings.Contains(clause, "'"+string(s)+"'")
		if named != Open(s) {
			t.Errorf("%q: Open()=%v but the guard index names it=%v — the two have drifted",
				s, Open(s), named)
		}
	}
}

// The reply predicate in SQL and the transition table say the same thing.
//
// RecordReply spells its admissible statuses as a literal `status IN (...)`
// because a SQL statement cannot ask the Go transition table. That makes two
// spellings of one rule, and the way they fail is silent: adding a state a
// capture may reply from — a follow-up sent, say — updates the table, leaves
// the statement behind, and the new state simply never advances. Nothing
// errors; the ask just sits there looking unanswered forever.
//
// So this reads the statuses back OUT of the store's own SQL and checks the
// table agrees, in both directions.
func TestTheReplyPredicateMatchesTheLifecycle(t *testing.T) {
	source, err := os.ReadFile("reply.go")
	if err != nil {
		t.Fatalf("reading the reply path: %v", err)
	}
	// BOTH status lists in the statement: the CTE's, which chooses and locks
	// the row, and the UPDATE's, which re-checks it after the lock is released
	// by a winner. They must agree with each other AND with the table — a
	// mismatch between the two is how the loser of a race writes a second
	// transition over a reply that already happened.
	statement := regexp.MustCompile(
		`(?s)WITH prior AS \(.*?RETURNING prior\.status`).Find(source)
	if statement == nil {
		t.Fatal("RecordReply's statement is no longer where this test looks — " +
			"re-point it, do not delete it")
	}
	lists := regexp.MustCompile(`status IN \(([^)]*)\)`).FindAllSubmatch(statement, -1)
	if len(lists) != 2 {
		t.Fatalf("the statement carries %d status list(s); want two — the CTE's, which "+
			"locks the row, and the UPDATE's, which re-checks it after a winner "+
			"commits. One alone lets a concurrent reply write a second transition",
			len(lists))
	}
	inSQL := map[Status]bool{}
	for i, list := range lists {
		names := map[Status]bool{}
		for _, quoted := range regexp.MustCompile(`'([a-z_]+)'`).FindAllSubmatch(list[1], -1) {
			names[Status(quoted[1])] = true
		}
		if len(names) == 0 {
			t.Fatalf("status list %d names nothing, so it admits nothing", i+1)
		}
		if i == 0 {
			inSQL = names
			continue
		}
		if !reflect.DeepEqual(names, inSQL) {
			t.Errorf("the two status lists disagree (%v vs %v) — the looser one decides, "+
				"and a status only the UPDATE admits is one a concurrent reply can "+
				"transition twice", inSQL, names)
		}
	}

	inTable := map[Status]bool{}
	for _, tr := range transitions {
		if tr.to == StatusReplied && tr.by == ActorCapture {
			inTable[tr.from] = true
		}
	}

	for from := range inTable {
		if !inSQL[from] {
			t.Errorf("the lifecycle lets a capture reply from %q, but the SQL never matches it — "+
				"an ask in that state would wait forever", from)
		}
	}
	for from := range inSQL {
		if !inTable[from] {
			t.Errorf("the SQL admits a reply from %q, which the lifecycle forbids", from)
		}
	}
}
