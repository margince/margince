// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// The queued requests the drain fixtures act on. Canonical uuids because the
// ledger refuses anything else for an entity id, which is the core's own check
// and one the fake runs rather than waves through.
const (
	firstRequestID  = "7d2f4c11-9b83-4e6a-8f05-2a1c7b6d3e94"
	secondRequestID = "c0a83f26-15d7-4b8e-9a31-6f4d2e8b7c05"
	landedActivity  = "42b91e70-3c85-4d19-b6a7-8e05f1c2d3a4"
)

// queuedRow scripts one row of queuedColumns, in that order, so a column added
// to the projection is ONE edit in the fixtures rather than one per row.
func queuedRow(id string, attempts int, body string) []any {
	return []any{id, ownerUserID, ownerRef, []byte(body), signedAt, attempts}
}

// draining is the Runtime a tick meets: nobody behind it, which is what the core
// mints for a scheduled job, and one batch of rows waiting.
func draining(rows ...[]any) *fakeRuntime {
	rt := newRuntime().unattended()
	rt.tx.queryRows = rows
	return rt
}

// landable is a document the record builder accepts, so a test about the drain's
// own decisions is not also a test about the envelope.
func landable(tb testing.TB, messageID string) string {
	tb.Helper()
	doc := landableArrival()
	doc.MessageID = messageID
	return string(arrivalJSON(tb, doc))
}

// accepted is what the core answers for a record it kept.
func accepted() extension.Result {
	return extension.Result{
		Ref:         extension.Ref{Type: "activity", ID: landedActivity},
		Disposition: extension.DispositionAccepted,
	}
}

// A queued request becomes a timeline entry without anyone clicking, landed
// under the OWNER's name — the member frozen on the row at arrival, never
// anything the anonymous sender chose.
func TestAQueuedRequestBecomesATimelineEntryUnderTheOwnersName(t *testing.T) {
	t.Parallel()
	rt := draining(queuedRow(firstRequestID, 0, landable(t, "m-1")))
	rt.results = []extension.Result{accepted()}
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("draining: %v", err)
	}
	if len(rt.ingested) != 1 {
		t.Fatalf("the tick handed the core %d record(s); one request was waiting", len(rt.ingested))
	}
	if string(rt.ingestedFor[0]) != ownerUserID {
		t.Fatalf("the record was landed for %q; it is the endpoint owner's authority that bounds it", rt.ingestedFor[0])
	}
	sql, args := rt.tx.statementMentioning(t, "activity_id = nullif")
	if !strings.Contains(sql, "state = $2") {
		t.Fatalf("the advance does not set the state:\n%s", sql)
	}
	if args[1] != stateLanded || args[2] != landedActivity {
		t.Fatalf("the row was advanced to %v naming activity %v", args[1], args[2])
	}
}

// The rule the whole file exists to keep: a record is ingested with NONE of this
// unit's transactions open, and the row moves only afterwards.
//
// Both halves are asserted, because they fail differently. Nesting hangs a small
// connection pool in production and is refused by the fake exactly as the core
// refuses it; advancing first loses the message when the ingest then fails.
func TestARecordIsIngestedOutsideEveryTransactionAndTheRowMovesAfterwards(t *testing.T) {
	t.Parallel()
	rt := draining(
		queuedRow(firstRequestID, 0, landable(t, "m-1")),
		queuedRow(secondRequestID, 0, landable(t, "m-2")),
	)
	rt.results = []extension.Result{accepted(), accepted()}
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("draining: %v", err)
	}
	// One statement had been issued when the first record was handed over: the
	// batch read. Anything more means a row was advanced before the core had
	// answered for it.
	if got := rt.ingestedAfter[0]; got != 1 {
		t.Fatalf("%d statement(s) had run when the first record was ingested; only the batch read may have", got)
	}
	if got := rt.ingestedAfter[1]; got != 2 {
		t.Fatalf("%d statement(s) had run when the second record was ingested; the first request's own advance is the only one that may have", got)
	}
}

// A skip is a SUCCESS. The core drops a wholly-internal message deliberately and
// commits a breadcrumb; treating that as a failure would retry a deliberate drop
// forever, on a cadence.
func TestASkippedRecordAdvancesTheRowJustAsAnAcceptedOneDoes(t *testing.T) {
	t.Parallel()
	rt := draining(queuedRow(firstRequestID, 0, landable(t, "m-1")))
	rt.results = []extension.Result{{Disposition: extension.DispositionSkipped}}
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("a deliberate drop was reported as a failure: %v", err)
	}
	_, args := rt.tx.statementMentioning(t, "activity_id = nullif")
	if args[1] != stateLanded {
		t.Fatalf("a skipped request was left in state %v, so the next tick offers it again", args[1])
	}
	if args[2] != "" {
		t.Fatalf("a skipped request named activity %v; a skip creates no entry to name", args[2])
	}
}

