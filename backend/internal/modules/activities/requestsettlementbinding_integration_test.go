// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A verdict is bound to the request and answers it read. Each case below
// changes that evidence between the read and the commit, while the model call
// runs. Each verdict outcome must then be dropped.

// asSettler is the settlement pass's own principal.
func asSettler(e *loadEnv) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: RequestSettlementActor,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

// asEditor is a colleague who may change activities.
func asEditor(e *loadEnv) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"manager"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}, "contact": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

var bindingReadAt = requestInstant.Add(48 * time.Hour)

func settlementRows(t *testing.T, e *loadEnv, request ids.UUID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity_request_settlement WHERE request_activity_id = $1`, request).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func newestReplyID(t *testing.T, e *loadEnv, request ids.UUID) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(e.as(), `SELECT id FROM activity WHERE direction = 'outbound'
		AND thread_key = (SELECT thread_key FROM activity WHERE id = $1) ORDER BY occurred_at DESC LIMIT 1`, request).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAVerdictOverChangedEvidenceIsDroppedForEveryOutcome(t *testing.T) {
	changes := map[string]func(t *testing.T, e *loadEnv, request ids.UUID){
		"the answer was edited": func(t *testing.T, e *loadEnv, request ids.UUID) {
			body := "Edited: nothing was sent after all."
			if _, err := storeKnowing(e).UpdateActivity(asEditor(e), ids.From[ids.ActivityKind](newestReplyID(t, e, request)), UpdateActivityInput{Body: &body}); err != nil {
				t.Fatal(err)
			}
		},
		"the answer was narrowed": func(t *testing.T, e *loadEnv, request ids.UUID) {
			if _, err := storeKnowing(e).SetAudience(asEditor(e), ids.From[ids.ActivityKind](newestReplyID(t, e, request)), SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
				t.Fatal(err)
			}
		},
		"the request was edited": func(t *testing.T, e *loadEnv, request ids.UUID) {
			body := "Please send the report and the invoice."
			if _, err := storeKnowing(e).UpdateActivity(asEditor(e), ids.From[ids.ActivityKind](request), UpdateActivityInput{Body: &body}); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, change := range changes {
		for _, verdict := range []string{RequestSettled, RequestStillOwed, RequestUnsure} {
			t.Run(name+"/"+verdict, func(t *testing.T) {
				e := setupLoad(t)
				request := seedEmailRequest(t, e, "Bound evidence", "commitment", OwedVerdictAsksUs)
				replyTo(t, e, request, "Here is the report.", requestInstant.Add(time.Hour))
				candidate, ok := repliedIDs(t, e, bindingReadAt)[request]
				if !ok {
					t.Fatal("the answered request was not offered")
				}
				change(t, e, request)
				in := RequestSettlementInput{Request: candidate, Verdict: verdict, Confidence: 0.9, DecidedBy: "test-model"}
				if verdict == RequestStillOwed {
					in.Remaining = "Send the invoice"
				}
				if err := storeKnowing(e).SettleRequest(asSettler(e), in); err != nil {
					t.Fatal(err)
				}
				if got := settlementRows(t, e, request); got != 0 {
					t.Fatalf("a %s verdict over changed evidence was recorded", verdict)
				}
			})
		}
	}
}

// A verdict that read unchanged evidence commits and records what it read.
func TestAVerdictOverUnchangedEvidenceRecordsWhatItRead(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Unchanged evidence", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Here is the report.", requestInstant.Add(time.Hour))
	candidate := repliedIDs(t, e, bindingReadAt)[request]
	if candidate.Evidence == "" {
		t.Fatal("the candidate carries no evidence binding")
	}
	if err := storeKnowing(e).SettleRequest(asSettler(e), RequestSettlementInput{Request: candidate, Verdict: RequestUnsure, Confidence: 0.5, DecidedBy: "test-model"}); err != nil {
		t.Fatal(err)
	}
	var recorded string
	if err := e.owner.QueryRow(e.as(), `SELECT coalesce(judged_evidence, '') FROM activity_request_settlement WHERE request_activity_id = $1`, request).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != candidate.Evidence {
		t.Fatalf("judged_evidence = %q, want the evidence read %q", recorded, candidate.Evidence)
	}
}

// settleThroughPass settles the request as the pass would: read, then commit.
func settleThroughPass(t *testing.T, e *loadEnv, request ids.UUID, at time.Time, verdict string) RepliedRequest {
	t.Helper()
	candidate, ok := repliedIDs(t, e, at)[request]
	if !ok {
		t.Fatalf("the request was not offered at %s", at)
	}
	in := RequestSettlementInput{Request: candidate, Verdict: verdict, Confidence: 0.9, DecidedBy: "test-model"}
	if verdict == RequestStillOwed {
		in.Remaining = "Send the second half"
	}
	if err := storeKnowing(e).SettleRequest(asSettler(e), in); err != nil {
		t.Fatal(err)
	}
	return candidate
}

// An answer that arrives after a settled verdict is judged, not lost. A
// still-owed verdict over it reopens the reminder the pass itself completed.
func TestALaterAnswerToASettledRequestIsJudgedAndReopensThePassesReminder(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Settled then reopened", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Here is the first half.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))
	settleThroughPass(t, e, request, bindingReadAt, RequestSettled)
	if _, _, done, _ := taskFor(t, e, request); !done {
		t.Fatal("the settled verdict did not complete the reminder")
	}
	replyTo(t, e, request, "Sorry, the second half follows.", requestInstant.Add(50*time.Hour))
	settleThroughPass(t, e, request, requestInstant.Add(60*time.Hour), RequestStillOwed)
	if _, _, done, _ := taskFor(t, e, request); done {
		t.Fatal("a still-owed verdict left the pass's own completion standing")
	}
}

// A human reopening the pass's completion is a decision; the same evidence
// is not judged again, so the reminder cannot flap back to done.
func TestAHumanReopeningIsNotSettledAgainOnTheSameEvidence(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Human undo", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Here it is.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))
	settleThroughPass(t, e, request, bindingReadAt, RequestSettled)
	task, _, _, _ := taskFor(t, e, request)
	open := false
	if _, err := storeKnowing(e).UpdateActivity(asEditor(e), ids.From[ids.ActivityKind](task), UpdateActivityInput{IsDone: &open}); err != nil {
		t.Fatal(err)
	}
	if _, ok := repliedIDs(t, e, requestInstant.Add(72*time.Hour))[request]; ok {
		t.Fatal("the request was offered again over the evidence a human already overruled")
	}
	if _, _, done, _ := taskFor(t, e, request); done {
		t.Fatal("the human's reopening did not stand")
	}
}

// A reminder a human completed is never reopened by a later verdict, and a
// request older than the waiting horizon stays done as record information.
func TestALaterVerdictNeverReopensHumanOrAgedWork(t *testing.T) {
	for name, aged := range map[string]bool{"human completed": false, "past the horizon": true} {
		t.Run(name, func(t *testing.T) {
			e := setupLoad(t)
			request := seedEmailRequest(t, e, "Not reopened", "commitment", OwedVerdictAsksUs)
			replyTo(t, e, request, "Here it is.", requestInstant.Add(time.Hour))
			mintReminder(t, e, requestInstant.Add(2*time.Hour))
			settleThroughPass(t, e, request, bindingReadAt, RequestSettled)
			if !aged {
				// A human renames the completed reminder: it is theirs now.
				task, _, _, _ := taskFor(t, e, request)
				subject := "Report sent, confirmed by phone"
				if _, err := storeKnowing(e).UpdateActivity(asEditor(e), ids.From[ids.ActivityKind](task), UpdateActivityInput{Subject: &subject}); err != nil {
					t.Fatal(err)
				}
			}
			later := requestInstant.Add(50 * time.Hour)
			at := requestInstant.Add(60 * time.Hour)
			if aged {
				later, at = requestInstant.AddDate(0, 0, 120), requestInstant.AddDate(0, 0, 121)
			}
			replyTo(t, e, request, "One more thing.", later)
			settleThroughPass(t, e, request, at, RequestStillOwed)
			if _, _, done, _ := taskFor(t, e, request); !done {
				t.Fatalf("%s work was reopened", name)
			}
		})
	}
}

// holdOpen runs write in a transaction kept open until release, and returns
// once write ran, with that transaction's backend. Cleanup releases it too.
func holdOpen(t *testing.T, e *loadEnv, as func(*loadEnv) context.Context, write func(context.Context, pgx.Tx) error) (holder int32, release func() error) {
	t.Helper()
	ctx := as(e)
	held, let, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- storeKnowing(e).tx(ctx, func(tx pgx.Tx) error {
			if err := write(ctx, tx); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
				return err
			}
			close(held)
			<-let
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-done:
		t.Fatalf("the held transaction ended before it held anything: %v", err)
	}
	var once sync.Once
	var result error
	release = func() error {
		once.Do(func() {
			close(let)
			result = <-done
		})
		return result
	}
	t.Cleanup(func() {
		if err := release(); err != nil {
			t.Logf("the held transaction ended with %v", err)
		}
	})
	return holder, release
}

// startRacer runs write on its own goroutine, to queue behind a held
// transaction. Cleanup releases the hold before it waits, so a failure leaks nothing.
func startRacer(t *testing.T, release func() error, write func() error) (result <-chan error, returned <-chan struct{}) {
	t.Helper()
	out, done := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(done)
		out <- write()
	}()
	t.Cleanup(func() {
		if err := release(); err != nil {
			t.Logf("the held transaction ended with %v", err)
		}
		<-done
	})
	return out, done
}

// verdictBehind starts the verdict until one is seen queued behind holder. A
// verdict gives up as busy after settlementLockWait, so a missed one starts again.
func verdictBehind(t *testing.T, e *loadEnv, holder int32, release func() error, n int, in RequestSettlementInput) <-chan error {
	t.Helper()
	var result <-chan error
	var returned <-chan struct{}
	start := func() {
		result, returned = startRacer(t, release, func() error { return storeKnowing(e).SettleRequest(asSettler(e), in) })
	}
	start()
	// No racer channel: look restarts a verdict that returned, which is no failure.
	testdb.WaitForContention(t, nil, "",
		fmt.Sprintf("no verdict waited behind the held transaction within %s", testdb.ProbeBudget),
		func(ctx context.Context) (bool, error) {
			select {
			case <-returned:
				if err := <-result; err != nil {
					return false, err
				}
				if settlementRows(t, e, in.Request.RequestID) != 0 {
					t.Fatal("a verdict committed without waiting behind the held transaction")
				}
				start()
				return false, nil
			default:
			}
			queued, err := testdb.QueuedBehind(ctx, e.owner, holder)
			return queued >= n, err
		})
	return result
}

// An answer edit still uncommitted at the verdict's final check makes the
// verdict wait. Once the edit commits, the verdict sees it and is dropped.
func TestAVerdictWaitsForAnAnswerEditInFlightAndDropsItself(t *testing.T) {
	for _, verdict := range []string{RequestSettled, RequestStillOwed, RequestUnsure} {
		t.Run(verdict, func(t *testing.T) {
			e := setupLoad(t)
			request := seedEmailRequest(t, e, "Edit in flight", "commitment", OwedVerdictAsksUs)
			replyTo(t, e, request, "Here is the report.", requestInstant.Add(time.Hour))
			candidate := repliedIDs(t, e, bindingReadAt)[request]
			answer := newestReplyID(t, e, request)
			holder, release := holdOpen(t, e, asEditor, func(ctx context.Context, tx pgx.Tx) error {
				body := "Edited: the report is not attached."
				_, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](answer), UpdateActivityInput{Body: &body})
				return err
			})
			in := RequestSettlementInput{Request: candidate, Verdict: verdict, Confidence: 0.9, DecidedBy: "test-model"}
			if verdict == RequestStillOwed {
				in.Remaining = "Attach the report"
			}
			settled := verdictBehind(t, e, holder, release, 1, in)
			if err := release(); err != nil {
				t.Fatal(err)
			}
			if err := <-settled; err != nil {
				t.Fatal(err)
			}
			if got := settlementRows(t, e, request); got != 0 {
				t.Fatalf("a %s verdict committed over an answer edited during its check", verdict)
			}
		})
	}
}

// A verdict over evidence that later changes comes back to the pass. The
// cases are a backdated answer and an edit to the answer it read. Unchanged
// evidence stays skipped.
func TestChangedEvidenceBringsABoundVerdictBack(t *testing.T) {
	for name, change := range map[string]func(t *testing.T, e *loadEnv, request ids.UUID){
		"a backdated answer": func(t *testing.T, e *loadEnv, request ids.UUID) {
			replyTo(t, e, request, "Also, the appendix.", requestInstant.Add(2*time.Hour))
		},
		"an edited answer": func(t *testing.T, e *loadEnv, request ids.UUID) {
			body := "Edited: nothing was sent."
			if _, err := storeKnowing(e).UpdateActivity(asEditor(e), ids.From[ids.ActivityKind](newestReplyID(t, e, request)), UpdateActivityInput{Body: &body}); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupLoad(t)
			request := seedEmailRequest(t, e, "Bound then changed", "commitment", OwedVerdictAsksUs)
			replyTo(t, e, request, "Here is the report.", requestInstant.Add(3*time.Hour))
			settleThroughPass(t, e, request, bindingReadAt, RequestSettled)
			if _, ok := repliedIDs(t, e, bindingReadAt)[request]; ok {
				t.Fatal("unchanged evidence was offered again")
			}
			change(t, e, request)
			if _, ok := repliedIDs(t, e, bindingReadAt)[request]; !ok {
				t.Fatalf("after %s the settled request was not offered again", name)
			}
		})
	}
}

// A reply being written, through the real writer, when the verdict reaches its
// final check makes the verdict wait for it. Once the reply commits, the
// verdict sees it and is dropped, whatever it said.
func TestAVerdictWaitsForAReplyBeingWrittenAndDropsItself(t *testing.T) {
	for _, verdict := range []string{RequestSettled, RequestStillOwed, RequestUnsure} {
		t.Run(verdict, func(t *testing.T) {
			e := setupLoad(t)
			request := seedEmailRequest(t, e, "Reply in flight", "commitment", OwedVerdictAsksUs)
			replyTo(t, e, request, "Here is the report.", requestInstant.Add(time.Hour))
			candidate := repliedIDs(t, e, bindingReadAt)[request]
			var key string
			if err := e.owner.QueryRow(e.as(), `SELECT thread_key FROM activity WHERE id = $1`, request).Scan(&key); err != nil {
				t.Fatal(err)
			}
			holder, release := holdOpen(t, e, asClassifier, func(ctx context.Context, tx pgx.Tx) error {
				subject, direction, body, at := "Re: request", "outbound", "One more part.", requestInstant.Add(2*time.Hour)
				_, _, err := storeKnowing(e).LogActivityTx(ctx, tx, LogActivityInput{
					Kind: "email", Subject: &subject, Body: &body, Direction: &direction, ThreadKey: key, Source: "test",
					OccurredAt: &at, CounterpartyEmail: requestCounterparty, CounterpartyOutboundAttested: true,
				})
				return err
			})
			in := RequestSettlementInput{Request: candidate, Verdict: verdict, Confidence: 0.9, DecidedBy: "test-model"}
			if verdict == RequestStillOwed {
				in.Remaining = "Send the rest"
			}
			settled := verdictBehind(t, e, holder, release, 1, in)
			if err := release(); err != nil {
				t.Fatal(err)
			}
			if err := <-settled; err != nil {
				t.Fatal(err)
			}
			if got := settlementRows(t, e, request); got != 0 {
				t.Fatalf("a %s verdict committed without the reply written during its check", verdict)
			}
		})
	}
}

// A reply written while a verdict holds its evidence waits for that verdict
// to commit. The verdict never commits over a reply it did not see.
func TestAReplyWrittenAfterTheFinalCheckWaitsForTheVerdict(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Reply after the check", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Here is the report.", requestInstant.Add(time.Hour))
	holder, release := holdVerdict(t, e, request)
	var key string
	if err := e.owner.QueryRow(e.as(), `SELECT thread_key FROM activity WHERE id = $1`, request).Scan(&key); err != nil {
		t.Fatal(err)
	}
	written, returned := startRacer(t, release, func() error {
		subject, direction, body, at := "Re: request", "outbound", "One more part.", requestInstant.Add(2*time.Hour)
		_, _, err := storeKnowing(e).LogActivity(asClassifier(e), LogActivityInput{
			Kind: "email", Subject: &subject, Body: &body, Direction: &direction, ThreadKey: key, Source: "test",
			OccurredAt: &at, CounterpartyEmail: requestCounterparty, CounterpartyOutboundAttested: true,
		})
		return err
	})
	testdb.AwaitQueuedBehind(t, e.owner, holder, 1, returned, "the write")
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
}

// A settled verdict whose only answer is gone has nothing left to rest on. It
// becomes unsure, so the request is no longer hidden as answered, and the
// reminder the pass closed stays closed. A verdict with an answer left is not
// touched; it is judged again instead.
func TestASettledVerdictWhoseLastAnswerIsGoneIsWithdrawn(t *testing.T) {
	for name, answers := range map[string]int{"its only answer": 1, "one of two answers": 2} {
		t.Run(name, func(t *testing.T) {
			e := setupLoad(t)
			request := seedEmailRequest(t, e, "Answer removed", "commitment", OwedVerdictAsksUs)
			replyTo(t, e, request, "Here is the report.", requestInstant.Add(time.Hour))
			if answers == 2 {
				replyTo(t, e, request, "And the appendix.", requestInstant.Add(3*time.Hour))
			}
			mintReminder(t, e, requestInstant.Add(4*time.Hour))
			settleThroughPass(t, e, request, bindingReadAt, RequestSettled)
			e.exec(t, `UPDATE activity SET archived_at = now() WHERE id = (SELECT id FROM activity WHERE direction = 'outbound'
				AND thread_key = (SELECT thread_key FROM activity WHERE id = $1) ORDER BY occurred_at LIMIT 1)`, request)
			withdrawn, err := storeKnowing(e).WithdrawLostSettlements(asSettler(e), bindingReadAt, 10)
			if err != nil {
				t.Fatal(err)
			}
			var verdict string
			if err := e.owner.QueryRow(e.as(), `SELECT verdict FROM activity_request_settlement WHERE request_activity_id = $1`, request).Scan(&verdict); err != nil {
				t.Fatal(err)
			}
			if answers == 1 && (withdrawn != 1 || verdict != RequestUnsure) {
				t.Fatalf("withdrawn %d, verdict %q: a verdict with no answer left still claims the request was answered", withdrawn, verdict)
			}
			if answers == 2 {
				if withdrawn != 0 || verdict != RequestSettled {
					t.Fatalf("withdrawn %d, verdict %q: a verdict with an answer left was withdrawn", withdrawn, verdict)
				}
				if _, ok := repliedIDs(t, e, bindingReadAt)[request]; !ok {
					t.Fatal("the verdict whose evidence changed was not offered again")
				}
			}
			if _, _, done, _ := taskFor(t, e, request); !done {
				t.Fatal("the reminder the pass closed was reopened")
			}
		})
	}
}
