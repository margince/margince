// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A classified chat is judged one burst at a time. Chat follows mail's two
// layers: the contact's sharing and the message's own audience. An ordinary
// burst on a shared contact opens for a colleague. A confidential burst on the
// same contact stays held, and a burst's verdict never reaches another burst.
// These cases file through capture.FileChatMessage and judge through the real
// engine, with the model standing in at the completion boundary.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// burstBrain answers a chat burst by what it says: a burst that mentions the
// diagnosis is confidential, any other is ordinary. Every mail thread is
// ordinary. It records the chat prompts it was shown.
type burstBrain struct {
	chatPrompts atomic.Int64
	seen        []string
}

func (b *burstBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	marker, ok := promptfence.MarkerIn(req.System)
	if !ok {
		return model.Response{}, fmt.Errorf("the prompt names no boundary")
	}
	prompt := req.Messages[0].Content
	_, rest, found := strings.Cut(prompt, "<"+marker+` id="`)
	ref, _, closed := strings.Cut(rest, `"`)
	if !found || !closed {
		return model.Response{}, fmt.Errorf("the prompt fences no thread")
	}
	verdict := "ordinary"
	if strings.Contains(prompt, "Chat conversation") {
		b.chatPrompts.Add(1)
		b.seen = append(b.seen, prompt)
		if strings.Contains(prompt, "diagnosis") {
			verdict = "personal"
		}
	}
	text, err := json.Marshal(map[string]any{"results": []map[string]any{{"id": ref, "verdict": verdict, "confidence": 0.95}}})
	return model.Response{Text: string(text)}, err
}

// burstFiling files one member's chat messages, each at an offset from base.
type burstFiling struct {
	t    *testing.T
	fx   chatFixture
	base time.Time
}

func newBurstFiling(t *testing.T, posture string, team bool) burstFiling {
	t.Helper()
	return burstFiling{t: t, fx: newChatFixture(t, posture, team), base: clocktest.Now(t).Add(-48 * time.Hour).Truncate(time.Second)}
}

// file stores one inbound message under its provider id and returns its activity.
func (f burstFiling) file(key string, offset time.Duration, body string) ids.UUID {
	f.t.Helper()
	return f.fileIn("990100", key, offset, body)
}

// fileIn is file in the chat of one conversation.
func (f burstFiling) fileIn(conversation, key string, offset time.Duration, body string) ids.UUID {
	f.t.Helper()
	chat, err := capturemod.FileChatMessage(f.fx.ctx,
		compose.NewCaptureSink(f.fx.e.Pool, compose.CaptureConfig{}), f.fx.binding,
		connector.ChatMessage{
			ConversationID: conversation, IdempotencyKey: key, Direction: connector.DirectionInbound,
			Counterparty: connector.ChatParty{UserID: conversation, DisplayName: "Buyer"},
			Body:         body, OccurredAt: f.base.Add(offset),
		})
	if err != nil {
		f.t.Fatalf("filing chat message %s: %v", key, err)
	}
	return chat.ID
}

// burstOf reads the burst a message was placed in, zero when it has none.
func (f burstFiling) burstOf(activity ids.UUID) ids.UUID {
	f.t.Helper()
	var burst *ids.UUID
	if err := database.WithWorkspaceTx(f.fx.e.Admin(), f.fx.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT burst_id FROM capture_import WHERE activity_id = $1`, activity).Scan(&burst)
	}); err != nil {
		f.t.Fatalf("reading the burst of a message: %v", err)
	}
	if burst == nil {
		return ids.Nil
	}
	return *burst
}

// ledgerRows counts the ledger rows whose key is one of the given burst ids or
// whose first message is one of the given activities.
func (f burstFiling) ledgerRows(activities ...ids.UUID) int {
	f.t.Helper()
	var n int
	if err := database.WithWorkspaceTx(f.fx.e.Admin(), f.fx.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM capture_thread_verdict WHERE first_activity_id = ANY($1)`, activities).Scan(&n)
	}); err != nil {
		f.t.Fatalf("counting ledger rows: %v", err)
	}
	return n
}

// share publishes the contact to the workspace, as its owner does.
func (f burstFiling) share() {
	f.t.Helper()
	store := contacts.NewStore(f.fx.e.DB()).WithSenderStamp(activities.StampProvenSendersTx)
	if err := store.PromoteOwnCapturedContact(ownerOf(f.fx.e), f.fx.contact); err != nil {
		f.t.Fatalf("publishing the contact: %v", err)
	}
}

