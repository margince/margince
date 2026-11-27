// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A reply sent from Margince on a new topic starts its own conversation, as the
// same reply sent from the mailbox would. The wire, the stored row and the echo
// agree on the key. The owner's hold on the old conversation covers the new one.
func TestAReplySentOnANewTopicStartsItsOwnConversation(t *testing.T) {
	t.Parallel()
	p, anchor := heldConversation(t, clocktest.Now(t).Add(-time.Hour))
	deliveryID, messageID := p.sendSplitReply(t, anchor, "transactional")
	if got := p.threadKeyOfSourceID(t, messageID); got != messageID {
		t.Fatalf("the sent reply is filed under %q, want its own conversation %q", got, messageID)
	}
	if got := p.seatVerdict(t, messageID); got != "held_by_owner" {
		t.Errorf("the new conversation's verdict is %q, want the owner's hold carried over", got)
	}

	rfc822, gmailConnector := p.transmit(t, deliveryID, "")
	for _, header := range []string{"\r\nIn-Reply-To:", "\r\nReferences:"} {
		if strings.Contains(string(rfc822), header) {
			t.Errorf("the message carries %s, which files it back into the old conversation", strings.TrimSpace(header))
		}
	}
	msg, err := mailmap.Parse(rfc822, sendingMailbox)
	if err != nil {
		t.Fatalf("parsing what went out: %v", err)
	}
	echo := msg.AttestSentByOwner(true).ToRecord(gmailConnector.Descriptor().Name, rfc822)
	if echo.ThreadKey != messageID {
		t.Fatalf("the mailbox copy derives the key %q from its headers, want %q", echo.ThreadKey, messageID)
	}
	if _, err := capture.NewSink(p.DB()).Upsert(p.mailboxCtx(t), echo); err != nil {
		t.Fatalf("capturing the mailbox copy: %v", err)
	}
	if got := p.threadKeyOfSourceID(t, messageID); got != messageID {
		t.Errorf("after the echo the reply is filed under %q, want %q", got, messageID)
	}
}

// Gmail files the sent copy under an identity of its own, which renames the
// reply's conversation. The owner's hold moves to the new name with it.
func TestAHoldFollowsASplitReplyTheProviderRenames(t *testing.T) {
	t.Parallel()
	p, anchor := heldConversation(t, clocktest.Now(t).Add(-time.Hour))
	deliveryID, _ := p.sendSplitReply(t, anchor, "transactional")
	transmitted, _ := p.transmit(t, deliveryID, gmailStamped)
	if got := p.seatVerdict(t, gmailStamped); got != "held_by_owner" {
		t.Errorf("after the provider renamed the reply, its conversation's verdict is %q, want the hold", got)
	}
	p.captureEcho(t, storedCopy(t, transmitted, gmailStamped))
	if got := p.threadKeyOfSourceID(t, gmailStamped); got != gmailStamped {
		t.Errorf("after the echo the reply is filed under %q, want %q", got, gmailStamped)
	}
	if got := p.seatVerdict(t, gmailStamped); got != "held_by_owner" {
		t.Errorf("after the echo the reply's verdict is %q, want the hold", got)
	}
}

// The buyer wrote two years ago and the send names no purpose, so only the
// thread arm admits the reply. Transmit asks the conversation it answers.
func TestOnlyTheAnsweredThreadAdmitsAnOldSplitReply(t *testing.T) {
	t.Parallel()
	p, anchor := heldConversation(t, clocktest.Now(t).Add(-2*365*24*time.Hour))
	deliveryID, _ := p.sendSplitReply(t, anchor, "")
	p.transmit(t, deliveryID, "")
}

// heldConversation captures the buyer's mail at the instant given and holds its
// conversation for the sending seat, through the share door.
func heldConversation(t *testing.T, at time.Time) (*preflightEnv, ids.UUID) {
	t.Helper()
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	anchor := p.theyWrite(t, inbound{dated: at, arrived: at, authenticated: true})
	if status := p.Call(t, "POST", "/v1/activities/threads/inquiry@preflight.test/audience",
		AnyMap{"share": false}, nil, nil); status != http.StatusOK {
		t.Fatalf("holding the conversation → %d, want 200", status)
	}
	return p, anchor
}

// sendSplitReply answers the anchor on a new topic and returns the delivery and
// the reply's own identity.
func (p *preflightEnv) sendSplitReply(t *testing.T, anchor ids.UUID, purpose string) (ids.UUID, string) {
	t.Helper()
	body := AnyMap{"subject": "Invoice 4711 overdue", "body": "Attached.", "to": []string{"buyer@preflight.test"}}
	if purpose != "" {
		body["consent_purpose"] = purpose
	}
	var sent struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", "/v1/activities/"+anchor.String()+"/send-email", body, nil, &sent); status != http.StatusAccepted {
		t.Fatalf("send-email → %d, want 202", status)
	}
	return p.deliveryFor(t, uuidOf(t, sent.ID))
}

func (p *preflightEnv) threadKeyOfSourceID(t *testing.T, sourceID string) string {
	t.Helper()
	var key string
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(thread_key, '') FROM activity WHERE source_id = $1`, sourceID).Scan(&key)
	}); err != nil {
		t.Fatalf("reading the thread key of %s: %v", sourceID, err)
	}
	return key
}

func (p *preflightEnv) seatVerdict(t *testing.T, threadKey string) string {
	t.Helper()
	var status string
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT coalesce(max(status), 'none') FROM capture_thread_verdict
			 WHERE thread_key = $1 AND user_id = $2`, threadKey, p.user).Scan(&status)
	}); err != nil {
		t.Fatalf("reading the verdict on %s: %v", threadKey, err)
	}
	return status
}
