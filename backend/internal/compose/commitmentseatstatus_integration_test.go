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
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// taskWorthyConfidence is above CommitmentTaskConfidence, so the rule takes the
// direct-task branch — which is the branch these tests are about.
const taskWorthyConfidence = 0.95

const seatStatusPromise = "I will send the revised pricing by Friday."

// A promise in mail from a colleague who has left does not fail the pass, and
// asks nobody.
//
// Writing the task directly met the writer's refusal inside the conversation's
// own transaction, which failed the conversation and the pass with it. A failed
// conversation's watermark does not move. A request names a member, and one
// who has left can be asked nothing, so nothing is staged for them.
func TestAMailedPromiseByADeactivatedColleagueAsksNobody(t *testing.T) {
	e, reply := seedMailedPromise(t, "deactivated")
	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed on a promise by a deactivated colleague: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("%d task(s) written to a deactivated seat", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d commitment proposals staged for a deactivated promiser, want none", n)
	}
}

// An archived colleague's promise asks nobody: no task, no proposal, and so no
// notice or email to anyone else in their place.
func TestAMailedPromiseByAnArchivedColleagueAsksNobody(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	e.WsExec(t, `UPDATE app_user SET archived_at = now() WHERE id = $1`, e.Rep2)
	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed on a promise by an archived colleague: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("%d task(s) written for an archived promiser, want none", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d proposal(s) staged for an archived promiser, want none", n)
	}
	// The finding stays on its source, under Commitment review.
	if n := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission
		WHERE outcome = 'promiser_not_assignable' AND NOT current_work`); n != 1 {
		t.Errorf("the unassignable promise is recorded %d time(s) on its source, want once", n)
	}
}

// An active or an invited colleague's promise is written straight to them. An
// invited colleague is a real colleague; the task waits until they sign in, as
// a task made for them by hand does.
//
// The baseline the cases above are measured against: without it, a change that stopped
// writing commitment tasks at all would satisfy them.
func TestAMailedPromiseByAnActiveColleagueIsStillWrittenDirectly(t *testing.T) {
	for _, status := range []string{"active", "invited"} {
		t.Run(status, func(t *testing.T) {
			e, reply := seedMailedPromise(t, status)
			if err := runSignalPass(t, e, reply); err != nil {
				t.Fatalf("the pass failed on an %s colleague's promise: %v", status, err)
			}
			if got := e.WsScalar(t, `SELECT coalesce(string_agg(assignee_id::text, ','), '') FROM activity WHERE kind = 'task'`); got != e.Rep2.String() {
				t.Errorf("the task belongs to %q, want the %s colleague who made the promise", got, status)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
				t.Errorf("%d proposals staged for an %s colleague, who can simply be given the task", n, status)
			}
		})
	}
}

// An unsure promise is proposed to the colleague who made it and to nobody else.
// An invited colleague's card waits the commitment horizon for them to sign in;
// an active colleague's keeps the usual window.
func TestAnUnsurePromiseIsProposedToItsColleagueAlone(t *testing.T) {
	for status, window := range map[string]time.Duration{
		"active":  72 * time.Hour,
		"invited": 30 * 24 * time.Hour,
	} {
		t.Run(status, func(t *testing.T) {
			e, _ := seedMailedPromise(t, status)
			message, err := ids.Parse(e.WsScalar(t, `SELECT activity_id::text FROM activity_identity WHERE identity_kind='mail' AND identity_key='thread-seat@buyer.example'`))
			if err != nil {
				t.Fatal(err)
			}
			if err := runSignalPass(t, e, mailedCommitmentReply(t, message, e.Rep2, 0.75)); err != nil {
				t.Fatalf("the pass failed on an unsure promise: %v", err)
			}
			id, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM approval WHERE kind = 'commitment_task'
				 AND proposed_change->>'seat_id' = $1 AND proposed_change->>'promiser_seat_id' = $1
				 AND on_behalf_of::text = $1 AND requested_decider_id::text = $1`, e.Rep2.String()))
			if err != nil {
				t.Fatalf("no proposal names the colleague who made the promise: %v", err)
			}
			hours := e.WsCount(t, `SELECT round(extract(epoch FROM expires_at - created_at) / 3600)::int FROM approval WHERE id = $1`, id)
			if time.Duration(hours)*time.Hour != window {
				t.Errorf("the %s colleague's card waits %dh, want %s", status, hours, window)
			}
			queue := approvalsServiceWithEffects(e.Pool)
			for name, reader := range map[string]context.Context{
				"the admin":         e.Admin(),
				"a third colleague": e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms),
			} {
				if could, err := queue.PendingDecidableBy(reader, ids.From[ids.ApprovalKind](id)); err != nil || could {
					t.Errorf("%s may decide a colleague's promise (could=%v, err=%v)", name, could, err)
				}
				if _, err := queue.Decide(reader, ids.From[ids.ApprovalKind](id), true, nil); err == nil {
					t.Errorf("%s accepted a colleague's promise and became its owner", name)
				}
			}
		})
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
	message := captureSeatPromise(t, e, ines, "thread-seat@buyer.example", seatStatusPromise, time.Now().Add(-48*time.Hour))
	if status != "active" {
		e.WsExec(t, `UPDATE app_user SET status = $2 WHERE id = $1`, e.Rep2, status)
	}
	return e, mailedCommitmentReply(t, message, e.Rep2, taskWorthyConfidence)
}

