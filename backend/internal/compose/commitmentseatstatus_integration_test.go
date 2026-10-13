// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// taskWorthyConfidence is above CommitmentTaskConfidence, so the rule takes the
// direct-task branch — which is the branch these tests are about.
const taskWorthyConfidence = 0.95

const seatStatusPromise = "I will send the revised pricing by Friday."

// A promise in mail from a colleague who cannot hold work is proposed, and the pass
// finishes.
//
// An automatic writer may only assign to an ACTIVE seat, and on an installation whose
// history arrived by import most seats on mail never signed in or have left. Writing
// the task directly met that refusal inside the conversation's own transaction, which
// failed the conversation and the pass with it — and a failed conversation's
// watermark does not move, so the same message was read again every hour, for ever.
//
// The promise is still owed to the customer, so it goes to a human as a proposal.
func TestAMailedPromiseByAColleagueWhoCannotHoldWorkIsProposed(t *testing.T) {
	for _, status := range []string{"invited", "deactivated"} {
		t.Run(status, func(t *testing.T) {
			e, reply := seedMailedPromise(t, status)
			if err := runSignalPass(t, e, reply); err != nil {
				t.Fatalf("the pass failed on a promise by an %s colleague: %v\n"+
					"every later pass reads the same conversation again, because a failed "+
					"conversation's watermark does not move", status, err)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
				t.Errorf("%d task(s) written to an %s seat", n, status)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 1 {
				t.Errorf("%d commitment proposals staged, want one: a promise nobody can be "+
					"given is still owed, and dropping it loses what the customer was told", n)
			}
		})
	}
}

// An ACTIVE colleague's promise is still written straight to them.
//
// The baseline the cases above are measured against: without it, a change that stopped
// writing commitment tasks at all would satisfy both of them.
func TestAMailedPromiseByAnActiveColleagueIsStillWrittenDirectly(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed on an active colleague's promise: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 1 {
		t.Errorf("%d task(s) written for an active colleague, want one", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d proposals staged for an active colleague, who can simply be given the "+
			"task", n)
	}
}

// seedMailedPromise lands one outbound message carrying a promise, sent by a seat in
// the named status, read surely enough to be written as a task rather than proposed.
func seedMailedPromise(t *testing.T, status string) (*integration.Env, string) {
	t.Helper()
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := e.SeedContact(t, "Ines Huber", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, ines, company)
	if status != "active" {
		e.WsExec(t, `UPDATE app_user SET status = $2 WHERE id = $1`, e.Rep2, status)
	}
	sent := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	message := integration.SeedIDRow(t, integration.OwnerConn(t), `INSERT INTO activity
		(id, kind, direction, subject, body, thread_key, occurred_at, created_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Pricing', 'Hi Ines, `+seatStatusPromise+`', 'thread-seat',
		        '`+sent+`', '`+sent+`', 'gmail', 'connector:gmail')`)
	integration.LinkActivity(t, integration.OwnerConn(t), message, "contact", ines)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, contact_id, role)
		VALUES ($1, $2, NULL, 'from')`, message, e.Rep2)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, contact_id, role)
		VALUES ($1, NULL, $2, 'to')`, message, ines)
	return e, mailedCommitmentReply(t, message, taskWorthyConfidence)
}

// mailedCommitmentReply is the one event the model is arranged to return.
func mailedCommitmentReply(t *testing.T, message ids.UUID, confidence float64) string {
	t.Helper()
	reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "message_id": message.String(),
		"summary": "Send the revised pricing", "quote": seatStatusPromise,
		"due_date": "", "confidence": confidence,
	}}})
	if err != nil {
		t.Fatalf("building the model reply: %v", err)
	}
	return string(reply)
}

// runSignalPass runs the hourly pass the way the worker does.
func runSignalPass(t *testing.T, e *integration.Env, reply string) error {
	t.Helper()
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), signalScanActor)
	_, err := NewSignalExtractor(e.Pool, cannedBrain{reply: reply}, time.Now, slog.Default()).
		RunWorkspace(pass, ids.From[ids.WorkspaceKind](e.WS))
	return err
}