// judge runs one engine pass with the model, or with none when brain is nil.
func (f burstFiling) judge(brain *burstBrain) {
	f.t.Helper()
	var lane interface {
		Complete(context.Context, model.Request) (model.Response, error)
	}
	if brain != nil {
		lane = brain
	}
	engine := compose.NewConfidentialityVerdictEngine(f.fx.e.Pool, lane, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), f.fx.e.WS), 0); err != nil {
		f.t.Fatalf("confidentiality pass: %v", err)
	}
}

func (f burstFiling) readers(activity ids.UUID) []ids.UUID { return readersOf(f.t, f.fx.e, activity) }

func (f burstFiling) wantReaders(step string, activity ids.UUID, want int) {
	f.t.Helper()
	if got := f.readers(activity); len(got) != want {
		f.t.Fatalf("%s: readable by %v, want %d seats", step, got, want)
	}
}

func TestEachChatBurstIsJudgedOnItsOwnAndNoVerdictReachesAnotherBurst(t *testing.T) {
	f := newBurstFiling(t, "classified", false)
	one := f.file("1", 0, "Danke fuer das Angebot, wir melden uns.")
	two := f.file("2", 5*time.Minute, "Passt der Liefertermin im Dezember?")
	three := f.file("3", 14*time.Minute, "Dann bestellen wir.")
	four := f.file("4", 2*time.Hour, "Ich habe die diagnosis vom Arzt bekommen.")
	f.share()

	for _, step := range []struct {
		name string
		id   ids.UUID
	}{{"first burst", one}, {"first burst", two}, {"first burst", three}, {"second burst", four}} {
		f.wantReaders(step.name+" before any verdict", step.id, 1)
	}

	brain := &burstBrain{}
	f.judge(brain)

	if got := brain.chatPrompts.Load(); got != 2 {
		t.Fatalf("the model judged %d chat bursts, want one call per burst (2)", got)
	}
	for _, prompt := range brain.seen {
		if strings.Contains(prompt, "diagnosis") == strings.Contains(prompt, "Angebot") {
			t.Fatalf("a call read both bursts or neither:\n%s", prompt)
		}
	}
	for _, id := range []ids.UUID{one, two, three} {
		f.wantReaders("an ordinary burst on a shared contact", id, 2)
	}
	f.wantReaders("a confidential burst on the same shared contact", four, 1)

	// A message after the judged burst starts a burst of its own, born held
	// and not opened by the ordinary verdict before it.
	five := f.file("5", 14*time.Minute+5*time.Minute, "Noch eine Frage zur Rechnung.")
	if f.burstOf(five) == f.burstOf(three) {
		t.Fatal("a message after a judged burst joined it, so the verdict would cover text it never read")
	}
	f.wantReaders("a later burst before its own verdict", five, 1)
	f.wantReaders("the first burst after a later one was filed", one, 2)
	f.judge(brain)
	f.wantReaders("a later ordinary burst once judged", five, 2)
	f.wantReaders("the confidential burst after a later one was judged", four, 1)
}

func TestAMessageJoinsTheBurstWhenTheGapIsUnderFifteenMinutes(t *testing.T) {
	f := newBurstFiling(t, "classified", false)
	one := f.file("1", 0, "Hallo")
	ten := f.file("2", 10*time.Minute, "Noch da?")
	late := f.file("3", 30*time.Minute, "Wieder da, 20 Minuten spaeter.")

	if f.burstOf(ten) != f.burstOf(one) {
		t.Fatal("a message 10 minutes after the last one did not join its burst")
	}
	if f.burstOf(late) == f.burstOf(ten) {
		t.Fatal("a message 20 minutes after the last one joined its burst")
	}

	f.file("1", 0, "Hallo")
	f.file("3", 30*time.Minute, "Wieder da, 20 Minuten spaeter.")
	if f.burstOf(ten) != f.burstOf(one) || f.burstOf(late) == f.burstOf(one) {
		t.Fatal("a redelivered message moved to another burst")
	}
	if got := f.ledgerRows(one, late); got != 2 {
		t.Fatalf("two bursts have %d ledger rows, want one each", got)
	}
}

func TestWithNoModelEveryChatBurstStaysHeld(t *testing.T) {
	f := newBurstFiling(t, "classified", false)
	one := f.file("1", 0, "Danke fuer das Angebot.")
	two := f.file("2", 3*time.Hour, "Und ein zweites Thema.")
	f.share()
	f.judge(nil)
	f.wantReaders("a burst with no model bound", one, 1)
	f.wantReaders("a second burst with no model bound", two, 1)
}

