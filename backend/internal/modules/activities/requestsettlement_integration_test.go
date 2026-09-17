// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The settlement trigger set, its effects and its refusals, against a real
// database.
//
// Nothing in the unit lane reaches any of it. The candidate read is the waiting
// query with two more clauses over a correlated subquery; the effect runs
// through updateActivityInTx inside the same transaction as the verdict write;
// and the third door in requestUnsettledSQL is a predicate the database
// evaluates. Each can be wrong in a way that compiles and simply settles the
// wrong requests — or, worse, closes work somebody had picked back up.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// replyTo writes an outbound message on the request's own thread.
//
// Through the real writer and on the SAME thread key, because that pairing is
// the whole trigger: a reply on another thread is somebody else's conversation,
// and the candidate read must not offer it.
//
// It carries the request's own correspondent, ATTESTED, which is what a message
// the provider really sent to that address looks like. The thread key alone no
// longer settles anything — a sender types their own References root — so a
// fixture that set only the key would be testing a door the reads have closed.
func replyTo(t *testing.T, e *loadEnv, request ids.UUID, body string, at time.Time) {
	t.Helper()
	replyToCounterparty(t, e, request, requestCounterparty, body, at)
}

// replyToCounterparty is replyTo with the correspondent named, for the tests
// that ask what happens when our outbound went to somebody else.
func replyToCounterparty(t *testing.T, e *loadEnv, request ids.UUID, counterparty, body string, at time.Time) {
	t.Helper()
	store := storeKnowing(e)
	var key string
	if err := e.owner.QueryRow(e.as(), `SELECT coalesce(thread_key, '') FROM activity WHERE id = $1`,
		request).Scan(&key); err != nil {
		t.Fatal(err)
	}
	subject, direction := "Re: request", "outbound"
	if _, _, err := store.LogActivity(asClassifier(e), LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction,
		ThreadKey: key, Source: "test", OccurredAt: &at,
		CounterpartyEmail: counterparty, CounterpartyOutboundAttested: true,
	}); err != nil {
		t.Fatal(err)
	}
}

// repliedIDs is the candidate set as ids, for assertions that ask whether one
// particular request is in it. By id rather than by count: the lane's template
// is shared across this package's tests, so another test's request is present
// too and a count would pass or fail on which tests ran.
func repliedIDs(t *testing.T, e *loadEnv, asOf time.Time) map[ids.UUID]RepliedRequest {
	t.Helper()
	rows, err := storeKnowing(e).RepliedRequests(asClassifier(e), asOf, 64)
	if err != nil {
		t.Fatal(err)
	}
	out := map[ids.UUID]RepliedRequest{}
	for _, row := range rows {
		out[row.RequestID] = row
	}
	return out
}

// mintReminder runs the pass that files the email_request task, which is what
// gives a settlement something to complete.
//
// Called explicitly rather than folded into seedEmailRequest, because the four
// trigger-set tests above deliberately have no reminder: the candidate read
// must offer a request whether or not one was ever filed, and a seed that
// always minted one would hide a task-less request from every test here.
func mintReminder(t *testing.T, e *loadEnv, asOf time.Time) {
	t.Helper()
	if err := storeKnowing(e).CaptureEmailRequests(asClassifier(e), asOf); err != nil {
		t.Fatal(err)
	}
}

func taskFor(t *testing.T, e *loadEnv, request ids.UUID) (id ids.UUID, subject string, done bool, due *time.Time) {
	t.Helper()
	if err := e.owner.QueryRow(e.as(), `
		SELECT id, coalesce(subject, ''), is_done, due_at FROM activity
		 WHERE source_system = $1 AND source_activity_id = $2`,
		EmailRequestTaskSource, request).Scan(&id, &subject, &done, &due); err != nil {
		t.Fatalf("reading the request's task: %v", err)
	}
	return id, subject, done, due
}