// A request nothing will ever land parks on the FIRST attempt rather than
// spending five ticks at the head of a queue, and parking is never a silent
// drop: the row stays, carrying the class, and a ledger row says what happened.
func TestARequestNothingWillEverLandParksVisiblyAtOnce(t *testing.T) {
	t.Parallel()
	rt := draining(queuedRow(firstRequestID, 0, "not the document at all"))
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("a request only its own sender can fix failed the tick: %v", err)
	}
	if len(rt.ingested) != 0 {
		t.Fatal("a document this connector cannot read was handed to the core anyway")
	}
	_, args := rt.tx.statementMentioning(t, "last_error_class = $3")
	if args[1] != stateParked {
		t.Fatalf("the request was left in state %v; nothing will ever land it", args[1])
	}
	if args[2] != classPayloadUnusable.Class {
		t.Fatalf("the row records class %v", args[2])
	}
	if len(rt.tx.audited) != 1 || rt.tx.audited[0].ID != firstRequestID {
		t.Fatalf("parking recorded %d ledger row(s); a message this installation accepted and will never act on is a fact somebody asks about", len(rt.tx.audited))
	}
	if rt.tx.published[0].Verb != eventRequestParked {
		t.Fatalf("parking published %q", rt.tx.published[0].Verb)
	}
}

// A stall that a human can repair keeps its place in the queue until the cap,
// and parks there — visibly — rather than being retried forever or dropped.
func TestARepeatedlyStallingRequestKeepsItsPlaceUntilTheCap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		attempts int
		want     string
		ledger   int
	}{
		{"below the cap", 0, stateWaiting, 0},
		{"one short of it", maxDrainAttempts - 2, stateWaiting, 0},
		{"reaching it", maxDrainAttempts - 1, stateParked, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := draining(queuedRow(firstRequestID, tc.attempts, landable(t, "m-1")))
			rt.ingestErrs = []error{fmt.Errorf("%w: that member was archived", extension.ErrForbidden)}
			err := drain(context.Background(), rt)
			if class, marked := extension.FailureClassOf(err); !marked || class.Class != classMemberNotPermitted.Class {
				t.Fatalf("the tick reported %v; a stall nobody else shares is still the tick landing nothing", err)
			}
			_, args := rt.tx.statementMentioning(t, "last_error_class = $3")
			if args[1] != tc.want {
				t.Fatalf("after %d prior attempt(s) the request is in state %v, want %v", tc.attempts, args[1], tc.want)
			}
			if len(rt.tx.audited) != tc.ledger {
				t.Fatalf("it recorded %d ledger row(s), want %d — parking is the fact worth recording and a retry is not", len(rt.tx.audited), tc.ledger)
			}
		})
	}
}

// A capture pipeline nobody can reach POSTPONES the tick rather than failing it.
// Failing spends the child's attempts and discards the row, which at this cadence
// manufactures one piece of dead work a minute for the length of an outage — each
// of them a banner saying a human must intervene in work no human can help with.
func TestAnUnreachableCapturePipelinePostponesTheTick(t *testing.T) {
	t.Parallel()
	rt := draining(
		queuedRow(firstRequestID, 0, landable(t, "m-1")),
		queuedRow(secondRequestID, 0, landable(t, "m-2")),
	)
	rt.ingestErrs = []error{errors.New("dial: connection refused"), errors.New("dial: connection refused")}
	err := drain(context.Background(), rt)
	class, marked := extension.FailureClassOf(err)
	if !marked || class.Class != classCaptureUnavailable.Class {
		t.Fatalf("the tick reported %v", err)
	}
	delay, postponed := extension.RescheduleAfter(err)
	if !postponed {
		t.Fatal("an outage failed the tick, so an outage of any length manufactures dead work at the cadence")
	}
	if delay != pollRetryDelay {
		t.Fatalf("it postponed by %s and the dispatcher ticks every %s", delay, pollRetryDelay)
	}
}