func TestABurstStillBeingWrittenToWaitsForItsPause(t *testing.T) {
	f := newBurstFiling(t, "classified", false)
	f.base = clocktest.Now(f.t).Add(-5 * time.Minute)
	one := f.file("1", 0, "Danke fuer das Angebot.")
	f.share()
	brain := &burstBrain{}
	f.judge(brain)
	if got := brain.chatPrompts.Load(); got != 0 {
		t.Fatalf("a burst that was written to 5 minutes ago was judged %d times", got)
	}
	f.wantReaders("a burst still being written to", one, 1)
	var attempts int
	if err := database.WithWorkspaceTx(f.fx.e.Admin(), f.fx.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT attempts FROM capture_thread_verdict WHERE first_activity_id = $1`, one).Scan(&attempts)
	}); err != nil || attempts != 0 {
		t.Fatalf("waiting for a pause cost %d attempts (err %v), want none", attempts, err)
	}
}

func TestOnlyAClassifiedMemberSeatOpensABurstQuestion(t *testing.T) {
	for _, seat := range []struct {
		name, posture string
		team          bool
	}{{"team seat shared", "shared", true}, {"team seat held", "held", true}, {"member seat held", "held", false}, {"member seat shared", "shared", false}} {
		t.Run(seat.name, func(t *testing.T) {
			f := newBurstFiling(t, seat.posture, seat.team)
			chat := f.file("1", 0, "Danke fuer das Angebot.")
			if got := f.ledgerRows(chat); got != 0 {
				t.Fatalf("%d ledger rows for a seat that is never classified", got)
			}
			if f.burstOf(chat) != ids.Nil {
				t.Fatal("a message on a seat that is never classified was placed in a burst")
			}
		})
	}
}

// A message that reaches the burst after the model's claim was not read by it.
// The answer covers the messages it read, and the others move to a burst of
// their own with a question of their own.
func TestAMessageTheVerdictDidNotReadMovesToItsOwnBurst(t *testing.T) {
	f := newBurstFiling(t, "classified", false)
	one := f.file("1", 0, "Danke fuer das Angebot.")
	two := f.file("2", time.Minute, "Und noch etwas.")
	store := capturemod.NewThreadVerdictStore(compose.InstallationDB(f.fx.e.Pool))
	claimed, err := store.ClaimDue(principal.SystemActing(f.fx.e.Admin(), "agent:capture_confidentiality_verdict"), 10)
	if err != nil {
		t.Fatalf("claiming: %v", err)
	}
	var row capturemod.PendingThread
	for _, c := range claimed {
		if c.IsBurst() {
			row = c
		}
	}
	if len(row.BurstMembers) != 2 {
		t.Fatalf("the claim read %d messages, want the burst's two", len(row.BurstMembers))
	}
	row.BurstMembers = row.BurstMembers[:1]

	var moved []ids.UUID
	if err := database.WithWorkspaceTx(principal.SystemActing(f.fx.e.Admin(), "agent:capture_confidentiality_verdict"), f.fx.e.Pool, func(tx pgx.Tx) error {
		out, err := store.RecordOutcomeOnRowTx(context.Background(), tx, row, capturemod.VerdictCleared, "ordinary", nil)
		moved = out.Moved
		return err
	}); err != nil {
		t.Fatalf("applying the answer: %v", err)
	}
	if len(moved) != 1 || moved[0] != two {
		t.Fatalf("moved %v, want only the message the verdict did not read (%s)", moved, two)
	}
	if f.burstOf(two) == f.burstOf(one) {
		t.Fatalf("both messages are still in burst %s", f.burstOf(one))
	}
	if got := f.ledgerRows(two); got != 1 {
		t.Fatalf("the unread message has %d questions of its own, want 1", got)
	}
}

// liveIdentities counts the live channel identities of one chat user, which is
// what stands for a contact only a chat created.
func (f burstFiling) liveIdentities(channelUser string) int {
	f.t.Helper()
	var n int
	if err := database.WithWorkspaceTx(f.fx.e.Admin(), f.fx.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM contact_channel_identity i JOIN contact c ON c.id = i.contact_id
			 WHERE i.channel_user_id = $1 AND i.archived_at IS NULL AND c.archived_at IS NULL`, channelUser).Scan(&n)
	}); err != nil {
		f.t.Fatalf("counting the identities of a chat user: %v", err)
	}
	return n
}