// The defect this whole feature exists for: the reply landed BEFORE the verdict
// did, which is the ordinary case — mail arrives and is answered within the
// hour, and the hourly classifier reaches it days later.
//
// A trigger set that keyed on "a reply since we recognised the request" would
// miss exactly this, and would have looked correct in every test written the
// other way round.
func TestAReplySentBeforeTheVerdictStillOffersTheRequest(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Kurzes Gespräch", "meeting", OwedVerdictAsksUs)
	replyTo(t, e, request, "Passt. Bis zum 30ten.", requestInstant.Add(time.Hour))

	candidates := repliedIDs(t, e, requestInstant.Add(48*time.Hour))
	if _, ok := candidates[request]; !ok {
		t.Fatal("a request answered an hour after it arrived is not offered for settlement — " +
			"the reply predates the verdict, which is the ordinary order and the case this feature exists for")
	}
}

// A request nobody has answered is never judged. The pass costs one model call
// per conversation, and a thread with no reply on it has nothing to read: what
// our words did is not a question until there are words.
func TestAnUnansweredRequestIsNeverOffered(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Unanswered ask", "commitment", OwedVerdictAsksUs)

	if _, ok := repliedIDs(t, e, requestInstant.Add(48*time.Hour))[request]; ok {
		t.Fatal("a request with no reply on its thread was offered for settlement")
	}
}

// A scheduled send has not reached the customer, so it cannot have settled
// anything. The same bound the engagement rule carries, and for the same
// reason — without it a queued mail would take a request out of the queue
// before anybody had answered it.
func TestAFutureDatedReplyDoesNotOfferTheRequest(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Scheduled answer", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "This goes out tomorrow.", requestInstant.Add(72*time.Hour))

	if _, ok := repliedIDs(t, e, requestInstant.Add(2*time.Hour))[request]; ok {
		t.Fatal("a reply dated after the read instant offered the request for settlement")
	}
}