// The conversion settles every signal it is owed, not only the first.
//
// The owed signals are read OLDEST FIRST, so a signal the loop cannot get past heads
// the list on every later pass and every signal behind it waits for as long as that
// one is broken. With a thousand owed, one record is a permanent ceiling on the whole
// backlog — which is why the loop charges a failure to its own signal and carries on.
//
// What this holds is the carrying on: two signals are owed, and a pass settles both.
func TestTheLegacyConversionSettlesEverySignalItIsOwed(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	older := seedOwedCommitmentSignal(t, e, "thread-older", -72*time.Hour)
	newer := seedOwedCommitmentSignal(t, e, "thread-newer", -71*time.Hour)

	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	for _, c := range []struct {
		name   string
		signal ids.UUID
	}{{"older", older}, {"newer", newer}} {
		// Still open is the observable state of a signal the conversion never
		// reached, and the figure the production report counted: 0 of 1,016
		// settled, every one still open.
		if n := e.WsCount(t,
			`SELECT count(*) FROM signal WHERE id = $1 AND status = 'open'`, c.signal); n != 0 {
			t.Errorf("the %s owed signal is still open: a pass that stops at one leaves every "+
				"signal behind it waiting on that one for ever", c.name)
		}
	}
}

// seedOwedCommitmentSignal lands an open commitment signal owed a reading, citing a
// message of its own so each is converted independently.
func seedOwedCommitmentSignal(t *testing.T, e *integration.Env, thread string, age time.Duration) ids.UUID {
	t.Helper()
	company := e.SeedCompany(t, "Owed "+thread, &e.Rep1)
	contact := e.SeedContact(t, "Owed Contact "+thread, &e.Rep1)
	// Employed BY that company, or the thread resolves to no account and the
	// conversion settles an empty reading — which looks like success and exercises
	// none of the dispatch.
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, contact, company)
	at := time.Now().Add(age).UTC().Format(time.RFC3339Nano)
	message := integration.SeedIDRow(t, integration.OwnerConn(t), `INSERT INTO activity
		(id, kind, direction, subject, body, thread_key, occurred_at, created_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Owed `+thread+`', 'Hi, `+seatStatusPromise+` (`+thread+`)', '`+thread+`',
		        '`+at+`', '`+at+`', 'gmail', 'connector:gmail')`)
	integration.LinkActivity(t, integration.OwnerConn(t), message, "contact", contact)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, contact_id, role)
		VALUES ($1, $2, NULL, 'from')`, message, e.Rep1)
	// entity_type is required while resolution_state is 'resolved', which is its
	// default: signal_resolved_has_entity refuses a resolved signal that names no
	// record, and a commitment signal names the account it is owed on.
	return integration.SeedIDRow(t, integration.OwnerConn(t), `INSERT INTO signal
		(id, kind, source_channel, entity_type, entity_id, resolved_company_id,
		 resolution_state, severity, summary, status, detected_at, source, captured_by, evidence)
		VALUES ($1, 'commitment_made', 'derived', 'company', '`+company.String()+`',
		        '`+company.String()+`', 'resolved', 'info', 'An old promise', 'open', '`+at+`',
		        'signal-scan', 'system:signal-scan',
		        jsonb_build_array(jsonb_build_object('source_type', 'activity',
		                                             'source_id', '`+message.String()+`')))`)
}

// A signal the conversion cannot read costs only itself.
//
// The owed signals are read OLDEST FIRST, so one the loop cannot get past heads the
// list on every later pass and every signal behind it waits for as long as that one
// is broken. With a thousand owed, one record is a permanent ceiling on the backlog.
//
// The fault still comes back, after the rest have had their turn: a failure nobody
// sees is how one record becomes a silent ceiling.
func TestASignalTheConversionCannotReadCostsOnlyItself(t *testing.T) {
	e, _ := seedMailedPromise(t, "active")
	older := seedOwedCommitmentSignal(t, e, "thread-older", -72*time.Hour)
	newer := seedOwedCommitmentSignal(t, e, "thread-newer", -71*time.Hour)

	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), signalScanActor)
	brain := brainFailingOn{marker: "thread-older", reply: `{"events":[]}`}
	_, err := NewSignalExtractor(e.Pool, brain, time.Now, slog.Default()).
		RunWorkspace(pass, ids.From[ids.WorkspaceKind](e.WS))
	if err == nil {
		t.Error("the pass reported no fault for a signal it could not read, so one unconvertible " +
			"record becomes a ceiling nobody is told about")
	}

	if n := e.WsCount(t, `SELECT count(*) FROM signal WHERE id = $1 AND status = 'open'`, older); n != 1 {
		t.Errorf("the signal that could not be read was settled anyway, so its reading is lost")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM signal WHERE id = $1 AND status = 'open'`, newer); n != 0 {
		t.Errorf("the signal BEHIND the unreadable one is still open: it waits on a record that " +
			"heads the list every pass, which is a thousand-deep backlog going nowhere")
	}
}

