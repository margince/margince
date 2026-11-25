// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// What a chat provider says about a message after it was filed. Every case runs
// through capture.FileChatMessage and capture.ChangeChatMessage, the two
// functions every chat network uses, over the sink production composes.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const chatChangeConversation = "990100"

var chatChangeEpoch = time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)

type chatChangeEnv struct {
	e       *integration.SearchEnv
	sink    *capturemod.Sink
	binding connector.ChatBinding
}

// newChatChangeEnv is one member's own Telegram account, the case whose deletion
// destroys content.
func newChatChangeEnv(t *testing.T) chatChangeEnv {
	t.Helper()
	e := newCaptureEnv(t).e
	purger := compose.NewCapturePurger(e.Pool, compose.NewRetentionServiceFor(compose.InstallationDB(e.Pool), nil, slog.Default()))
	return chatChangeEnv{
		e:    e,
		sink: compose.NewCaptureSink(e.Pool, compose.CaptureConfig{}).WithChatPurger(purger.PurgeRemovedChat),
		binding: connector.ChatBinding{
			Provider: "telegram", CapturedBy: "connector:telegram", KeyScope: "42", ThreadScope: "42",
			Ownership: connector.ChatOwnedByMember, OwnerUserID: e.Rep1,
		},
	}
}

func chatOriginal(id, body string) connector.ChatMessage {
	return connector.ChatMessage{
		ConversationID: chatChangeConversation, IdempotencyKey: id, Direction: connector.DirectionInbound,
		Counterparty: connector.ChatParty{UserID: chatChangeConversation, DisplayName: "Buyer"},
		Body:         body, OccurredAt: chatChangeEpoch,
	}
}

func chatEdit(id, body string, minutes int) connector.ChatChange {
	return connector.ChatChange{
		Kind: connector.ChatEdited, ConversationID: chatChangeConversation, IdempotencyKey: id,
		Body: body, Revision: chatChangeEpoch.Add(time.Duration(minutes) * time.Minute),
	}
}

func chatDeletion(id string) connector.ChatChange {
	return connector.ChatChange{Kind: connector.ChatDeleted, ConversationID: chatChangeConversation, IdempotencyKey: id}
}

func (c chatChangeEnv) file(t *testing.T, id, body string) ids.UUID {
	t.Helper()
	ref, err := capturemod.FileChatMessage(connectorSeatCtx(c.e), c.sink, c.binding, chatOriginal(id, body))
	if err != nil {
		t.Fatalf("filing message %s: %v", id, err)
	}
	return ref.ID
}

func (c chatChangeEnv) change(t *testing.T, change connector.ChatChange) connector.ChatChangeOutcome {
	t.Helper()
	outcome, err := capturemod.ChangeChatMessage(connectorSeatCtx(c.e), c.sink, c.binding, change)
	if err != nil {
		t.Fatalf("changing message %s (%s): %v", change.IdempotencyKey, change.Kind, err)
	}
	return outcome
}

func (c chatChangeEnv) body(t *testing.T, id string) string {
	t.Helper()
	return wsScalar(t, c.e, `SELECT coalesce(body, '') FROM activity WHERE source_system = 'telegram' AND source_id = $1`, "42:"+chatChangeConversation+":"+id)
}

