// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func captureSentMail(t *testing.T, e *integration.Env, messageID, to string) ids.UUID {
	t.Helper()
	raw := []byte("From: Rep One <a@authz.test>\r\nTo: " + to + "\r\nSubject: Angebot\r\n" +
		"Date: Mon, 02 Jan 2026 10:00:00 +0000\r\nMessage-ID: <" + messageID + "@mail.example>\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\nAnbei.\r\n")
	parsed, err := mailmap.Parse(raw, "a@authz.test")
	if err != nil {
		t.Fatalf("parsing %s: %v", messageID, err)
	}
	ref, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(mailboxOwnerCtx(e, e.Rep1),
		parsed.AttestSentByOwner(true).ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("capturing %s: %v", messageID, err)
	}
	return ref.ID
}

// Mail the owner sent to a colleague's provider-proven mailbox, which is not
// their login, still carries a participant row bound to that colleague. The
// attendance arm reads only rows with a user_id.
func TestAMailSentToAColleaguesProvedMailboxBindsTheColleague(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const mailbox = "rep2.box@home.example"
	connectSeatMailbox(t, e, e.Rep2, mailbox, "shared")

	mail := captureSentMail(t, e, "proved-box-1", mailbox)

	if n := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = $2 AND user_id = $3`,
		mail, mailbox, e.Rep2); n != 1 {
		t.Errorf("%d participant rows bind the colleague's proven mailbox to their seat, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = $2 AND user_id IS NULL`,
		mail, mailbox); n != 0 {
		t.Errorf("%d address-only rows for the colleague's mailbox, want 0", n)
	}
}