func TestAPersonalBurstRetractsAChatOnlyContactAndALaterBurstDoesNotMakeItAgain(t *testing.T) {
	f := newBurstFiling(t, "classified", false)
	const stranger = "770200"
	first := f.fileIn(stranger, "1", 0, "Ich habe die diagnosis vom Arzt bekommen.")
	if got := f.liveIdentities(stranger); got != 1 {
		t.Fatalf("the chat made %d contacts for its first message, want 1", got)
	}

	f.judge(&burstBrain{})
	if got := f.liveIdentities(stranger); got != 0 {
		var status, kind string
		var links, owners int
		if err := database.WithWorkspaceTx(f.fx.e.Admin(), f.fx.e.Pool, func(tx pgx.Tx) error {
			if err := tx.QueryRow(context.Background(), `SELECT status, coalesce(kind, '') FROM capture_thread_verdict WHERE first_activity_id = $1`, first).Scan(&status, &kind); err != nil {
				return err
			}
			return tx.QueryRow(context.Background(), `SELECT count(*), count(c.owner_id) FROM activity_link l JOIN contact c ON c.id = l.contact_id WHERE l.activity_id = $1 AND l.entity_type = 'contact'`, first).Scan(&links, &owners)
		}); err != nil {
			t.Fatalf("reading the state: %v", err)
		}
		t.Logf("ledger %s/%s, %d contact links, %d owned", status, kind, links, owners)
		t.Fatalf("a personal burst left %d live contacts, want its chat-only contact retracted", got)
	}

	later := f.fileIn(stranger, "2", 3*time.Hour, "Passt der Termin am Montag?")
	if got := f.liveIdentities(stranger); got != 0 {
		t.Fatalf("a later burst of the same chat made %d contacts again", got)
	}
	f.wantReaders("a later burst in a chat judged personal", later, 1)
}

// A mail thread's root is the sender's own text, so it may equal any string,
// a burst's id among them. It must neither read nor move the burst's verdict.
func TestAMailThreadRootedAtABurstIdLeavesTheBurstVerdictAlone(t *testing.T) {
	f := newBurstFiling(t, "classified", false)
	one := f.file("1", 0, "Danke fuer das Angebot.")
	f.share()
	f.judge(&burstBrain{})
	f.wantReaders("a judged ordinary burst", one, 2)

	f.fx.env.sync(t, email("stranger@elsewhere.example", "Stranger", captureOwner, "root-collision@elsewhere.example", f.burstOf(one).String()))
	f.wantReaders("the burst after a mail rooted at its id arrived", one, 2)
	var status string
	if err := database.WithWorkspaceTx(f.fx.e.Admin(), f.fx.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status FROM capture_thread_verdict WHERE first_activity_id = $1`, one).Scan(&status)
	}); err != nil || status != capturemod.VerdictCleared {
		t.Fatalf("the burst's verdict is %q (err %v) after a mail rooted at its id, want cleared", status, err)
	}
}

// edit changes the text of one filed message, later than the filing.
func (f burstFiling) edit(key, body string) {
	f.t.Helper()
	sink := compose.NewCaptureSink(f.fx.e.Pool, compose.CaptureConfig{})
	outcome, err := capturemod.ChangeChatMessage(f.fx.ctx, sink, f.fx.binding, connector.ChatChange{
		Kind: connector.ChatEdited, ConversationID: "990100", IdempotencyKey: key,
		Body: body, Revision: clocktest.Now(f.t),
	})
	if err != nil || outcome != connector.ChatChangeApplied {
		f.t.Fatalf("editing message %s: outcome %q, err %v", key, outcome, err)
	}
}

// A verdict covers the text the model read. An edit after it takes the message
// out of the burst, so the old answer cannot open the new text.
func TestAnEditedMessageOfAJudgedBurstIsNotOpenedOnItsOldText(t *testing.T) {
	f := newBurstFiling(t, "classified", false)
	one := f.file("1", 0, "Danke fuer das Angebot.")
	two := f.file("2", time.Minute, "Passt der Liefertermin?")
	f.share()
	brain := &burstBrain{}
	f.judge(brain)
	f.wantReaders("a judged ordinary burst", two, 2)

	f.edit("2", "Ich habe die diagnosis vom Arzt bekommen.")
	if f.burstOf(two) == f.burstOf(one) {
		t.Fatal("an edited message stayed in the burst judged on its old text")
	}
	f.wantReaders("the edited message before its own verdict", two, 1)
	f.wantReaders("the unedited message of the same burst", one, 2)
	if got := f.ledgerRows(two); got != 1 {
		t.Fatalf("the edited message has %d questions of its own, want 1", got)
	}

	f.judge(brain)
	f.wantReaders("the edited message judged personal", two, 1)
	f.wantReaders("the unedited message after the edit was judged", one, 2)
}

func TestAnEditBeforeTheVerdictKeepsTheBurst(t *testing.T) {
	f := newBurstFiling(t, "classified", false)
	one := f.file("1", 0, "Danke fuer das Angebot.")
	two := f.file("2", time.Minute, "Passt der Liefertermin?")
	f.edit("2", "Passt der Liefertermin im Dezember?")
	if f.burstOf(two) != f.burstOf(one) {
		t.Fatal("an edit of a message in a burst nobody judged moved it out")
	}
}