// A tick whose own context is done did not meet an outage: it met its own window,
// and every later tick spends the same window and expires in the same place.
// Postponing that hides a drain that can never finish.
func TestATickThatRanOutOfItsOwnWindowFailsRatherThanPostponing(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rt := draining(queuedRow(firstRequestID, 0, landable(t, "m-1")))
	rt.ingestErrs = []error{errors.New("dial: connection refused")}
	err := drain(ctx, rt)
	if _, postponed := extension.RescheduleAfter(err); postponed {
		t.Fatal("a tick that ran out of wall clock postponed itself, which hides a fan-out that can never finish behind a row that looks patient")
	}
	if class, marked := extension.FailureClassOf(err); !marked || class.Class != classCaptureUnavailable.Class {
		t.Fatalf("the tick reported %v", err)
	}
}

// A composition that does not admit this unit's ingress needs a human. Postponing
// it would be the unit quietly deciding not to tell anybody.
func TestBeingRefusedAtTheCapturePortFailsRatherThanPostponing(t *testing.T) {
	t.Parallel()
	rt := draining(queuedRow(firstRequestID, 0, landable(t, "m-1")))
	rt.ingestErrs = []error{extension.ErrIngressNotDeclared}
	err := drain(context.Background(), rt)
	if _, postponed := extension.RescheduleAfter(err); postponed {
		t.Fatal("a mis-composed installation postponed itself, so nothing on any screen would ever say so")
	}
	if class, _ := extension.FailureClassOf(err); class.Class != classCaptureNotDeclared.Class {
		t.Fatalf("the tick reported class %q", class.Class)
	}
}

// Requests stalling for DIFFERENT reasons is not one outage, and the class for it
// says so rather than picking one request's cause to speak for the rest.
func TestRequestsStallingDifferentlyAreNotReportedAsOneOutage(t *testing.T) {
	t.Parallel()
	rt := draining(
		queuedRow(firstRequestID, 0, landable(t, "m-1")),
		queuedRow(secondRequestID, 0, landable(t, "m-2")),
	)
	rt.ingestErrs = []error{
		errors.New("dial: connection refused"),
		fmt.Errorf("%w: that member was archived", extension.ErrForbidden),
	}
	err := drain(context.Background(), rt)
	if _, postponed := extension.RescheduleAfter(err); postponed {
		t.Fatal("several unrelated problems postponed the tick, so none of their owners is told")
	}
	if class, _ := extension.FailureClassOf(err); class.Class != classEveryRequestFailed.Class {
		t.Fatalf("the tick reported class %q", class.Class)
	}
}

// One request's failure does not stop the others, and a tick that landed anything
// is a tick that worked: reporting it as failed would put a banner on a pass that
// delivered messages.
func TestOneStalledRequestDoesNotFailATickThatLandedAnother(t *testing.T) {
	t.Parallel()
	rt := draining(
		queuedRow(firstRequestID, 0, landable(t, "m-1")),
		queuedRow(secondRequestID, 0, landable(t, "m-2")),
	)
	rt.results = []extension.Result{accepted()}
	rt.ingestErrs = []error{nil, errors.New("dial: connection refused")}
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("a tick that landed a message reported %v", err)
	}
	if len(rt.ingested) != 2 {
		t.Fatalf("the tick stopped after %d record(s); one request's failure is not the others' problem", len(rt.ingested))
	}
}

// The batch is BOUNDED and taken oldest first. Unbounded, one flooded endpoint
// spends every other member's turn and the child's wall clock with it.
func TestTheTickTakesABoundedBatchOldestFirst(t *testing.T) {
	t.Parallel()
	rt := draining()
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("draining an empty queue: %v", err)
	}
	sql, args := rt.tx.statementMentioning(t, "ORDER BY q.received_at")
	if !strings.Contains(sql, "LIMIT $2") {
		t.Fatalf("the batch read is unbounded:\n%s", sql)
	}
	if args[0] != stateWaiting || args[1] != drainBatch {
		t.Fatalf("the read took %v rows in state %v", args[1], args[0])
	}
}

// An empty queue is not a failure, and a tick that found nothing must not report
// one: at this cadence that is a dead row a minute saying a schedule ran.
func TestAnEmptyQueueIsNotAFailure(t *testing.T) {
	t.Parallel()
	rt := draining()
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("an empty queue reported %v", err)
	}
	if len(rt.tx.audited) != 0 {
		t.Fatal("a tick that found nothing wrote a ledger row, which is one per cadence forever to say a schedule ran")
	}
}