// brainFailingOn answers every reading but one, which it refuses with a fault that is
// neither a deferral nor a refused reading — the shape that must cost one signal.
type brainFailingOn struct {
	marker string
	reply  string
}

func (b brainFailingOn) Complete(_ context.Context, req model.Request) (model.Response, error) {
	for _, message := range req.Messages {
		if strings.Contains(message.Content, b.marker) {
			return model.Response{}, errors.New("the model could not read this conversation")
		}
	}
	return model.Response{Text: b.reply}, nil
}

// A deferral stops the conversion and still carries what an earlier signal refused.
//
// The two faults answer different questions — one signal could not be read, and the
// workspace then ran out of budget — so reporting only the second leaves a recurring
// per-signal failure invisible on every pass that happens to end in a deferral.
//
// ai.IsDeferral reads through the join, so the caller still classifies the pass as
// deferred rather than failed.
func TestADeferralCarriesWhatAnEarlierSignalRefused(t *testing.T) {
	t.Parallel()
	refused := errors.New("the model could not read this conversation")
	joined := errors.Join(refused, ai.ErrBudgetDeferred)

	if !ai.IsDeferral(joined) {
		t.Error("the joined fault does not read as a deferral, so the pass is recorded as failed " +
			"and its next attempt is scheduled as if the budget were available")
	}
	if !errors.Is(joined, refused) {
		t.Error("the joined fault has lost the signal that could not be read, which is the one " +
			"that recurs every pass")
	}

	// And the pass separates them: the deferral is its state, the carried fault is
	// one of its failures. Reporting the deferral among the failures would schedule
	// the next attempt as though the budget were available; dropping the carried one
	// loses the signal that recurs.
	beside := faultsBesideADeferral(joined)
	if len(beside) != 1 || !errors.Is(beside[0], refused) {
		t.Errorf("the pass kept %v beside the deferral, want just the signal it could not read",
			beside)
	}
	for _, fault := range beside {
		if ai.IsDeferral(fault) {
			t.Error("the deferral is reported as one of the pass's failures, so a spent budget " +
				"reads as a fault and the next attempt is scheduled as if it were available")
		}
	}
}

// A fault reported ALONGSIDE a deferral, inside one error, still reaches the pass.
//
// A provider reports its own state as several errors joined together, and the whole of
// that reads as a deferral when any member does. Asking only the outermost members
// would drop such a group whole, taking the signal fault beside the sentinel with it —
// and that fault is the one that recurs.
func TestAFaultJoinedToADeferralInsideOneErrorStillReachesThePass(t *testing.T) {
	t.Parallel()
	refused := errors.New("the model could not read this conversation")
	fromTheProvider := errors.Join(refused, ai.ErrProviderDown)

	beside := faultsBesideADeferral(errors.Join(fromTheProvider))
	if len(beside) != 1 || !errors.Is(beside[0], refused) {
		t.Errorf("the pass kept %v, want the signal it could not read: a fault joined to the "+
			"sentinel rather than beside it is still a fault", beside)
	}
}

