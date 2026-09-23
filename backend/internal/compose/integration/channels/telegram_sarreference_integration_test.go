// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// The Art. 15 half of raw_capture's own reference. A channel original is keyed
// on the poll's redelivery counter and never on the activity's own key, so the
// disclosure gate can only follow activity.raw_capture_id — and a gate that
// matches nothing withholds an open message from its own subject, which no
// normal run surfaces.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestASubjectReceivesTheirOpenChannelOriginal proves the fix: an ordinary,
// fully open Telegram message is captured, and its own subject's export
// discloses the payload rather than withholding it for a key that was never
// going to match.
func TestASubjectReceivesTheirOpenChannelOriginal(t *testing.T) {
	c := setupTelegramConnected(t)
	u := telegramUpdate{updateID: 6300, messageID: 63, senderID: 771301, username: "openoriginal", firstName: "Aylin", text: "what they wrote"}

	c.ingestOne(t, u, compose.JobRunnerConfig{})

	_, contactID := c.capturedMessage(t, u)
	contact, err := ids.Parse(contactID)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := privacy.AssembleSAR(c.adminStoreCtx(t), c.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.RawCapture) != 1 {
		t.Fatalf("the export lists %d originals, want 1", len(pkg.RawCapture))
	}
	if pkg.RawCapture[0]["payload"] == nil {
		t.Fatal("the original of an OPEN message is withheld from its own subject: the disclosure gate " +
			"could not find the activity that names it")
	}
}

// telegramMembershipRaw renders a my_chat_member update in the shape the poll
// stores and telegram.ParseMembership reads: chat.id IS the private chat's
// customer (new_chat_member.user names the BOT, never the customer — see
// telegram/membership.go), and it is classified before Normalize ever runs,
// so it never mints an activity.
func telegramMembershipRaw(t *testing.T, updateID, chatID int64, username, status string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"update_id": updateID,
		"my_chat_member": map[string]any{
			"chat": map[string]any{"id": chatID, "type": "private", "username": username},
			"new_chat_member": map[string]any{
				"user":   map[string]any{"id": telegramBotID, "is_bot": true, "username": telegramBotUser},
				"status": status,
			},
		},
	})
	if err != nil {
		t.Fatalf("building the membership update: %v", err)
	}
	return raw
}

// TestAnOriginalNoRecordVouchesForStaysWithheld is the withholding side: a
// my_chat_member update is captured under the same channel identity as an
// earlier open message, but telegram.ParseMembership consumes it before
// Normalize ever runs, so it never becomes an activity — the shape an
// internal-only drop or an activity erased ahead of its original both leave
// behind too. Its raw_capture row is still listed, because Art. 15 owes the
// fact that an original is held, and its payload stays withheld because
// nothing names it.
func TestAnOriginalNoRecordVouchesForStaysWithheld(t *testing.T) {
	c := setupTelegramConnected(t)
	u := telegramUpdate{updateID: 6400, messageID: 64, senderID: 771302, username: "membertest", firstName: "Deniz", text: "first hello"}

	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)

	// Bootstrap: one real message binds the contact to a channel identity —
	// SetChannelIdentityBlocked matches no row for an identity nobody has
	// bound yet, so the membership update below needs this to have anything
	// to apply to.
	c.arrive(t, sub, u)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	_, contactID := c.capturedMessage(t, u)

	membershipUpdateID := int64(6401)
	c.api.hold(telegramMembershipRaw(t, membershipUpdateID, u.senderID, u.username, "kicked"))
	c.pollNow(t, sub, membershipUpdateID+1)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())

	contact, err := ids.Parse(contactID)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := privacy.AssembleSAR(c.adminStoreCtx(t), c.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.RawCapture) != 2 {
		t.Fatalf("the export lists %d originals, want 2 — the message an activity names and the "+
			"membership update no activity ever does", len(pkg.RawCapture))
	}
	var disclosed, withheld int
	for _, row := range pkg.RawCapture {
		if row["payload"] != nil {
			disclosed++
		} else {
			withheld++
		}
	}
	if disclosed != 1 {
		t.Errorf("%d originals disclosed, want 1 — the message an activity names", disclosed)
	}
	if withheld != 1 {
		t.Errorf("an original no record vouches for was disclosed (%d withheld, want 1) — the my_chat_member "+
			"update never became an activity, and absence must not read as consent", withheld)
	}
}