// mailedCommitmentReply is the one event the model is arranged to return.
func mailedCommitmentReply(t *testing.T, message, promiser ids.UUID, confidence float64) string {
	t.Helper()
	reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "message_id": message.String(),
		"summary": "Send the revised pricing", "quote": seatStatusPromise,
		"due_date": "", "confidence": confidence, "promiser_ref": "seat:" + promiser.String(),
		"commitment": openCommitmentMeaning("", ""),
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

	replies := map[string]string{}
	read := map[string]bool{}
	for thread, signal := range map[string]ids.UUID{"thread-older": older, "thread-newer": newer} {
		message := ids.MustParse(e.WsScalar(t, `SELECT evidence->0->>'source_id' FROM signal WHERE id=$1`, signal))
		replies[thread] = mailedCommitmentReply(t, message, e.Rep1, taskWorthyConfidence)
	}
	brain := offerDraftCompleterStub{reply: func(req model.Request) string {
		for _, message := range req.Messages {
			for thread, answer := range replies {
				if strings.Contains(message.Content, thread) {
					read[thread] = true
					return answer
				}
			}
		}
		return reply
	}}
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), signalScanActor)
	if _, err := NewSignalExtractor(e.Pool, brain, func() time.Time { return clocktest.Now(t) }, slog.Default()).RunWorkspace(pass, ids.From[ids.WorkspaceKind](e.WS)); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	for thread := range replies {
		if !read[thread] {
			t.Fatalf("the conversion never read %s", thread)
		}
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
	// The create audit a real capture writes on the seat's behalf. Without it
	// the seat row is the shape the legacy backfill guessed, which no reader
	// takes as the rep having sent the mail (auth.ParticipantStandsSQL).
	e.WsExec(t, `INSERT INTO audit_log (id, actor_type, actor_id, on_behalf_of, action, entity_type, entity_id)
		VALUES (uuidv7(), 'connector', 'connector:gmail', $1, 'create', 'activity', $2)`, e.Rep1, message)
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

// A promise in PRIVATE mail made by the mailbox's own owner is their task, also
// while they are invited. They may read the mail, so they may hold the work.
func TestAPrivatePromiseByItsInvitedOwnerIsTheirTask(t *testing.T) {
	e, reply := seedMailedPromise(t, "invited")
	// Capture-private to the seat that sent it: a contact visible to its owner alone
	// keeps every message filed against it to that owner.
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $1
		 WHERE id IN (SELECT contact_id FROM activity_link
		               WHERE entity_type = 'contact' AND contact_id IS NOT NULL)`, e.Rep2)

	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed on a private promise: %v", err)
	}
	if got := e.WsScalar(t, `SELECT coalesce(string_agg(assignee_id::text, ','), '') FROM activity WHERE kind = 'task'`); got != e.Rep2.String() {
		t.Errorf("the private promise's task belongs to %q, want its owner %s", got, e.Rep2)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d proposals staged beside the owner's own task", n)
	}
}

// An AGENT seat cannot hold work or decide, and a promise naming one stages nothing.
//
// Its own case because an agent is refused by a different arm of the same check —
// AgentAssigneeError rather than not-found — and a dispatcher reading only the
// not-found answer would mint a task onto an agent seat that fails to move afterwards.
func TestAPromiseNamingAnAgentSeatStagesNothing(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	e.WsExec(t, `UPDATE app_user SET is_agent = true WHERE id = $1`, e.Rep2)

	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed on a promise naming an agent seat: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("%d task(s) written to an agent seat", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d proposals staged for an agent seat, which cannot be asked to decide", n)
	}
}

// A private promise naming nobody asks nobody, not even the conversation's
// owner: taking on a promise somebody else made is explicit, never a fallback.
// The finding stays on its source, which only that owner may read.
func TestAPrivatePromiseNamingNobodyAsksNobody(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	reply = mailedReplyWithoutPromiser(t, reply)
	// No colleague on the line: the sender participant goes, so nothing names a seat.
	e.WsExec(t, `DELETE FROM activity_participant WHERE role = 'from' AND user_id IS NOT NULL`)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $1
		 WHERE id IN (SELECT contact_id FROM activity_link
		               WHERE entity_type = 'contact' AND contact_id IS NOT NULL)`, e.Rep2)

	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the pass failed on a private promise naming nobody: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d proposal(s) put to the mailbox owner for a promise nobody made by name, want none", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("%d task(s) written for a promise nobody made by name, want none", n)
	}
}

func captureSeatPromise(t *testing.T, e *integration.Env, contact ids.UUID, key, body string, sent time.Time) ids.UUID {
	t.Helper()
	buyer := "ines@buyer.example"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Emails: []contacts.ContactEmailInput{{Email: buyer, EmailType: "work", IsPrimary: true}}}); err != nil {
		t.Fatal(err)
	}
	address := seatAddress(t, e, e.Rep2)
	raw := strings.Join([]string{"From: " + address, "To: " + buyer, "Subject: Pricing", "Date: " + sent.Format(time.RFC1123Z), "Message-ID: <" + key + ">", "Content-Type: text/plain; charset=utf-8", "", "Hi Ines, " + body}, "\r\n")
	captureThreadMail(t, e, e.Rep2, []byte(raw))
	message, err := ids.Parse(e.WsScalar(t, `SELECT activity_id::text FROM activity_identity WHERE identity_kind='mail' AND identity_key=$1`, key))
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	actor, ok := principal.Actor(reader)
	if !ok {
		t.Fatal("fixture has no acting colleague")
	}
	actor.SeatType = principal.SeatFull
	reader = principal.WithActor(reader, actor)
	if _, err := NewThreadAudienceSetter(e.Pool).Decide(reader, key, true); err != nil {
		t.Fatal(err)
	}
	return message
}

func mailedReplyWithoutPromiser(t *testing.T, reply string) string {
	t.Helper()
	var payload extractPayload
	if err := json.Unmarshal([]byte(reply), &payload); err != nil {
		t.Fatal(err)
	}
	for i := range *payload.Events {
		(*payload.Events)[i].PromiserRef = ""
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
