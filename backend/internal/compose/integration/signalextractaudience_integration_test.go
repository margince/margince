// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What the signal extraction pass may READ, as against what it concludes.
// Apart from signalextract_integration_test.go, which is the pass's own
// judgement — which conversations are settled, which account they belong to,
// what a reading produces. This half is one question asked three ways: a
// conversation carrying a message its summary's readers may not open is not
// offered, is not read, and its body does not reach the model.

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A limited message takes its whole conversation out of the pass, and it does
// NOT fall back to the mailbox owner the way a capture-private RECORD does.
//
// The two look alike and are not. A record's owner visibility says one person is
// the reader, so a summary addressed to that person discloses nothing new. A
// limited audience says the content is withheld from readers who can still see
// the records the message is filed against, and an owner-scoped signal is a
// durable, searchable restatement of that content which outlives the message's
// own limit. There is no owner for whom extracting it is free.
func TestAThreadWithALimitedMessageIsNotOfferedToTheModel(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Acme", &e.Rep1)
	at := extractClock.Add(-48 * time.Hour)
	contact := employeeOf(t, e, org, "Ada at Acme")

	// Two messages on one conversation: one ordinary, one limited. The pass
	// reads a whole thread at once, so what it writes is as private as the most
	// private thing it read — the ordinary half is not a licence to read it.
	seedMessage(t, e, contact, "thread-limited", "Renewal", "Happy to continue.", "inbound", at)
	limited := seedMessage(t, e, contact, "thread-limited", "Renewal",
		"internal note: we are preparing to terminate", "inbound", at.Add(time.Minute))
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, limited); err != nil {
		t.Fatal(err)
	}

	brain := &scriptedBrain{reply: `{"events": []}`}
	if raised := extractPass(t, e, brain); raised != 0 {
		t.Fatalf("a conversation carrying a limited message was read: %d signals raised", raised)
	}
	if brain.calls != 0 {
		t.Errorf("the model was called %d times on a conversation carrying a limited message — its bodies were sent for summarising", brain.calls)
	}

	// Opening it makes the same conversation due, which proves the refusal was
	// the audience and not some other property of the fixture.
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE activity SET audience = 'workspace' WHERE id = $1`, limited); err != nil {
		t.Fatal(err)
	}
	reopened := &scriptedBrain{reply: `{"events": []}`}
	if extractPass(t, e, reopened); reopened.calls != 1 {
		t.Errorf("the re-opened conversation was read %d times, want 1 — the fixture was refused for something other than its audience", reopened.calls)
	}
}

// The window read carries the audience itself, because the pass that offers a
// conversation and the pass that reads it are different transactions.
//
// dueThreads commits, then each thread's readThread opens its own transaction.
// Under read-committed the window read is a later statement against a newer
// snapshot, so a message limited in between — by a human, or by a verdict, in
// another process — is invisible to the offer and visible here. A window that
// trusted the offer would send that message's body to the model.
//
// The race cannot be staged from inside the pass: by the time the pass calls
// the model for a thread, that thread's window has already been read. So the
// narrowing is applied between the two passes and the SECOND pass is what must
// not carry it — which reaches the same statement by the same route, with the
// concurrent writer standing in for another process.
func TestAThreadReadAgainAfterANarrowingCarriesOnlyItsOpenMessages(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Acme", &e.Rep1)
	at := extractClock.Add(-48 * time.Hour)
	contact := employeeOf(t, e, org, "Ada at Acme")

	seedMessage(t, e, contact, "thread-mid", "Renewal", "Happy to continue.", "inbound", at)
	late := seedMessage(t, e, contact, "thread-mid", "Renewal",
		"internal note: we are preparing to terminate", "inbound", at.Add(time.Minute))

	brain := &recordingBrain{reply: `{"events": []}`}
	extractor := compose.NewSignalExtractor(e.Pool, brain,
		func() time.Time { return extractClock }, slog.Default())
	if _, err := extractor.RunWorkspace(e.Admin(), ids.From[ids.WorkspaceKind](e.WS)); err != nil {
		t.Fatalf("signal extract: %v", err)
	}
	if len(brain.prompts) != 1 {
		t.Fatalf("the first pass made %d model call(s), want 1 — the fixture never reaches the window under test", len(brain.prompts))
	}
	if !strings.Contains(brain.prompts[0], "preparing to terminate") {
		t.Fatal("the open conversation did not carry both its messages — the assertion below could not tell a working clause from a short window")
	}

	// The narrowing, and the thread made due again. The OFFER now refuses this
	// conversation, so nothing should read it — but if it is reached by any
	// route, the window must not carry the limited message.
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, late); err != nil {
		t.Fatal(err)
	}
	if _, err := OwnerConn(t).Exec(context.Background(),
		`DELETE FROM signal_thread_scan WHERE thread_key = 'thread-mid'`); err != nil {
		t.Fatal(err)
	}
	before := len(brain.prompts)
	if _, err := extractor.RunWorkspace(e.Admin(), ids.From[ids.WorkspaceKind](e.WS)); err != nil {
		t.Fatalf("signal extract, second pass: %v", err)
	}
	for _, prompt := range brain.prompts[before:] {
		if strings.Contains(prompt, "preparing to terminate") {
			t.Error("a limited message's body reached the model on a later reading of its conversation")
		}
	}
}

// recordingBrain keeps every prompt it was given, so a test can assert on what
// the model was shown rather than on what the pass reported doing.
type recordingBrain struct {
	reply   string
	prompts []string
}

// The whole-thread audience test spans every email on the conversation, not
// only the connector-captured ones.
//
// The due-threads CTE keeps `captured_by LIKE 'connector:%'` — it is looking
// for conversations a mailbox sync produced — so a HAND-LOGGED limited message
// on a captured thread is invisible to it. The window read that follows is not
// connector-scoped, so that message is one the offer would not see and the
// reading would.
//
// Its body never reaches the model (the window excludes it), so this is not a
// text leak. What it refuses is the shape: a summary written from the rest of a
// conversation, presented to readers from whom one of its messages is withheld,
// with nothing on the summary to say a part is missing.
func TestAHandLoggedLimitedMessageTakesItsThreadOutOfThePass(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Acme", &e.Rep1)
	at := extractClock.Add(-48 * time.Hour)
	contact := employeeOf(t, e, org, "Ada at Acme")

	seedMessage(t, e, contact, "thread-mixed", "Renewal", "Happy to continue.", "inbound", at)

	// Hand-logged, not connector-captured: `captured_by` is a human, which is
	// precisely what the due-threads filter drops.
	// The timestamp is derived from the FROZEN clock, never from now(): this
	// suite's services read extractClock, and a fixture seeded against the
	// database clock would drift a day further from it every day the suite is
	// not run.
	handLoggedAt := at.Add(time.Minute).UTC().Format(time.RFC3339Nano)
	handLogged := SeedIDRow(t, OwnerConn(t), `
		INSERT INTO activity (id, kind, direction, subject, body, thread_key, occurred_at, created_at, source, captured_by, audience)
		VALUES ($1, 'email', 'outbound', 'Renewal', 'internal note: we are preparing to terminate', 'thread-mixed',
		        '`+handLoggedAt+`', '`+handLoggedAt+`', 'manual', 'human:someone', 'participants')`)
	LinkActivity(t, OwnerConn(t), handLogged, "person", contact)

	brain := &scriptedBrain{reply: `{"events": []}`}
	if raised := extractPass(t, e, brain); raised != 0 {
		t.Fatalf("a conversation carrying a hand-logged limited message was read: %d signals raised", raised)
	}
	if brain.calls != 0 {
		t.Errorf("the model was called %d times on a conversation one of whose messages is withheld from the summary's readers", brain.calls)
	}

	// Opening it offers the same conversation, which proves the refusal was the
	// audience and not the hand-logged provenance.
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE activity SET audience = 'workspace' WHERE id = $1`, handLogged); err != nil {
		t.Fatal(err)
	}
	reopened := &scriptedBrain{reply: `{"events": []}`}
	extractPass(t, e, reopened)
	if reopened.calls != 1 {
		t.Errorf("the re-opened conversation was read %d times, want 1 — the fixture was refused for something other than its audience", reopened.calls)
	}
}

func (b *recordingBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	for _, m := range req.Messages {
		b.prompts = append(b.prompts, m.Content)
	}
	return model.Response{Text: b.reply}, nil
}