// A provider ALREADY blocked carries no cause, and no cause is not a fault.
//
// The failure that blocked it was recorded on the call that did; every call behind
// that one is refused without one. Keeping the absence makes a pass that only ever
// deferred report a failure, and the next attempt is then scheduled as though the
// provider were up.
func TestAProviderAlreadyBlockedLeavesThePassWithNothingToReport(t *testing.T) {
	t.Parallel()
	blocked := &ai.ProviderDownError{Provider: "anthropic", RetryAfter: time.Now().Add(time.Hour)}

	if beside := faultsBesideADeferral(errors.Join(blocked)); len(beside) != 0 {
		t.Errorf("the pass kept %v from a provider that reported no cause, so a deferral-only "+
			"pass reads as failed and is retried as though the provider were up", beside)
	}
}

// A promise in PRIVATE mail is proposed to the conversation's own owner, whatever
// their status.
//
// Nobody else may be shown the mail, so the owner is the only reviewer there is —
// and an owner who has not signed in yet finds the proposal waiting when they do,
// which is what an invited colleague's queue is for. Clearing the seat here would
// stage it for whoever reviews the queue, which means showing them a conversation
// the product keeps to one seat.
//
// No TASK, though: the automatic writer still may not assign to them.
func TestAPrivatePromiseIsProposedToItsOwnOwner(t *testing.T) {
	e, reply := seedMailedPromise(t, "invited")
	// Capture-private to the seat that sent it: a contact visible to its owner alone
	// keeps every message filed against it to that owner.
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $1
		 WHERE id IN (SELECT contact_id FROM activity_link
		               WHERE entity_type = 'contact' AND contact_id IS NOT NULL)`, e.Rep2)

	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed on a private promise: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("%d task(s) written to a seat that cannot hold work", n)
	}
	// Named to the owner, not to nobody: an unnamed proposal on a private
	// conversation is the mail shown to whoever opens the queue.
	named := e.WsScalar(t, `
		SELECT coalesce(proposed_change->>'seat_id', '')
		  FROM approval WHERE kind = 'commitment_task'`)
	if named != e.Rep2.String() {
		t.Errorf("the private proposal names %q, want the conversation's owner %s: anyone else "+
			"reviewing it is being shown mail kept to one seat", named, e.Rep2)
	}
}

// An AGENT seat cannot hold work either, and a promise naming one is proposed.
//
// Its own case because an agent is refused by a different arm of the same check —
// AgentAssigneeError rather than not-found — and a dispatcher reading only the
// not-found answer would mint a task onto an agent seat that fails to move afterwards.
func TestAPromiseNamingAnAgentSeatIsProposed(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	e.WsExec(t, `UPDATE app_user SET is_agent = true WHERE id = $1`, e.Rep2)

	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed on a promise naming an agent seat: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("%d task(s) written to an agent seat", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 1 {
		t.Errorf("%d proposals staged, want one", n)
	}
}

// A private promise naming NOBODY still reaches its owner.
//
// The seat comes off the message's own sender, so a thread whose sender is not a
// colleague names none — and the question "can this seat hold work" has no subject.
// Answering it as no and stopping there would drop the promise, since a private
// conversation has exactly one reader to propose to and that reader is unaffected by
// whoever sent the mail.
func TestAPrivatePromiseNamingNobodyStillReachesItsOwner(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	// No colleague on the line: the sender participant goes, so nothing names a seat.
	e.WsExec(t, `DELETE FROM activity_participant WHERE role = 'from' AND user_id IS NOT NULL`)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $1
		 WHERE id IN (SELECT contact_id FROM activity_link
		               WHERE entity_type = 'contact' AND contact_id IS NOT NULL)`, e.Rep2)

	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed on a private promise naming nobody: %v", err)
	}

	named := e.WsScalar(t, `
		SELECT coalesce(proposed_change->>'seat_id', '')
		  FROM approval WHERE kind = 'commitment_task'`)
	if named != e.Rep2.String() {
		t.Errorf("the proposal names %q, want the conversation's owner %s: a promise with no "+
			"sender still belongs to the one colleague who may read the thread", named, e.Rep2)
	}
}