// The watermark: judged through its newest reply, a thread is not re-read, and
// a NEWER reply arms it again. Without the first half the pass pays for the same
// conversation every hour forever; without the second it never revisits a
// conversation that has moved on.
func TestAJudgedThreadReturnsOnlyWhenSomebodyWritesAgain(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	request := seedEmailRequest(t, e, "Watermark", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "I will check.", requestInstant.Add(time.Hour))

	asOf := requestInstant.Add(48 * time.Hour)
	candidate, ok := repliedIDs(t, e, asOf)[request]
	if !ok {
		t.Fatal("the answered request was not offered at all")
	}
	if err := store.SettleRequest(asClassifier(e), RequestSettlementInput{
		Request: candidate, Verdict: RequestStillOwed, Remaining: "Send the quote",
		Confidence: 0.9, DecidedBy: "test-model",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := repliedIDs(t, e, asOf)[request]; ok {
		t.Fatal("a thread judged through its newest reply was offered again — the pass would " +
			"pay for the same conversation on every tick")
	}

	replyTo(t, e, request, "Here it is.", requestInstant.Add(24*time.Hour))
	if _, ok := repliedIDs(t, e, asOf)[request]; !ok {
		t.Fatal("a newer reply did not re-arm the question — the conversation moved on and " +
			"the verdict standing over it is about words nobody has read")
	}
}

// settled completes the reminder, and the completion is what every other
// surface reads: requestUnsettledSQL already treats a done task as settled, so
// the waiting lane and the email move clear with no further change.
func TestSettledCompletesTheReminderAndClearsTheRequest(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	request := seedEmailRequest(t, e, "Answered ask", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Done — sent this morning.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))

	asOf := requestInstant.Add(48 * time.Hour)
	candidate := repliedIDs(t, e, asOf)[request]
	if candidate.TaskID == nil {
		t.Fatal("the request carries no reminder, so the settlement has nothing to complete")
	}
	if err := store.SettleRequest(asClassifier(e), RequestSettlementInput{
		Request: candidate, Verdict: RequestSettled, Confidence: 0.95, DecidedBy: "test-model",
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, done, _ := taskFor(t, e, request); !done {
		t.Error("a settled verdict left the reminder open")
	}
	summaries, err := store.EmailSummariesByID(e.as(), []ids.UUID{request})
	if err != nil {
		t.Fatal(err)
	}
	if summaries[request].Move == crmcontracts.EmailSummaryMoveNeedsReply {
		t.Error("the message still reads as needing a reply after its request was settled")
	}
}

// still_owed sharpens a MACHINE-filed, undated reminder, so the task says what
// is actually owed rather than the mail's subject line.
func TestStillOwedRetitlesTheMachinesOwnUndatedReminder(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	request := seedEmailRequest(t, e, "Preis für die zweite Charge", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Ich kläre das und melde mich.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))

	asOf := requestInstant.Add(48 * time.Hour)
	candidate := repliedIDs(t, e, asOf)[request]
	if !candidate.TaskMachineMinted {
		t.Fatal("the reminder the owed pass filed does not read as machine-minted, so nothing " +
			"could ever be retitled")
	}
	due := requestInstant.Add(96 * time.Hour)
	if err := store.SettleRequest(asClassifier(e), RequestSettlementInput{
		Request: candidate, Verdict: RequestStillOwed, Remaining: "Send the quote",
		DueAt: &due, Confidence: 0.9, DecidedBy: "test-model",
	}); err != nil {
		t.Fatal(err)
	}

	_, subject, done, dueAt := taskFor(t, e, request)
	if subject != "Send the quote" {
		t.Errorf("the reminder is titled %q, want the thing actually owed", subject)
	}
	if done {
		t.Error("still_owed completed the reminder")
	}
	if dueAt == nil || !dueAt.Equal(due) {
		t.Errorf("due_at = %v, want the date our own words named", dueAt)
	}
}

// A reminder a HUMAN dated is theirs. The machine may not move a deadline
// somebody agreed to, and captured_by plus an absent due date is what tells the
// two apart — never the subject, which a human accepting a request keeps.
func TestStillOwedLeavesAHumansOwnDeadlineAlone(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	request := seedEmailRequest(t, e, "Dated by hand", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Looking at it.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))

	asOf := requestInstant.Add(48 * time.Hour)
	candidate := repliedIDs(t, e, asOf)[request]
	task, _, _, _ := taskFor(t, e, request)

	// A human puts a date on it, which is the act that makes it theirs.
	theirs := requestInstant.Add(240 * time.Hour)
	actor, _ := principal.Actor(e.as())
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Update: true}
	if _, err := store.UpdateActivity(principal.WithActor(e.as(), actor),
		ids.From[ids.ActivityKind](task), UpdateActivityInput{DueAt: &theirs}); err != nil {
		t.Fatal(err)
	}

	// Re-read: the human's edit moved the version, and the candidate the pass
	// judges has to be the one it read.
	candidate = repliedIDs(t, e, asOf)[request]
	if candidate.TaskMachineMinted {
		t.Fatal("a reminder somebody dated still reads as the machine's to retitle")
	}
	if err := store.SettleRequest(asClassifier(e), RequestSettlementInput{
		Request: candidate, Verdict: RequestStillOwed, Remaining: "Send the quote",
		Confidence: 0.9, DecidedBy: "test-model",
	}); err != nil {
		t.Fatal(err)
	}

	_, subject, _, dueAt := taskFor(t, e, request)
	if subject != "Dated by hand" {
		t.Errorf("the reminder was retitled to %q over a human's own dated task", subject)
	}
	if dueAt == nil || !dueAt.Equal(theirs) {
		t.Errorf("due_at = %v, want the date somebody set", dueAt)
	}
}