// trail counts the audit rows and outbox events the message's activity has.
func (c chatChangeEnv) trail(t *testing.T, activity ids.UUID) (audits, events int) {
	t.Helper()
	audits = wsCount(t, c.e, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, activity)
	events = wsCount(t, c.e, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1::text`, activity)
	return audits, events
}

func (c chatChangeEnv) activities(t *testing.T) int {
	t.Helper()
	return wsCount(t, c.e, `SELECT count(*) FROM activity WHERE source_system = 'telegram'`)
}

func wantOutcome(t *testing.T, step string, got, want connector.ChatChangeOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: outcome %q, want %q", step, got, want)
	}
}

func TestAnEditEndsAtTheNewestTextWhateverOrderTheReportsArriveIn(t *testing.T) {
	c := newChatChangeEnv(t)
	id := c.file(t, "7", "first text")
	auditsBefore, eventsBefore := c.trail(t, id)

	wantOutcome(t, "newer edit", c.change(t, chatEdit("7", "second text", 10)), connector.ChatChangeApplied)
	if got := c.body(t, "7"); got != "second text" {
		t.Fatalf("body %q after the edit, want the edited text", got)
	}
	audits, events := c.trail(t, id)
	if audits != auditsBefore+1 || events != eventsBefore+1 {
		t.Fatalf("an edit left %d audit rows and %d events, want one of each added to %d and %d", audits, events, auditsBefore, eventsBefore)
	}

	wantOutcome(t, "older edit arriving late", c.change(t, chatEdit("7", "older text", 5)), connector.ChatChangeStale)
	if got := c.body(t, "7"); got != "second text" {
		t.Fatalf("body %q after a stale edit, want the newest text", got)
	}

	wantOutcome(t, "redelivered edit", c.change(t, chatEdit("7", "second text", 10)), connector.ChatChangeStale)
	if a, ev := c.trail(t, id); a != audits || ev != events {
		t.Fatalf("a redelivered edit left %d audit rows and %d events, want the %d and %d it had", a, ev, audits, events)
	}

	// The original, redelivered after its edit, files nothing and restores nothing.
	c.file(t, "7", "first text")
	if got := c.body(t, "7"); got != "second text" {
		t.Fatalf("body %q after the original was redelivered, want the edited text", got)
	}

	wantOutcome(t, "newest edit", c.change(t, chatEdit("7", "third text", 20)), connector.ChatChangeApplied)
	if got := c.body(t, "7"); got != "third text" {
		t.Fatalf("body %q, want the newest text", got)
	}
}

func TestAnEditThatArrivesBeforeTheOriginalFilesTheNewestText(t *testing.T) {
	c := newChatChangeEnv(t)
	edit := chatEdit("8", "edited text", 10)
	restated := chatOriginal("8", "edited text")
	edit.Restated = &restated

	wantOutcome(t, "edit before the original", c.change(t, edit), connector.ChatChangeApplied)
	if got := c.body(t, "8"); got != "edited text" {
		t.Fatalf("body %q, want the edited text filed from the restated message", got)
	}

	c.file(t, "8", "original text")
	if n := c.activities(t); n != 1 {
		t.Fatalf("%d activities after the late original, want the one the edit filed", n)
	}
	if got := c.body(t, "8"); got != "edited text" {
		t.Fatalf("body %q after the late original, want the edited text", got)
	}
}

func TestAnEditThatCannotRestateTheMessageWaitsForTheOriginal(t *testing.T) {
	c := newChatChangeEnv(t)
	edit := chatEdit("9", "edited text", 10)

	wantOutcome(t, "edit before the original", c.change(t, edit), connector.ChatChangeNotFiled)
	if n := c.activities(t); n != 0 {
		t.Fatalf("%d activities, want none: an edit with no text of its own files nothing", n)
	}

	c.file(t, "9", "original text")
	wantOutcome(t, "edit after the original", c.change(t, edit), connector.ChatChangeApplied)
	if got := c.body(t, "9"); got != "edited text" {
		t.Fatalf("body %q, want the edited text", got)
	}
}

func TestADeletionBeforeTheOriginalRefusesALateOriginal(t *testing.T) {
	c := newChatChangeEnv(t)

	wantOutcome(t, "deletion before the original", c.change(t, chatDeletion("10")), connector.ChatChangeTombstoned)
	_, err := capturemod.FileChatMessage(connectorSeatCtx(c.e), c.sink, c.binding, chatOriginal("10", "late original"))
	if !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("filing the late original = %v, want a skip", err)
	}
	if n := c.activities(t); n != 0 {
		t.Fatalf("%d activities after a late original, want none", n)
	}
	restated := chatOriginal("10", "edited")
	edit := chatEdit("10", "edited", 5)
	edit.Restated = &restated
	wantOutcome(t, "edit of a removed message", c.change(t, edit), connector.ChatChangeStale)
	if n := c.activities(t); n != 0 {
		t.Fatalf("%d activities after an edit of a removed message, want none", n)
	}
	// The removal record is a hash of the key: no conversation or account id.
	if n := wsCount(t, c.e, `SELECT count(*) FROM raw_capture WHERE source_system = 'chat_removed' AND (source_id LIKE '%'||$1||'%' OR payload::text LIKE '%'||$1||'%')`, chatChangeConversation); n != 0 {
		t.Fatalf("%d removal records name the conversation id", n)
	}
}

func TestADeletionAfterTheOriginalDestroysAMembersOwnCopy(t *testing.T) {
	c := newChatChangeEnv(t)
	id := c.file(t, "11", "private words")

	wantOutcome(t, "deletion", c.change(t, chatDeletion("11")), connector.ChatChangeRemoved)
	if got := c.body(t, "11"); got != "" {
		t.Fatalf("the deleted message kept its body %q", got)
	}
	if n := wsCount(t, c.e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, id); n != 1 {
		t.Fatal("the deleted message is not archived")
	}
	audits, events := c.trail(t, id)

	wantOutcome(t, "redelivered deletion", c.change(t, chatDeletion("11")), connector.ChatChangeStale)
	if a, ev := c.trail(t, id); a != audits || ev != events {
		t.Fatalf("a redelivered deletion left %d audit rows and %d events, want %d and %d", a, ev, audits, events)
	}

	if _, err := capturemod.FileChatMessage(connectorSeatCtx(c.e), c.sink, c.binding, chatOriginal("11", "private words")); err != nil && !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("filing the original again: %v", err)
	}
	if got := c.body(t, "11"); got != "" {
		t.Fatalf("the original came back with body %q", got)
	}
	if a, ev := c.trail(t, id); a != audits || ev != events {
		t.Fatalf("a redelivered original left %d audit rows and %d events, want %d and %d", a, ev, audits, events)
	}
	wantOutcome(t, "edit of a deleted message", c.change(t, chatEdit("11", "back again", 5)), connector.ChatChangeStale)
	if got := c.body(t, "11"); got != "" {
		t.Fatalf("an edit restored the deleted message's body %q", got)
	}
}

func TestADeletionOnACompanyAccountRemovesNothing(t *testing.T) {
	e := newCaptureEnv(t).e
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	binding := connector.ChatBinding{
		Provider: "telegram", CapturedBy: "connector:telegram", KeyScope: "42:biz:880001", ThreadScope: "biz:880001",
		Ownership: connector.ChatOwnedByWorkspace, SeatID: chatSeatFor(t, e, "shared", true),
	}
	c := chatChangeEnv{e: e, sink: sink, binding: binding}
	ctx := workspaceConnectorCtx(e)
	if _, err := capturemod.FileChatMessage(ctx, sink, binding, chatOriginal("12", "for the team")); err != nil {
		t.Fatalf("filing: %v", err)
	}

	outcome, err := capturemod.ChangeChatMessage(ctx, sink, binding, chatDeletion("12"))
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}
	wantOutcome(t, "deletion on a shared account", outcome, connector.ChatChangeKept)
	body := wsScalar(t, e, `SELECT coalesce(body, '') FROM activity WHERE source_system = 'telegram' AND source_id = '42:biz:880001:990100:12'`)
	if body != "for the team" {
		t.Fatalf("body %q, want the team's copy kept", body)
	}
	if c.activities(t) != 1 {
		t.Fatal("the deletion changed the number of activities")
	}
}

func TestADeliveryStatusNeverTouchesTheConversation(t *testing.T) {
	c := newChatChangeEnv(t)
	id := c.file(t, "13", "sent text")
	audits, events := c.trail(t, id)

	for _, status := range []connector.ChatDeliveryStatus{connector.ChatStatusSent, connector.ChatStatusDelivered, connector.ChatStatusRead, connector.ChatStatusRead} {
		wantOutcome(t, string(status), c.change(t, connector.ChatChange{
			Kind: connector.ChatDeliveryReported, ConversationID: chatChangeConversation, IdempotencyKey: "13",
			Status: status, StatusAt: chatChangeEpoch,
		}), connector.ChatChangeNoted)
	}
	// A status for a message that never landed creates nothing either.
	wantOutcome(t, "status of an unknown message", c.change(t, connector.ChatChange{
		Kind: connector.ChatDeliveryReported, ConversationID: chatChangeConversation, IdempotencyKey: "999", Status: connector.ChatStatusFailed,
	}), connector.ChatChangeNoted)

	if n := c.activities(t); n != 1 {
		t.Fatalf("%d activities after delivery statuses, want the one message", n)
	}
	if a, ev := c.trail(t, id); a != audits || ev != events {
		t.Fatalf("delivery statuses left %d audit rows and %d events, want %d and %d", a, ev, audits, events)
	}
	if got := c.body(t, "13"); !strings.HasPrefix(got, "sent text") {
		t.Fatalf("body %q changed", got)
	}
}

func TestAChangeNeedsTheBindingChecksFilingNeeds(t *testing.T) {
	c := newChatChangeEnv(t)
	c.file(t, "14", "text")
	bad := c.binding
	bad.Ownership = ""
	if _, err := capturemod.ChangeChatMessage(connectorSeatCtx(c.e), c.sink, bad, chatEdit("14", "x", 5)); err == nil {
		t.Fatal("an edit through a binding with no ownership was accepted")
	}
	colon := chatEdit("14", "x", 5)
	colon.IdempotencyKey = "1:4"
	if _, err := capturemod.ChangeChatMessage(connectorSeatCtx(c.e), c.sink, c.binding, colon); err == nil {
		t.Fatal("an edit naming a message id with a colon was accepted")
	}
	if got := c.body(t, "14"); got != "text" {
		t.Fatalf("body %q, want it unchanged", got)
	}
}

func wsCount(t *testing.T, e *integration.SearchEnv, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&n)
	}); err != nil {
		t.Fatalf("counting (%s): %v", query, err)
	}
	return n
}

func wsScalar(t *testing.T, e *integration.SearchEnv, query string, args ...any) string {
	t.Helper()
	var out string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&out)
	}); err != nil {
		t.Fatalf("reading (%s): %v", query, err)
	}
	return out
}