// unsure records that the pass asked and would not commit. It touches no task —
// the request stays owed exactly as it was — and it advances the watermark, so
// the same conversation is not re-read until somebody writes again.
func TestUnsureRecordsTheQuestionAndChangesNothingElse(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	request := seedEmailRequest(t, e, "Ambiguous", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Hm.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))

	asOf := requestInstant.Add(48 * time.Hour)
	candidate := repliedIDs(t, e, asOf)[request]
	if err := store.SettleRequest(asClassifier(e), RequestSettlementInput{
		Request: candidate, Verdict: RequestUnsure, Confidence: 0.4, DecidedBy: "test-model",
	}); err != nil {
		t.Fatal(err)
	}

	_, subject, done, _ := taskFor(t, e, request)
	if done {
		t.Error("an unsure verdict completed the reminder")
	}
	if subject != "Ambiguous" {
		t.Errorf("an unsure verdict retitled the reminder to %q", subject)
	}
	if _, ok := repliedIDs(t, e, asOf)[request]; ok {
		t.Error("an unsure verdict left the conversation in the backlog, so the pass would " +
			"re-read the same words on every tick")
	}
}

// Somebody picking the work back up outranks the machine's verdict. Without
// this the queue and the task list disagree: a reopened reminder sits on a
// request every other reader reports as settled.
func TestAReopenedReminderOutranksASettledVerdict(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	request := seedEmailRequest(t, e, "Reopened", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Sent.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))

	asOf := requestInstant.Add(48 * time.Hour)
	candidate := repliedIDs(t, e, asOf)[request]
	if err := store.SettleRequest(asClassifier(e), RequestSettlementInput{
		Request: candidate, Verdict: RequestSettled, Confidence: 0.95, DecidedBy: "test-model",
	}); err != nil {
		t.Fatal(err)
	}
	task, _, _, _ := taskFor(t, e, request)

	// A human reopens it.
	reopen := false
	actor, _ := principal.Actor(e.as())
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Update: true}
	if _, err := store.UpdateActivity(principal.WithActor(e.as(), actor),
		ids.From[ids.ActivityKind](task), UpdateActivityInput{IsDone: &reopen}); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.EmailSummariesByID(e.as(), []ids.UUID{request})
	if err != nil {
		t.Fatal(err)
	}
	if summaries[request].Move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Error("a request whose reminder somebody reopened does not read as outstanding — " +
			"the machine's settled verdict is overruling whoever took the work back")
	}
}

// A stranger cannot make our reply to somebody else answer their message.
//
// thread_key can be the RFC822 References root, which the SENDER types. So a
// cold mail can claim a thread it was never part of, and before the reads bound
// themselves to a correspondent, our outbound to the real customer satisfied
// "the workspace has answered" for the forged request. That offered the
// stranger's message for settlement, and the conversation the settlement pass
// then read was the REAL customer's — which is how their private mail reached a
// cloud model under an attacker's thread id.
//
// The reply here is attested to the real customer, which is the only thing that
// distinguishes it: the thread key, kind and provider all match the forgery.
func TestAForgedThreadIsNotSettledByOurReplyToSomebodyElse(t *testing.T) {
	e := setupLoad(t)
	forged := seedEmailRequestWithCounterparty(t, e, "Forged thread", "commitment",
		OwedVerdictAsksUs, "stranger@elsewhere.test")
	replyToCounterparty(t, e, forged, "Here are the terms you asked for.",
		"Our answer to the real customer.", requestInstant.Add(time.Hour))

	if _, ok := repliedIDs(t, e, requestInstant.Add(48*time.Hour))[forged]; ok {
		t.Fatal("a message that forged its way onto another customer's thread was offered " +
			"for settlement by our reply to that customer — the conversation read would " +
			"then hand the real customer's mail to the model as the stranger's context")
	}
}

// The counterparty is carried out of the selection so the conversation read can
// bind to it without reading the column a second time. A candidate that lost it
// would make that read unbindable, and an unbindable read is the whole defect.
func TestACandidateNamesTheCorrespondentItWasWith(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Named correspondent", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "On its way.", requestInstant.Add(time.Hour))

	candidate, ok := repliedIDs(t, e, requestInstant.Add(48*time.Hour))[request]
	if !ok {
		t.Fatal("the answered request was not offered for settlement")
	}
	if candidate.CounterpartyEmail != requestCounterparty {
		t.Fatalf("the candidate names %q as its correspondent, not %q",
			candidate.CounterpartyEmail, requestCounterparty)
	}
}
